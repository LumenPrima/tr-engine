package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"go.uber.org/zap"
)

// TestDBOptions configures the test database
type TestDBOptions struct {
	// Ephemeral uses a temporary directory that's cleaned up after the test
	// If false, uses a persistent directory for debugging
	Ephemeral bool

	// PersistentPath is used when Ephemeral is false
	// Defaults to ".testdb" in the project root
	PersistentPath string

	// Port for the embedded PostgreSQL server
	// Defaults to 0, which means find an available port dynamically
	Port uint32

	// RunMigrations runs database migrations after starting
	// For embedded postgres (no TimescaleDB), this uses TestSchema
	// For external postgres with TimescaleDB, set UseProductionMigrations=true
	RunMigrations bool

	// UseProductionMigrations uses the real migrations (requires TimescaleDB)
	// If false (default), uses TestSchema which works with plain PostgreSQL
	UseProductionMigrations bool
}

// DefaultTestDBOptions returns sensible defaults
func DefaultTestDBOptions() TestDBOptions {
	return TestDBOptions{
		Ephemeral:      true,
		PersistentPath: ".testdb",
		Port:           0, // 0 means find an available port dynamically
		RunMigrations:  true,
	}
}

// findFreePort finds an available TCP port by binding to port 0
func findFreePort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer listener.Close()
	return uint32(listener.Addr().(*net.TCPAddr).Port), nil
}

// TestDB wraps an embedded PostgreSQL instance for testing
type TestDB struct {
	postgres *embeddedpostgres.EmbeddedPostgres
	DB       *database.DB
	Pool     *pgxpool.Pool
	Config   config.DatabaseConfig
	dataDir  string
	opts     TestDBOptions
	logger   *zap.Logger
}

var (
	// globalTestDB allows reusing the same embedded postgres across tests
	globalTestDB *TestDB
	globalMu     sync.Mutex
)

// NewTestDBForMain creates a test database for use in TestMain
// Unlike NewTestDB, it doesn't require a testing.TB and prints errors to stderr
func NewTestDBForMain(opts TestDBOptions) *TestDB {
	logger, _ := zap.NewDevelopment()

	// Find a free port if not specified
	port := opts.Port
	if port == 0 {
		var err error
		port, err = findFreePort()
		if err != nil {
			logger.Error("failed to find free port", zap.Error(err))
			return nil
		}
		logger.Info("using dynamic port for test database", zap.Uint32("port", port))
	}

	// Determine data directory
	var dataDir string
	if opts.Ephemeral {
		var err error
		dataDir, err = os.MkdirTemp("", "tr-engine-testdb-*")
		if err != nil {
			logger.Error("failed to create temp dir", zap.Error(err))
			return nil
		}
	} else {
		dataDir = opts.PersistentPath
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			logger.Error("failed to create persistent dir", zap.Error(err))
			return nil
		}
	}

	// Configure embedded postgres
	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(port).
			DataPath(filepath.Join(dataDir, "data")).
			RuntimePath(filepath.Join(dataDir, "runtime")).
			BinariesPath(filepath.Join(dataDir, "bin")).
			Username("test").
			Password("test").
			Database("tr_engine_test").
			StartTimeout(60*time.Second),
	)

	// Start PostgreSQL
	if err := pg.Start(); err != nil {
		logger.Error("failed to start embedded postgres", zap.Error(err))
		if opts.Ephemeral {
			os.RemoveAll(dataDir)
		}
		return nil
	}

	// Create database config
	dbConfig := config.DatabaseConfig{
		Host:           "localhost",
		Port:           int(port),
		Name:           "tr_engine_test",
		User:           "test",
		Password:       "test",
		MaxConnections: 10,
		SSLMode:        "disable",
	}

	// Connect to database
	db, err := database.New(dbConfig, logger)
	if err != nil {
		logger.Error("failed to connect to test database", zap.Error(err))
		pg.Stop()
		if opts.Ephemeral {
			os.RemoveAll(dataDir)
		}
		return nil
	}

	testDB := &TestDB{
		postgres: pg,
		DB:       db,
		Pool:     db.Pool(),
		Config:   dbConfig,
		dataDir:  dataDir,
		opts:     opts,
		logger:   logger,
	}

	// Run migrations if requested
	if opts.RunMigrations {
		if opts.UseProductionMigrations {
			if err := db.MigrateUp(); err != nil {
				logger.Error("failed to run migrations", zap.Error(err))
				testDB.Close()
				return nil
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := testDB.Pool.Exec(ctx, TestSchema); err != nil {
				logger.Error("failed to apply test schema", zap.Error(err))
				testDB.Close()
				return nil
			}
		}
	}

	return testDB
}

// NewTestDB creates a new embedded PostgreSQL instance for testing
func NewTestDB(t testing.TB, opts TestDBOptions) *TestDB {
	t.Helper()

	logger, _ := zap.NewDevelopment()

	// Find a free port if not specified
	port := opts.Port
	if port == 0 {
		var err error
		port, err = findFreePort()
		if err != nil {
			t.Fatalf("failed to find free port: %v", err)
		}
		t.Logf("using dynamic port %d for test database", port)
	}

	// Determine data directory
	var dataDir string
	if opts.Ephemeral {
		var err error
		dataDir, err = os.MkdirTemp("", "tr-engine-testdb-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
	} else {
		dataDir = opts.PersistentPath
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			t.Fatalf("failed to create persistent dir: %v", err)
		}
	}

	// Configure embedded postgres
	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(port).
			DataPath(filepath.Join(dataDir, "data")).
			RuntimePath(filepath.Join(dataDir, "runtime")).
			BinariesPath(filepath.Join(dataDir, "bin")).
			Username("test").
			Password("test").
			Database("tr_engine_test").
			StartTimeout(60*time.Second),
	)

	// Start PostgreSQL
	if err := pg.Start(); err != nil {
		if opts.Ephemeral {
			os.RemoveAll(dataDir)
		}
		t.Fatalf("failed to start embedded postgres: %v", err)
	}

	// Create database config
	dbConfig := config.DatabaseConfig{
		Host:           "localhost",
		Port:           int(port),
		Name:           "tr_engine_test",
		User:           "test",
		Password:       "test",
		MaxConnections: 10,
		SSLMode:        "disable",
	}

	// Connect to database
	db, err := database.New(dbConfig, logger)
	if err != nil {
		pg.Stop()
		if opts.Ephemeral {
			os.RemoveAll(dataDir)
		}
		t.Fatalf("failed to connect to test database: %v", err)
	}

	testDB := &TestDB{
		postgres: pg,
		DB:       db,
		Pool:     db.Pool(),
		Config:   dbConfig,
		dataDir:  dataDir,
		opts:     opts,
		logger:   logger,
	}

	// Run migrations if requested
	if opts.RunMigrations {
		if opts.UseProductionMigrations {
			// Use production migrations (requires TimescaleDB)
			if err := db.MigrateUp(); err != nil {
				testDB.Close()
				t.Fatalf("failed to run migrations: %v", err)
			}
		} else {
			// Use test schema (works with plain PostgreSQL)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := testDB.Pool.Exec(ctx, TestSchema); err != nil {
				testDB.Close()
				t.Fatalf("failed to apply test schema: %v", err)
			}
		}
	}

	return testDB
}

// Close stops the embedded PostgreSQL and cleans up
func (tdb *TestDB) Close() {
	if tdb.DB != nil {
		tdb.DB.Close()
	}
	if tdb.postgres != nil {
		tdb.postgres.Stop()
	}
	if tdb.opts.Ephemeral && tdb.dataDir != "" {
		os.RemoveAll(tdb.dataDir)
	}
}

// Reset truncates all tables for a clean state between tests
func (tdb *TestDB) Reset(t testing.TB) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all table names
	rows, err := tdb.Pool.Query(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename != 'schema_migrations'
	`)
	if err != nil {
		t.Fatalf("failed to get table names: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tables = append(tables, table)
	}

	// Truncate all tables
	for _, table := range tables {
		_, err := tdb.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("failed to truncate table %s: %v", table, err)
		}
	}
}

// GetSharedTestDB returns a shared test database instance
// This is more efficient for test suites as it reuses the same postgres instance
func GetSharedTestDB(t testing.TB) *TestDB {
	t.Helper()

	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTestDB == nil {
		opts := DefaultTestDBOptions()
		opts.Ephemeral = true
		globalTestDB = NewTestDB(t, opts)
	}

	return globalTestDB
}

// CleanupSharedTestDB stops the shared test database
// Call this in TestMain after all tests complete
func CleanupSharedTestDB() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTestDB != nil {
		globalTestDB.Close()
		globalTestDB = nil
	}
}

// WithTestDB is a helper that creates a test database, runs the test function,
// and cleans up afterward
func WithTestDB(t *testing.T, opts TestDBOptions, fn func(t *testing.T, db *TestDB)) {
	t.Helper()

	tdb := NewTestDB(t, opts)
	defer tdb.Close()

	fn(t, tdb)
}

// MustExec executes a SQL statement and fails the test on error
func (tdb *TestDB) MustExec(t testing.TB, sql string, args ...interface{}) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := tdb.Pool.Exec(ctx, sql, args...)
	if err != nil {
		t.Fatalf("failed to execute SQL: %v\nSQL: %s", err, sql)
	}
}
