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
	"github.com/snarg/tr-engine/internal/transcribe"
	"github.com/snarg/tr-engine/internal/trconfig"
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
	flag.StringVar(&overrides.WatchDir, "watch-dir", "", "Watch TR audio directory for new files (overrides WATCH_DIR)")
	flag.StringVar(&overrides.TRDir, "tr-dir", "", "Path to trunk-recorder directory for auto-discovery (overrides TR_DIR)")
	flag.StringVar(&overrides.WhisperURL, "whisper-url", "", "Whisper API URL for transcription (overrides WHISPER_URL)")
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
	// TR auto-discovery: read trunk-recorder's config.json + docker-compose.yaml
	var discovered *trconfig.DiscoveryResult
	if cfg.TRDir != "" {
		earlyLog := zerolog.New(os.Stdout).With().Timestamp().Logger()
		discovered, err = trconfig.Discover(cfg.TRDir, earlyLog)
		if err != nil {
			earlyLog.Fatal().Err(err).Str("tr_dir", cfg.TRDir).Msg("failed to read trunk-recorder config")
		}
		// Auto-set WatchDir and TRAudioDir if not explicitly configured
		if cfg.WatchDir == "" {
			cfg.WatchDir = discovered.CaptureDir
		}
		if cfg.TRAudioDir == "" {
			cfg.TRAudioDir = discovered.CaptureDir
		}
	}

	if err := cfg.Validate(); err != nil {
		early := zerolog.New(os.Stderr).With().Timestamp().Logger()
		early.Fatal().Err(err).Msg("invalid config")
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

	// MQTT (optional — not needed when using watch mode)
	var mqtt *mqttclient.Client
	if cfg.MQTTBrokerURL != "" {
		mqttLog := log.With().Str("component", "mqtt").Logger()
		mqtt, err = mqttclient.Connect(mqttclient.Options{
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
	} else {
		log.Info().Msg("mqtt not configured (watch-only mode)")
	}

	// Transcription (optional — build provider based on STT_PROVIDER)
	var transcribeOpts *transcribe.WorkerPoolOptions
	var sttProvider transcribe.Provider
	switch cfg.STTProvider {
	case "whisper":
		if cfg.WhisperURL != "" {
			sttProvider = transcribe.NewWhisperClient(cfg.WhisperURL, cfg.WhisperModel, cfg.WhisperTimeout)
		}
	case "elevenlabs":
		if cfg.ElevenLabsAPIKey == "" {
			log.Fatal().Msg("STT_PROVIDER=elevenlabs requires ELEVENLABS_API_KEY")
		}
		sttProvider = transcribe.NewElevenLabsClient(cfg.ElevenLabsAPIKey, cfg.ElevenLabsModel, cfg.ElevenLabsKeyterms, cfg.WhisperTimeout)
	case "none", "":
		// Transcription explicitly disabled
	default:
		log.Fatal().Str("provider", cfg.STTProvider).Msg("unknown STT_PROVIDER (valid: whisper, elevenlabs, none)")
	}

	if sttProvider != nil {
		transcribeOpts = &transcribe.WorkerPoolOptions{
			DB:              db,
			AudioDir:        cfg.AudioDir,
			TRAudioDir:      cfg.TRAudioDir,
			Provider:        sttProvider,
			ProviderTimeout: cfg.WhisperTimeout,
			Temperature:     cfg.WhisperTemperature,
			Language:        cfg.WhisperLanguage,
			Prompt:          cfg.WhisperPrompt,
			Hotwords:        cfg.WhisperHotwords,
			BeamSize:        cfg.WhisperBeamSize,
			PreprocessAudio: cfg.PreprocessAudio,
			Workers:         cfg.TranscribeWorkers,
			QueueSize:       cfg.TranscribeQueueSize,
			MinDuration:     cfg.TranscribeMinDuration,
			MaxDuration:     cfg.TranscribeMaxDuration,
			Log:             log.With().Str("component", "transcribe").Logger(),

			RepetitionPenalty:             cfg.WhisperRepetitionPenalty,
			NoRepeatNgramSize:             cfg.WhisperNoRepeatNgram,
			ConditionOnPreviousText:       cfg.WhisperConditionOnPrev,
			NoSpeechThreshold:             cfg.WhisperNoSpeechThreshold,
			HallucinationSilenceThreshold: cfg.WhisperHallucinationThreshold,
			MaxNewTokens:                  cfg.WhisperMaxTokens,
			VadFilter:                     cfg.WhisperVadFilter,
		}
		log.Info().
			Str("provider", sttProvider.Name()).
			Str("model", sttProvider.Model()).
			Int("workers", cfg.TranscribeWorkers).
			Msg("transcription enabled")
	}

	// Ingest Pipeline
	pipeline := ingest.NewPipeline(ingest.PipelineOptions{
		DB:               db,
		AudioDir:         cfg.AudioDir,
		TRAudioDir:       cfg.TRAudioDir,
		RawStore:         cfg.RawStore,
		RawIncludeTopics: cfg.RawIncludeTopics,
		RawExcludeTopics: cfg.RawExcludeTopics,
		TranscribeOpts:   transcribeOpts,
		Log:              log,
	})
	if err := pipeline.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start ingest pipeline")
	}
	defer pipeline.Stop()

	// Wire MQTT → Pipeline
	if mqtt != nil {
		mqtt.SetMessageHandler(pipeline.HandleMessage)
	}

	// Import talkgroup directory from TR's CSV files (if TR_DIR discovery found any)
	if discovered != nil {
		for _, sys := range discovered.Systems {
			if len(sys.Talkgroups) == 0 {
				continue
			}
			identity, idErr := pipeline.ResolveIdentity(ctx, cfg.WatchInstanceID, sys.ShortName)
			if idErr != nil {
				log.Warn().Err(idErr).Str("system", sys.ShortName).Msg("failed to resolve system for talkgroup import")
				continue
			}
			imported := 0
			for _, tg := range sys.Talkgroups {
				if uErr := db.UpsertTalkgroupDirectory(ctx, identity.SystemID, tg.Tgid,
					tg.AlphaTag, tg.Mode, tg.Description, tg.Tag, tg.Category, tg.Priority,
				); uErr != nil {
					log.Warn().Err(uErr).Int("tgid", tg.Tgid).Msg("failed to import talkgroup")
					continue
				}
				imported++
			}
			log.Info().
				Str("system", sys.ShortName).
				Int("imported", imported).
				Int("total", len(sys.Talkgroups)).
				Msg("talkgroup directory imported")
		}
	}

	// File watcher (optional — alternative to MQTT ingest)
	if cfg.WatchDir != "" {
		if err := pipeline.StartWatcher(cfg.WatchDir, cfg.WatchInstanceID, cfg.WatchBackfillDays); err != nil {
			log.Fatal().Err(err).Msg("failed to start file watcher")
		}
		log.Info().Str("watch_dir", cfg.WatchDir).Str("instance_id", cfg.WatchInstanceID).Msg("file watcher started")
	}

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
