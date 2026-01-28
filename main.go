package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/api"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/console"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/dedup"
	embeddeddocs "github.com/trunk-recorder/tr-engine/internal/docs"
	"github.com/trunk-recorder/tr-engine/internal/embeddedmqtt"
	"github.com/trunk-recorder/tr-engine/internal/embeddedpg"
	"github.com/trunk-recorder/tr-engine/internal/importer"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"github.com/trunk-recorder/tr-engine/internal/mqtt"
	"github.com/trunk-recorder/tr-engine/internal/storage"
	"go.uber.org/zap"
)

var (
	configPath     = flag.String("config", "config.yaml", "Path to configuration file")
	migrateCmd     = flag.String("migrate", "", "Run migrations: up, down, or version")
	showVersion    = flag.Bool("version", false, "Show version information")
	quietMode      = flag.Bool("quiet", false, "Suppress status output")
	noColor        = flag.Bool("no-color", false, "Disable colored output")
	importPath     = flag.String("import", "", "Import historical audio from trunk-recorder directory")
	importBatch    = flag.Int("batch-size", 1000, "Batch size for import operations")
	importThrottle = flag.Int("throttle", 0, "Max calls per second during import (0 = unlimited)")
)

const version = "0.1.1-beta1"

// @title           tr-engine API
// @version         0.1.1-beta1
// @description     Backend service for trunk-recorder data ingestion and querying. Provides REST APIs for accessing radio system data, calls, talkgroups, and units.

// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http

// @tag.name        systems
// @tag.description Radio system operations
// @tag.name        talkgroups
// @tag.description Talkgroup operations
// @tag.name        units
// @tag.description Radio unit operations
// @tag.name        calls
// @tag.description Call recording operations
// @tag.name        call-groups
// @tag.description Deduplicated call group operations
// @tag.name        stats
// @tag.description Statistics and activity operations

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("tr-engine version %s\n", version)
		os.Exit(0)
	}

	// Initialize console
	con := console.New(*quietMode, *noColor)

	// Print banner
	con.PrintBanner(version)

	// Create default config and README if config doesn't exist
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		if err := createDefaultConfig(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create default config: %v\n", err)
			os.Exit(1)
		}
		// Write README.md in the same directory as config
		readmePath := filepath.Join(filepath.Dir(*configPath), "README.md")
		if *configPath == "config.yaml" {
			readmePath = "README.md"
		}
		if err := os.WriteFile(readmePath, embeddeddocs.Readme, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create README.md: %v\n", err)
		}

		fmt.Printf("\nCreated %s with self-contained defaults.\n", *configPath)
		fmt.Printf("Created %s with documentation.\n", readmePath)
		fmt.Println()
		fmt.Println("Embedded PostgreSQL and MQTT broker enabled - no external services required.")
		fmt.Println()
		fmt.Println("Configure trunk-recorder to send MQTT to: tcp://<this-host>:1883")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Run tr-engine again to start the server")
		fmt.Println("  2. Open http://<this-host>:8080/ in your browser")
		fmt.Println("  3. See README.md for full documentation")
		fmt.Println()
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := initLogger(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Initialize embedded PostgreSQL if configured
	var embeddedServer *embeddedpg.Server
	if cfg.Database.Embedded {
		embeddedDone := con.StartTask("Starting embedded PostgreSQL")
		var err error
		embeddedServer, err = embeddedpg.New(cfg.Database, logger)
		if err != nil {
			embeddedDone(false, err.Error())
			os.Exit(1)
		}
		defer embeddedServer.Stop()
		cfg.Database = embeddedServer.GetConfig()
		embeddedDone(true, fmt.Sprintf("listening on :%d", cfg.Database.Port))
	}

	// Initialize database
	var dbTaskName string
	if cfg.Database.Embedded {
		dbTaskName = "Connecting to embedded PostgreSQL"
	} else {
		dbTaskName = "Connecting to PostgreSQL"
	}
	dbDone := con.StartTask(dbTaskName)
	db, err := database.New(cfg.Database, logger)
	if err != nil {
		dbDone(false, err.Error())
		os.Exit(1)
	}
	defer db.Close()

	// Initialize embedded schema if needed (no TimescaleDB)
	if cfg.Database.Embedded {
		needsInit, err := embeddedpg.NeedsInit(context.Background(), db.Pool())
		if err != nil {
			dbDone(false, fmt.Sprintf("failed to check schema: %s", err.Error()))
			os.Exit(1)
		}
		if needsInit {
			if err := embeddedpg.InitSchema(context.Background(), db.Pool()); err != nil {
				dbDone(false, fmt.Sprintf("failed to initialize schema: %s", err.Error()))
				os.Exit(1)
			}
			logger.Info("Initialized embedded database schema")
		}
	}

	// Get initial stats for display
	stats, _ := db.GetStats(context.Background())
	if stats != nil {
		dbDone(true, fmt.Sprintf("connected (%d systems, %d calls)", stats.SystemCount, stats.TotalCalls))
	} else {
		dbDone(true, "connected")
	}

	// Handle migrations if requested (only for external PostgreSQL)
	if *migrateCmd != "" {
		if cfg.Database.Embedded {
			logger.Warn("Migration commands are not supported in embedded mode")
			return
		}
		if err := runMigrations(db, *migrateCmd, logger); err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}
		return
	}

	// Run migrations on startup (skip for embedded - schema already initialized above)
	if !cfg.Database.Embedded {
		migrateDone := con.StartTask("Running database migrations")
		if err := db.MigrateUp(); err != nil {
			migrateDone(false, err.Error())
			os.Exit(1)
		}
		migrateDone(true, "")
	}

	// Handle import mode
	if *importPath != "" {
		runImport(db, *importPath, *importBatch, *importThrottle, logger)
		return
	}

	// Initialize storage manager
	storageDone := con.StartTask("Initializing audio storage")
	audioStorage := storage.NewAudioStorage(cfg.Storage.AudioPath, cfg.Storage.Mode, logger)
	// Verify the directory is writable (only for copy mode)
	if cfg.Storage.Mode != "external" {
		if err := verifyWritable(cfg.Storage.AudioPath); err != nil {
			storageDone(false, fmt.Sprintf("cannot write to %s: %v", cfg.Storage.AudioPath, err))
			os.Exit(1)
		}
	}
	storageDone(true, fmt.Sprintf("%s (mode: %s)", cfg.Storage.AudioPath, cfg.Storage.Mode))

	// Initialize deduplication engine
	dedupEngine := dedup.NewEngine(db, cfg.Deduplication, logger)

	// Initialize ingest processor
	processor := ingest.NewProcessor(db, audioStorage, dedupEngine, logger)

	// Initialize embedded MQTT broker if configured
	var mqttBroker *embeddedmqtt.Broker
	if cfg.MQTT.Embedded {
		brokerDone := con.StartTask("Starting embedded MQTT broker")
		var err error
		mqttBroker, err = embeddedmqtt.New(embeddedmqtt.Config{
			Port:     cfg.MQTT.EmbeddedPort,
			Username: cfg.MQTT.Username,
			Password: cfg.MQTT.Password,
		}, logger)
		if err != nil {
			brokerDone(false, err.Error())
			os.Exit(1)
		}
		defer mqttBroker.Stop()
		// Update broker address to point to embedded broker
		cfg.MQTT.Broker = fmt.Sprintf("tcp://localhost:%d", mqttBroker.Port())
		brokerDone(true, fmt.Sprintf("listening on :%d", mqttBroker.Port()))
	}

	// Initialize MQTT client
	var mqttTaskName string
	if cfg.MQTT.Embedded {
		mqttTaskName = "Connecting to embedded MQTT broker"
	} else {
		mqttTaskName = "Connecting to MQTT broker"
	}
	mqttDone := con.StartTask(mqttTaskName)
	mqttClient, err := mqtt.NewClient(cfg.MQTT, processor, logger)
	if err != nil {
		mqttDone(false, err.Error())
		os.Exit(1)
	}

	// Initialize API server
	httpDone := con.StartTask("Starting HTTP server")
	server := api.NewServer(cfg.Server, db, processor, logger, cfg.Storage.AudioPath)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start MQTT client
	if err := mqttClient.Connect(ctx); err != nil {
		mqttDone(false, err.Error())
		os.Exit(1)
	}
	mqttDone(true, fmt.Sprintf("connected (%s)", cfg.MQTT.Broker))

	// Print MQTT topics
	topics := []string{
		cfg.MQTT.Topics.Status,
		cfg.MQTT.Topics.Units,
	}
	if cfg.MQTT.Topics.Messages != "" {
		topics = append(topics, cfg.MQTT.Topics.Messages)
	}
	con.PrintTopics(topics)

	// Start API server
	go func() {
		if err := server.Start(); err != nil {
			logger.Error("API server error", zap.Error(err))
			cancel()
		}
	}()
	httpDone(true, fmt.Sprintf("listening on :%d", cfg.Server.Port))

	// Print ready message
	con.PrintReady()

	// Start status loop
	statusProvider := &statsProvider{db: db, server: server}
	con.StartStatusLoop(ctx, statusProvider, 30*time.Second)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	con.PrintShutdown()

	cancel()

	// Graceful shutdown
	mqttClient.Disconnect()
	processor.Stop()
	server.Shutdown(context.Background())

	con.PrintShutdownComplete()
}

// statsProvider implements console.StatusProvider
type statsProvider struct {
	db     *database.DB
	server *api.Server
}

func (p *statsProvider) GetStats(ctx context.Context) (console.StatusLine, error) {
	stats, err := p.db.GetStats(ctx)
	if err != nil {
		return console.StatusLine{}, err
	}

	return console.StatusLine{
		Systems:     stats.SystemCount,
		CallsPerMin: float64(stats.CallsLastMinute),
		ActiveUnits: stats.ActiveUnits,
		WSClients:   p.server.WSClientCount(),
	}, nil
}

func initLogger(level, format string) (*zap.Logger, error) {
	var cfg zap.Config

	if format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return cfg.Build()
}

func runMigrations(db *database.DB, cmd string, logger *zap.Logger) error {
	switch cmd {
	case "up":
		logger.Info("Running migrations up")
		return db.MigrateUp()
	case "down":
		logger.Info("Running migrations down")
		return db.MigrateDown()
	case "version":
		ver, dirty, err := db.MigrationVersion()
		if err != nil {
			return err
		}
		logger.Info("Migration version", zap.Uint("version", ver), zap.Bool("dirty", dirty))
		return nil
	default:
		return fmt.Errorf("unknown migration command: %s", cmd)
	}
}

// createDefaultConfig creates a default config file for embedded mode
func createDefaultConfig(path string) error {
	defaultConfig := `# tr-engine configuration
# Generated automatically - edit to customize

server:
  host: "0.0.0.0"
  port: 8080

database:
  # Embedded PostgreSQL (self-contained, no external DB required)
  embedded: true
  embedded_data_path: "./data/postgres"
  port: 5432
  name: "tr_engine"
  user: "tr_engine"
  password: "tr_engine"
  max_connections: 50

mqtt:
  # Embedded MQTT broker (self-contained, no external broker required)
  embedded: true
  embedded_port: 1883
  # trunk-recorder should connect to this address to send messages
  broker: "tcp://localhost:1883"
  client_id: "tr-engine"
  username: ""
  password: ""
  topics:
    # These match trunk-recorder MQTT plugin defaults
    status: "feeds/#"
    units: "units/#"
    # messages: "messages/#"  # Optional trunking messages, high volume
  qos: 1

storage:
  audio_path: "./data/audio"

deduplication:
  enabled: true
  time_window_seconds: 3
  threshold: 0.7

logging:
  level: "info"
  format: "json"
`
	return os.WriteFile(path, []byte(defaultConfig), 0644)
}

// verifyWritable checks if a directory is writable by creating a temp file
func verifyWritable(path string) error {
	testFile := filepath.Join(path, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return err
	}
	return os.Remove(testFile)
}

// runImport handles the --import mode for bulk importing historical audio
func runImport(db *database.DB, audioPath string, batchSize, throttle int, logger *zap.Logger) {
	fmt.Println()
	fmt.Println("=== tr-engine Historical Import ===")
	fmt.Printf("Audio path:   %s\n", audioPath)
	fmt.Printf("Batch size:   %d\n", batchSize)
	if throttle > 0 {
		fmt.Printf("Throttle:     %d calls/sec\n", throttle)
	} else {
		fmt.Printf("Throttle:     unlimited\n")
	}
	fmt.Printf("Checkpoint:   .tr-engine-import-checkpoint (in current directory)\n")
	fmt.Println()

	// Verify the path exists
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: audio path does not exist: %s\n", audioPath)
		os.Exit(1)
	}

	// Create importer
	imp := importer.New(db, importer.Config{
		AudioPath: audioPath,
		BatchSize: batchSize,
		Throttle:  throttle,
	}, logger)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nInterrupted - saving checkpoint and exiting...")
		cancel()
	}()

	// Run import
	if err := imp.Run(ctx); err != nil {
		if ctx.Err() != nil {
			fmt.Println("Import interrupted. Run again to resume from checkpoint.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Import failed: %v\n", err)
		os.Exit(1)
	}
}
