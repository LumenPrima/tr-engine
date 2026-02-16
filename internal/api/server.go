package api

import (
	"context"
	"io/fs"
	"net/http"
	"os"
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
	Config      *config.Config
	DB          *database.DB
	MQTT        *mqttclient.Client
	Live        LiveDataSource
	WebFiles    fs.FS  // embedded web/ directory
	OpenAPISpec []byte // embedded openapi.yaml
	Version     string
	StartTime   time.Time
	Log         zerolog.Logger
}

func NewServer(opts ServerOptions) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(RequestID)
	r.Use(CORS)
	r.Use(Recoverer)
	r.Use(Logger(opts.Log))

	// Health endpoint — no auth
	health := NewHealthHandler(opts.DB, opts.MQTT, opts.Live, opts.Version, opts.StartTime)
	r.Get("/api/v1/health", health.ServeHTTP)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(opts.Config.AuthToken))

		// All API routes under /api/v1
		r.Route("/api/v1", func(r chi.Router) {
			NewSystemsHandler(opts.DB).Routes(r)
			NewTalkgroupsHandler(opts.DB).Routes(r)
			NewUnitsHandler(opts.DB).Routes(r)
			NewCallsHandler(opts.DB, opts.Config.AudioDir, opts.Live).Routes(r)
			NewCallGroupsHandler(opts.DB).Routes(r)
			NewStatsHandler(opts.DB).Routes(r)
			NewRecordersHandler(opts.Live).Routes(r)
			NewEventsHandler(opts.Live).Routes(r)
			NewUnitEventsHandler(opts.DB).Routes(r)
			NewAffiliationsHandler(opts.Live).Routes(r)
			NewAdminHandler(opts.DB).Routes(r)
			NewQueryHandler(opts.DB).Routes(r)
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
