package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	trengine "github.com/snarg/tr-engine"
	"github.com/snarg/tr-engine/internal/api"
	"github.com/snarg/tr-engine/internal/config"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/ingest"
	"github.com/snarg/tr-engine/internal/mqttclient"
)

// version, commit, and buildTime are injected at build time via ldflags.
// See Makefile or build script for usage.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	// CLI flags
	var overrides config.Overrides
	var showVersion bool
	flag.StringVar(&overrides.EnvFile, "env-file", "", "Path to .env file (default: .env)")
	flag.StringVar(&overrides.HTTPAddr, "listen", "", "HTTP listen address (overrides HTTP_ADDR)")
	flag.StringVar(&overrides.LogLevel, "log-level", "", "Log level: debug, info, warn, error (overrides LOG_LEVEL)")
	flag.StringVar(&overrides.DatabaseURL, "database-url", "", "PostgreSQL connection URL (overrides DATABASE_URL)")
	flag.StringVar(&overrides.MQTTBrokerURL, "mqtt-url", "", "MQTT broker URL (overrides MQTT_BROKER_URL)")
	flag.StringVar(&overrides.AudioDir, "audio-dir", "", "Audio file directory (overrides AUDIO_DIR)")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("%s (commit=%s, built=%s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	startTime := time.Now()

	// Config (loads .env automatically, then env vars, then CLI overrides)
	cfg, err := config.Load(overrides)
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
	log.Info().
		Str("version", version).
		Str("commit", commit).
		Str("built", buildTime).
		Str("log_level", level.String()).
		Msg("tr-engine starting")

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
	mqtt, err := mqttclient.Connect(mqttclient.Options{
		BrokerURL: cfg.MQTTBrokerURL,
		ClientID:  cfg.MQTTClientID,
		Topics:    cfg.MQTTTopics,
		Username:  cfg.MQTTUsername,
		Password:  cfg.MQTTPassword,
		Log:       mqttLog,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to mqtt broker")
	}
	defer mqtt.Close()
	log.Info().Str("broker", cfg.MQTTBrokerURL).Str("client_id", cfg.MQTTClientID).Msg("mqtt connected")

	// Ingest Pipeline
	pipeline := ingest.NewPipeline(ingest.PipelineOptions{
		DB:               db,
		AudioDir:         cfg.AudioDir,
		RawStore:         cfg.RawStore,
		RawIncludeTopics: cfg.RawIncludeTopics,
		RawExcludeTopics: cfg.RawExcludeTopics,
		Log:              log,
	})
	if err := pipeline.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start ingest pipeline")
	}
	defer pipeline.Stop()

	// Wire MQTT → Pipeline
	mqtt.SetMessageHandler(pipeline.HandleMessage)

	// HTTP Server
	httpLog := log.With().Str("component", "http").Logger()
	srv := api.NewServer(api.ServerOptions{
		Config:      cfg,
		DB:          db,
		MQTT:        mqtt,
		Live:        pipeline,
		WebFiles:    trengine.WebFiles,
		OpenAPISpec: trengine.OpenAPISpec,
		Version:     fmt.Sprintf("%s (commit=%s, built=%s)", version, commit, buildTime),
		StartTime:   startTime,
		Log:         httpLog,
	})

	// Start HTTP server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	log.Info().
		Str("listen", cfg.HTTPAddr).
		Str("version", version).
		Dur("startup_ms", time.Since(startTime)).
		Msg("tr-engine ready")

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
