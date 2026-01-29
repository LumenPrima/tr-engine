package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/trunk-recorder/tr-engine/docs"
	"github.com/trunk-recorder/tr-engine/internal/api/rest"
	"github.com/trunk-recorder/tr-engine/internal/api/ws"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"github.com/trunk-recorder/tr-engine/internal/metrics"
	"github.com/trunk-recorder/tr-engine/internal/ui"
	"go.uber.org/zap"

	_ "github.com/trunk-recorder/tr-engine/docs"
)

// Server is the HTTP/WebSocket server
type Server struct {
	config        config.ServerConfig
	db            *database.DB
	processor     *ingest.Processor
	logger        *zap.Logger
	server        *http.Server
	hub           *ws.Hub
	router        *gin.Engine
	stopMetrics   chan struct{}
	audioBasePath string
}

// NewServer creates a new API server
func NewServer(cfg config.ServerConfig, db *database.DB, processor *ingest.Processor, logger *zap.Logger, audioBasePath string) *Server {
	// Set gin mode based on log level
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))
	router.Use(corsMiddleware())

	hub := ws.NewHub(logger)
	go hub.Run()

	// Connect processor to hub for broadcasting (if processor exists)
	if processor != nil {
		processor.SetHub(hub)
	}

	s := &Server{
		config:        cfg,
		db:            db,
		processor:     processor,
		logger:        logger,
		hub:           hub,
		router:        router,
		stopMetrics:   make(chan struct{}),
		audioBasePath: audioBasePath,
	}

	s.setupRoutes()

	// Start metrics gauge updater
	go s.updateMetricsGauges()

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting API server", zap.String("address", addr))

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Stop metrics updater
	close(s.stopMetrics)

	s.hub.Shutdown()

	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// updateMetricsGauges periodically updates Prometheus gauge metrics
func (s *Server) updateMetricsGauges() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Update immediately on start
	s.refreshGauges()

	for {
		select {
		case <-s.stopMetrics:
			return
		case <-ticker.C:
			s.refreshGauges()
		}
	}
}

// refreshGauges updates all gauge metrics
func (s *Server) refreshGauges() {
	// Update WebSocket client count
	metrics.ActiveWebSocketClients.Set(float64(s.hub.ClientCount()))

	// Get database stats
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := s.db.GetStats(ctx)
	if err != nil {
		s.logger.Debug("Failed to get database stats for metrics", zap.Error(err))
		return
	}

	metrics.SystemsRegistered.Set(float64(stats.SystemCount))
	metrics.TalkgroupsRegistered.Set(float64(stats.TalkgroupCount))
	metrics.UnitsTracked.Set(float64(stats.UnitCount))
}

// WSClientCount returns the number of connected WebSocket clients
func (s *Server) WSClientCount() int {
	return s.hub.ClientCount()
}

func (s *Server) setupRoutes() {
	// Embedded UI routes
	s.setupUIRoutes()

	// Health check
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Prometheus metrics endpoint
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger documentation - set host dynamically via middleware
	s.router.GET("/swagger/*any", func(c *gin.Context) {
		docs.SwaggerInfo.Host = c.Request.Host
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			docs.SwaggerInfo.Schemes = []string{"https"}
		} else {
			docs.SwaggerInfo.Schemes = []string{"http"}
		}
		c.Next()
	}, ginSwagger.WrapHandler(swaggerFiles.Handler))

	// WebSocket endpoint (under /api to work with Vite proxy)
	s.router.GET("/api/ws", func(c *gin.Context) {
		ws.HandleWebSocket(s.hub, c.Writer, c.Request, s.logger)
	})

	// REST API
	api := s.router.Group("/api/v1")
	{
		// Create REST handler
		handler := rest.NewHandler(s.db, s.processor, s.logger, s.audioBasePath)

		// Systems
		api.GET("/systems", handler.ListSystems)
		api.GET("/systems/:id", handler.GetSystem)
		api.GET("/systems/:id/talkgroups", handler.ListSystemTalkgroups)

		// Talkgroups
		api.GET("/talkgroups", handler.ListTalkgroups)
		api.GET("/talkgroups/encryption-stats", handler.GetTalkgroupEncryptionStats)
		api.GET("/talkgroups/:id", handler.GetTalkgroup)
		api.GET("/talkgroups/:id/calls", handler.ListTalkgroupCalls)

		// Units
		api.GET("/units", handler.ListUnits)
		api.GET("/units/active", handler.ListActiveUnits)
		api.GET("/units/:id", handler.GetUnit)
		api.GET("/units/:id/events", handler.ListUnitEvents)
		api.GET("/units/:id/calls", handler.ListUnitCalls)

		// Calls
		api.GET("/calls", handler.ListCalls)
		api.GET("/calls/active", handler.ListActiveCalls)
		api.GET("/calls/active/realtime", handler.GetActiveCallsRealtime)
		api.GET("/calls/recent", handler.GetRecentCalls)
		api.GET("/calls/:id", handler.GetCall)
		api.GET("/calls/:id/audio", handler.GetCallAudio)
		api.GET("/calls/:id/transmissions", handler.GetCallTransmissions)
		api.GET("/calls/:id/frequencies", handler.GetCallFrequencies)

		// Call groups
		api.GET("/call-groups", handler.ListCallGroups)
		api.GET("/call-groups/:id", handler.GetCallGroup)

		// Recorders
		api.GET("/recorders", handler.ListRecorders)

		// Stats
		api.GET("/stats", handler.GetStats)
		api.GET("/stats/rates", handler.GetRates)
		api.GET("/stats/activity", handler.GetActivity)
	}
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		statusStr := strconv.Itoa(status)

		// Skip metrics endpoint to avoid recursion
		if path != "/metrics" {
			metrics.HTTPRequestDuration.WithLabelValues(method, path, statusStr).Observe(latency.Seconds())
			metrics.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
		}

		logger.Debug("HTTP request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
		)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Content-Length, X-Requested-With, Origin, Cache-Control, Pragma")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// setupUIRoutes configures routes for the embedded dashboard UI
func (s *Server) setupUIRoutes() {
	staticFS, err := ui.StaticFiles()
	if err != nil {
		s.logger.Error("Failed to load embedded UI files", zap.Error(err))
		return
	}

	// Serve individual HTML files
	s.router.GET("/dashboard", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "dashboard.html")
	})
	s.router.GET("/dashboard.html", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "dashboard.html")
	})

	s.router.GET("/recorders", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "recorders.html")
	})
	s.router.GET("/recorders.html", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "recorders.html")
	})

	s.router.GET("/websocket", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "websocket.html")
	})
	s.router.GET("/websocket.html", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "websocket.html")
	})

	// Root serves landing page
	s.router.GET("/", func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "index.html")
	})
}

// serveEmbeddedFile serves a file from the embedded filesystem
func serveEmbeddedFile(c *gin.Context, fsys fs.FS, filename string) {
	data, err := fs.ReadFile(fsys, filename)
	if err != nil {
		c.String(http.StatusNotFound, "File not found")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
