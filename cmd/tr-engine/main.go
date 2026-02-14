package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/api"
	"github.com/snarg/tr-engine/internal/config"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/mqttclient"
)

var version = "dev"

func main() {
	startTime := time.Now()

	// Config
	cfg, err := config.Load()
	if err != nil {
		early := zerolog.New(os.Stderr).With().Timestamp().Logger()
		early.Fatal().Err(err).Msg("failed to load config")
	}

	// Logger
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	log := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(level)
	log.Info().Str("version", version).Msg("tr-engine starting")

	// Context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Database
	dbLog := log.With().Str("component", "database").Logger()
	db, err := database.Connect(ctx, cfg.DatabaseURL, dbLog)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// MQTT
	mqttLog := log.With().Str("component", "mqtt").Logger()
	mqtt, err := mqttclient.Connect(cfg.MQTTBrokerURL, cfg.MQTTClientID, cfg.MQTTTopics, mqttLog)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to mqtt broker")
	}
	defer mqtt.Close()

	// HTTP Server
	httpLog := log.With().Str("component", "http").Logger()
	srv := api.NewServer(cfg, db, mqtt, version, startTime, httpLog)

	// Start HTTP server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Msg("http server error")
		}
	}

	// Graceful shutdown with 10s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	}

	log.Info().Msg("tr-engine stopped")
}
