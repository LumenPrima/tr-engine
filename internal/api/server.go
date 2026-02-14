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

func NewServer(cfg *config.Config, db *database.DB, mqtt *mqttclient.Client, version string, startTime time.Time, log zerolog.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(RequestID)
	r.Use(Recoverer)
	r.Use(Logger(log))

	// Health endpoint — no auth
	health := NewHealthHandler(db, mqtt, version, startTime)
	r.Get("/api/v1/health", health.ServeHTTP)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(cfg.AuthToken))
		// Future routes go here
	})

	return &Server{
		http: &http.Server{
			Addr:         cfg.HTTPAddr,
			Handler:      r,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		log: log,
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
