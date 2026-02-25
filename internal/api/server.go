package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/config"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/mqttclient"
)

type Server struct {
	http *http.Server
	log  zerolog.Logger
}

type ServerOptions struct {
	Config        *config.Config
	DB            *database.DB
	MQTT          *mqttclient.Client
	Live          LiveDataSource
	Uploader      CallUploader // nil if upload ingest not available
	WebFiles      fs.FS        // embedded web/ directory
	OpenAPISpec   []byte       // embedded openapi.yaml
	Version       string
	StartTime     time.Time
	Log           zerolog.Logger
	OnSystemMerge func(sourceID, targetID int) // called after successful system merge to invalidate caches
	TGCSVPaths    map[int]string               // system_id → CSV file path for talkgroup writeback
	UnitCSVPaths  map[int]string               // system_id → CSV file path for unit tag writeback
}

func NewServer(opts ServerOptions) *Server {
	r := chi.NewRouter()

	// Parse CORS origins from config
	var corsOrigins []string
	if opts.Config.CORSOrigins != "" {
		for _, o := range strings.Split(opts.Config.CORSOrigins, ",") {
			if s := strings.TrimSpace(o); s != "" {
				corsOrigins = append(corsOrigins, s)
			}
		}
	}

	// Global middleware (no MaxBodySize here — upload endpoint needs a larger limit)
	r.Use(RequestID)
	r.Use(CORSWithOrigins(corsOrigins))
	r.Use(RateLimiter(opts.Config.RateLimitRPS, opts.Config.RateLimitBurst))
	r.Use(Recoverer)
	r.Use(Logger(opts.Log))

	// Unauthenticated endpoints
	health := NewHealthHandler(opts.DB, opts.MQTT, opts.Live, opts.Version, opts.StartTime)
	r.Get("/api/v1/health", health.ServeHTTP)

	// Web auth bootstrap — returns the token for web UI pages.
	// No file extension in the URL so CDNs (Cloudflare) won't cache it.
	if opts.Config.AuthToken != "" {
		tokenJSON := fmt.Sprintf(`{"token":"%s"}`, strings.ReplaceAll(opts.Config.AuthToken, `"`, `\"`))
		r.Get("/api/v1/auth-init", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Write([]byte(tokenJSON))
		})
	}

	// Upload endpoint with custom auth (accepts form field key/api_key)
	if opts.Uploader != nil {
		uploadHandler := NewUploadHandler(opts.Uploader, opts.Config.UploadInstanceID, opts.Log)
		r.Group(func(r chi.Router) {
			r.Use(MaxBodySize(50 << 20)) // 50 MB for audio uploads
			r.Use(UploadAuth(opts.Config.AuthToken))
			r.Post("/api/v1/call-upload", uploadHandler.Upload)
		})
	}

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(MaxBodySize(10 << 20)) // 10 MB for regular API requests
		r.Use(BearerAuth(opts.Config.AuthToken))
		r.Use(ResponseTimeout(opts.Config.WriteTimeout))

		// All API routes under /api/v1
		r.Route("/api/v1", func(r chi.Router) {
			NewSystemsHandler(opts.DB).Routes(r)
			NewTalkgroupsHandler(opts.DB, opts.TGCSVPaths).Routes(r)
			NewUnitsHandler(opts.DB, opts.UnitCSVPaths).Routes(r)
			NewCallsHandler(opts.DB, opts.Config.AudioDir, opts.Config.TRAudioDir, opts.Live).Routes(r)
			NewCallGroupsHandler(opts.DB, opts.Config.TRAudioDir).Routes(r)
			NewStatsHandler(opts.DB).Routes(r)
			NewRecordersHandler(opts.Live).Routes(r)
			NewEventsHandler(opts.Live).Routes(r)
			NewUnitEventsHandler(opts.DB).Routes(r)
			NewAffiliationsHandler(opts.Live).Routes(r)
			NewTranscriptionsHandler(opts.DB, opts.Live).Routes(r)
			NewAdminHandler(opts.DB, opts.OnSystemMerge).Routes(r)

			// Query endpoint always requires auth — disabled when AUTH_TOKEN is empty
			r.Group(func(r chi.Router) {
				r.Use(RequireAuth(opts.Config.AuthToken))
				NewQueryHandler(opts.DB).Routes(r)
			})
		})
	})

	// Serve embedded OpenAPI spec
	r.Get("/api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.Write(opts.OpenAPISpec)
	})

	// Serve web files: prefer local web/ directory on disk for dev, fall back to embedded
	var webFSys fs.FS
	if info, err := os.Stat("web"); err == nil && info.IsDir() {
		webFSys = os.DirFS("web")
		opts.Log.Info().Msg("serving web files from disk (dev mode)")
	} else {
		webFSys, _ = fs.Sub(opts.WebFiles, "web")
	}
	r.Get("/api/v1/pages", PagesHandler(webFSys))
	r.Handle("/*", http.FileServer(http.FS(webFSys)))

	srv := &http.Server{
		Addr:        opts.Config.HTTPAddr,
		Handler:     r,
		ReadTimeout: opts.Config.ReadTimeout,
		IdleTimeout: opts.Config.IdleTimeout,
		// WriteTimeout set to 0 to allow long-lived SSE connections.
		// Individual non-streaming handlers complete quickly due to DB query timeouts.
		WriteTimeout: 0,
	}

	return &Server{
		http: srv,
		log:  opts.Log,
	}
}

func (s *Server) Start() error {
	s.log.Info().Str("addr", s.http.Addr).Msg("http server starting")
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info().Msg("http server shutting down")
	return s.http.Shutdown(ctx)
}

