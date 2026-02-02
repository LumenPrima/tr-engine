package api

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/trunk-recorder/tr-engine/docs"
	"github.com/trunk-recorder/tr-engine/internal/api/middleware"
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
	authConfig    config.AuthConfig
	db            *database.DB
	processor     *ingest.Processor
	logger        *zap.Logger
	server        *http.Server
	hub           *ws.Hub
	router        *gin.Engine
	auth          *middleware.AuthMiddleware
	stopMetrics   chan struct{}
	audioBasePath string
	handler       *rest.Handler
}

// NewServer creates a new API server
func NewServer(cfg config.ServerConfig, authCfg config.AuthConfig, db *database.DB, processor *ingest.Processor, logger *zap.Logger, audioBasePath string) *Server {
	// Set gin mode based on log level
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))
	router.Use(corsMiddleware())

	// Load login template
	loginTmpl, err := loadLoginTemplate()
	if err != nil {
		logger.Warn("Failed to load login template", zap.Error(err))
	} else {
		router.SetHTMLTemplate(loginTmpl)
	}

	hub := ws.NewHub(logger)
	go hub.Run()

	// Connect processor to hub for broadcasting (if processor exists)
	if processor != nil {
		processor.SetHub(hub)
	}

	// Create auth middleware
	auth := middleware.NewAuthMiddleware(authCfg, db)

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			auth.CleanupSessions()
		}
	}()

	s := &Server{
		config:        cfg,
		authConfig:    authCfg,
		db:            db,
		processor:     processor,
		logger:        logger,
		hub:           hub,
		router:        router,
		auth:          auth,
		stopMetrics:   make(chan struct{}),
		audioBasePath: audioBasePath,
	}

	s.setupRoutes()

	// Start metrics gauge updater
	go s.updateMetricsGauges()

	return s
}

// loadLoginTemplate loads the login page template from embedded files
func loadLoginTemplate() (*template.Template, error) {
	staticFS, err := ui.StaticFiles()
	if err != nil {
		return nil, err
	}

	loginHTML, err := fs.ReadFile(staticFS, "login.html")
	if err != nil {
		return nil, err
	}

	return template.New("login.html").Parse(string(loginHTML))
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

// GetHub returns the WebSocket hub for external use
func (s *Server) GetHub() *ws.Hub {
	return s.hub
}

// SetRecorderProvider sets the recorder provider for watch mode
func (s *Server) SetRecorderProvider(provider rest.RecorderProvider) {
	if s.handler != nil {
		s.handler.SetRecorderProvider(provider)
	}
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
	// Auth routes (always accessible)
	s.router.GET("/login", s.auth.LoginPage)
	s.router.POST("/login", s.auth.Login)
	s.router.GET("/logout", s.auth.Logout)

	// Embedded UI routes (protected by dashboard auth)
	s.setupUIRoutes()

	// Health check (always accessible)
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Prometheus metrics endpoint (always accessible for monitoring)
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
	// Auth handled by cookie (dashboard users) or skip if auth disabled
	s.router.GET("/api/ws", func(c *gin.Context) {
		ws.HandleWebSocket(s.hub, c.Writer, c.Request, s.logger)
	})

	// REST API (protected by API key auth)
	api := s.router.Group("/api/v1")
	api.Use(s.auth.APIKeyAuth())
	{
		// Create REST handler
		s.handler = rest.NewHandler(s.db, s.processor, s.logger, s.audioBasePath)
		handler := s.handler

		// Systems
		api.GET("/systems", handler.ListSystems)
		api.GET("/systems/:id", handler.GetSystem)
		api.GET("/systems/:id/talkgroups", handler.ListSystemTalkgroups)

		// P25 Systems (grouped by sysid+wacn)
		api.GET("/p25-systems", handler.ListP25Systems)

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

		// Transcription
		api.GET("/calls/:id/transcription", handler.GetCallTranscription)
		api.POST("/calls/:id/transcribe", handler.QueueCallTranscription)
		api.GET("/transcriptions/recent", handler.GetRecentTranscriptions)
		api.GET("/transcriptions/search", handler.SearchTranscriptions)
		api.GET("/transcription/status", handler.GetTranscriptionStatus)

		// Admin - API Keys
		api.GET("/admin/api-keys", s.listAPIKeys)
		api.POST("/admin/api-keys", s.createAPIKey)
		api.DELETE("/admin/api-keys/:id", s.revokeAPIKey)
	}
}

// createAPIKey creates a new API key
func (s *Server) createAPIKey(c *gin.Context) {
	var req struct {
		Name      string     `json:"name" binding:"required"`
		Scopes    []string   `json:"scopes"`
		ReadOnly  bool       `json:"read_only"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plaintext, key, err := s.auth.CreateAPIKey(c.Request.Context(), req.Name, req.Scopes, req.ReadOnly, req.ExpiresAt)
	if err != nil {
		s.logger.Error("Failed to create API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create API key"})
		return
	}

	// Return the plaintext key only once
	c.JSON(http.StatusCreated, gin.H{
		"key":        plaintext,
		"id":         key.ID,
		"key_prefix": key.KeyPrefix,
		"name":       key.Name,
		"created_at": key.CreatedAt,
		"expires_at": key.ExpiresAt,
		"message":    "Save this key now - it cannot be retrieved again",
	})
}

// listAPIKeys returns all API keys (without the actual key values)
func (s *Server) listAPIKeys(c *gin.Context) {
	keys, err := s.auth.ListAPIKeys(c.Request.Context())
	if err != nil {
		s.logger.Error("Failed to list API keys", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list API keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keys,
		"count":    len(keys),
	})
}

// revokeAPIKey revokes an API key
func (s *Server) revokeAPIKey(c *gin.Context) {
	idStr := c.Param("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key ID"})
		return
	}

	if err := s.auth.RevokeAPIKey(c.Request.Context(), id); err != nil {
		s.logger.Error("Failed to revoke API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke API key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked"})
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

	// Dashboard auth middleware
	dashAuth := s.auth.DashboardAuth()

	// Serve individual HTML files (protected)
	s.router.GET("/dashboard", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "dashboard.html")
	})
	s.router.GET("/dashboard.html", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "dashboard.html")
	})

	s.router.GET("/recorders", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "recorders.html")
	})
	s.router.GET("/recorders.html", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "recorders.html")
	})

	s.router.GET("/websocket", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "websocket.html")
	})
	s.router.GET("/websocket.html", dashAuth, func(c *gin.Context) {
		serveEmbeddedFile(c, staticFS, "websocket.html")
	})

	// Root serves landing page (protected)
	s.router.GET("/", dashAuth, func(c *gin.Context) {
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
