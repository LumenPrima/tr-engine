package api

import (
	"context"
	"net/http"
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
	Config    *config.Config
	DB        *database.DB
	MQTT      *mqttclient.Client
	Live      LiveDataSource
	Version   string
	StartTime time.Time
	Log       zerolog.Logger
}

func NewServer(opts ServerOptions) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(RequestID)
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
			NewAdminHandler(opts.DB).Routes(r)
			NewQueryHandler(opts.DB).Routes(r)
		})
	})

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
