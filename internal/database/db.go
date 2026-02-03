package database

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Schema returns the current schema SQL (for embedded PostgreSQL and tests)
func Schema() (string, error) {
	data, err := migrationsFS.ReadFile("migrations/001_initial_schema.up.sql")
	if err != nil {
		return "", fmt.Errorf("failed to read schema: %w", err)
	}
	return string(data), nil
}

// DB wraps the database connection pool
type DB struct {
	pool   *pgxpool.Pool
	config config.DatabaseConfig
	logger *zap.Logger
}

// New creates a new database connection
func New(cfg config.DatabaseConfig, logger *zap.Logger) (*DB, error) {
	connString := cfg.ConnectionString()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxConnections)
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Connected to database",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Name),
	)

	return &DB{
		pool:   pool,
		config: cfg,
		logger: logger,
	}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.pool.Close()
}

// Pool returns the underlying connection pool
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Exec executes a query without returning rows
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) error {
	_, err := db.pool.Exec(ctx, sql, args...)
	return err
}

// QueryRow executes a query that returns a single row
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// Query executes a query that returns rows
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// BeginTx starts a transaction
func (db *DB) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return db.pool.Begin(ctx)
}

// MigrateUp runs all pending migrations
func (db *DB) MigrateUp() error {
	m, err := db.getMigrator()
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// MigrateDown rolls back all migrations
func (db *DB) MigrateDown() error {
	m, err := db.getMigrator()
	if err != nil {
		return err
	}

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration down failed: %w", err)
	}

	return nil
}

// MigrationVersion returns the current migration version
func (db *DB) MigrationVersion() (uint, bool, error) {
	m, err := db.getMigrator()
	if err != nil {
		return 0, false, err
	}

	return m.Version()
}

func (db *DB) getMigrator() (*migrate.Migrate, error) {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, db.config.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	return m, nil
}
