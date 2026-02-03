// Package embeddedpg provides embedded PostgreSQL for self-contained deployments.
package embeddedpg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"go.uber.org/zap"
)

// Server manages an embedded PostgreSQL instance
type Server struct {
	postgres *embeddedpostgres.EmbeddedPostgres
	dataDir  string
	config   config.DatabaseConfig
	logger   *zap.Logger
}

// New creates and starts a new embedded PostgreSQL server.
// The data directory will be created if it doesn't exist.
// PostgreSQL binaries are downloaded on first run (~10MB).
func New(cfg config.DatabaseConfig, logger *zap.Logger) (*Server, error) {
	dataDir := cfg.EmbeddedDataPath
	if dataDir == "" {
		dataDir = "./data/postgres"
	}

	// Convert to absolute path
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data path: %w", err)
	}

	// Create data directory
	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Use defaults for embedded mode
	port := uint32(cfg.Port)
	if port == 0 {
		port = 5432
	}
	dbName := cfg.Name
	if dbName == "" {
		dbName = "tr_engine"
	}
	user := cfg.User
	if user == "" {
		user = "tr_engine"
	}
	password := cfg.Password
	if password == "" {
		password = "tr_engine"
	}

	logger.Info("Starting embedded PostgreSQL",
		zap.String("data_dir", absDataDir),
		zap.Uint32("port", port),
		zap.String("database", dbName),
	)

	// Configure embedded postgres
	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(port).
			DataPath(filepath.Join(absDataDir, "data")).
			RuntimePath(filepath.Join(absDataDir, "runtime")).
			BinariesPath(filepath.Join(absDataDir, "bin")).
			Username(user).
			Password(password).
			Database(dbName).
			StartTimeout(120*time.Second), // Allow time for first-run binary download
	)

	// Start PostgreSQL
	if err := pg.Start(); err != nil {
		return nil, fmt.Errorf("failed to start embedded postgres: %w", err)
	}

	logger.Info("Embedded PostgreSQL started successfully")

	return &Server{
		postgres: pg,
		dataDir:  absDataDir,
		config:   cfg,
		logger:   logger,
	}, nil
}

// Stop gracefully stops the embedded PostgreSQL server
func (s *Server) Stop() error {
	if s.postgres != nil {
		s.logger.Info("Stopping embedded PostgreSQL")
		return s.postgres.Stop()
	}
	return nil
}

// GetConfig returns the database config for connecting to embedded PostgreSQL
func (s *Server) GetConfig() config.DatabaseConfig {
	cfg := s.config
	cfg.Host = "localhost"
	cfg.SSLMode = "disable"
	return cfg
}

// InitSchema initializes the embedded database with the schema from migrations.
// This should be called on first run when the database is empty.
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema, err := database.Schema()
	if err != nil {
		return fmt.Errorf("failed to get schema: %w", err)
	}
	_, err = pool.Exec(ctx, schema)
	return err
}

// NeedsInit checks if the database needs schema initialization
func NeedsInit(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'schema_migrations'
		)
	`).Scan(&exists)
	if err != nil {
		return false, err
	}
	return !exists, nil
}
