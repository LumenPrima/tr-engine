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

// Schema is the TimescaleDB-free schema for embedded PostgreSQL.
// This mirrors the production schema but without hypertable commands.
const Schema = `
-- Trunk-recorder instances connecting to this engine
CREATE TABLE IF NOT EXISTS instances (
    id              SERIAL PRIMARY KEY,
    instance_id     VARCHAR(255) UNIQUE NOT NULL,
    instance_key    VARCHAR(255),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    config_json     JSONB
);

-- SDR sources from each instance
CREATE TABLE IF NOT EXISTS sources (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    source_num      INTEGER NOT NULL,
    center_freq     BIGINT,
    rate            INTEGER,
    driver          VARCHAR(64),
    device          VARCHAR(255),
    antenna         VARCHAR(64),
    gain            INTEGER,
    config_json     JSONB,
    UNIQUE(instance_id, source_num)
);

-- Radio systems (trunked or conventional)
CREATE TABLE IF NOT EXISTS systems (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    sys_num         INTEGER NOT NULL,
    short_name      VARCHAR(64) NOT NULL,
    system_type     VARCHAR(32),
    sysid           VARCHAR(16),
    wacn            VARCHAR(16),
    nac             VARCHAR(16),
    rfss            INTEGER,
    site_id         INTEGER,
    config_json     JSONB,
    UNIQUE(instance_id, sys_num)
);

CREATE INDEX IF NOT EXISTS idx_systems_short_name ON systems(short_name);

-- Talkgroups (loaded from talkgroup files or discovered)
CREATE TABLE IF NOT EXISTS talkgroups (
    id              SERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    tgid            INTEGER NOT NULL,
    alpha_tag       VARCHAR(255),
    description     TEXT,
    tg_group        VARCHAR(255),
    tag             VARCHAR(64),
    priority        INTEGER,
    mode            VARCHAR(16),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(system_id, tgid)
);

CREATE INDEX IF NOT EXISTS idx_talkgroups_last_seen ON talkgroups(last_seen DESC);

-- Radio units
CREATE TABLE IF NOT EXISTS units (
    id              SERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_id         BIGINT NOT NULL,
    alpha_tag       VARCHAR(255),
    alpha_tag_source VARCHAR(32),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(system_id, unit_id)
);

CREATE INDEX IF NOT EXISTS idx_units_last_seen ON units(last_seen DESC);

-- Recorders from each instance
CREATE TABLE IF NOT EXISTS recorders (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    source_id       INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    rec_num         INTEGER NOT NULL,
    rec_type        VARCHAR(16),
    UNIQUE(instance_id, source_id, rec_num)
);

-- Call groups (deduplicated logical calls)
CREATE TABLE IF NOT EXISTS call_groups (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
    tgid            INTEGER NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ,
    primary_call_id BIGINT,
    call_count      INTEGER DEFAULT 1,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_call_groups_talkgroup ON call_groups(talkgroup_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_call_groups_system ON call_groups(system_id, start_time DESC);

-- Individual call recordings (one per site/instance)
CREATE TABLE IF NOT EXISTS calls (
    id              BIGSERIAL PRIMARY KEY,
    call_group_id   BIGINT,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
    recorder_id     INTEGER REFERENCES recorders(id) ON DELETE SET NULL,

    -- Call identifiers
    tr_call_id      VARCHAR(64),
    call_num        BIGINT,

    -- Timing
    start_time      TIMESTAMPTZ NOT NULL,
    stop_time       TIMESTAMPTZ,
    duration        REAL,

    -- Status
    call_state      SMALLINT,
    mon_state       SMALLINT,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE,
    phase2_tdma     BOOLEAN DEFAULT FALSE,
    tdma_slot       SMALLINT,
    conventional    BOOLEAN DEFAULT FALSE,
    analog          BOOLEAN DEFAULT FALSE,
    audio_type      VARCHAR(16),

    -- Quality metrics
    freq            BIGINT,
    freq_error      INTEGER,
    error_count     INTEGER,
    spike_count     INTEGER,
    signal_db       REAL,
    noise_db        REAL,

    -- Audio file reference
    audio_path      VARCHAR(512),
    audio_size      INTEGER,

    -- Patches
    patched_tgids   INTEGER[],

    -- Full JSON for any fields we don't explicitly model
    metadata_json   JSONB
);

CREATE INDEX IF NOT EXISTS idx_calls_talkgroup ON calls(talkgroup_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_calls_instance ON calls(instance_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_calls_group ON calls(call_group_id);
CREATE INDEX IF NOT EXISTS idx_calls_tr_call_id ON calls(tr_call_id);

-- Transmissions within calls (for unit tracking)
CREATE TABLE IF NOT EXISTS transmissions (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL,
    unit_id         INTEGER REFERENCES units(id) ON DELETE SET NULL,
    unit_rid        BIGINT NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    stop_time       TIMESTAMPTZ,
    duration        REAL,
    position        REAL,
    emergency       BOOLEAN DEFAULT FALSE,
    error_count     INTEGER,
    spike_count     INTEGER
);

CREATE INDEX IF NOT EXISTS idx_transmissions_unit ON transmissions(unit_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_transmissions_call ON transmissions(call_id);

-- Frequency usage within calls
CREATE TABLE IF NOT EXISTS call_frequencies (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL,
    freq            BIGINT NOT NULL,
    time            TIMESTAMPTZ NOT NULL,
    position        REAL,
    duration        REAL,
    error_count     INTEGER,
    spike_count     INTEGER
);

-- Unit events (registration, affiliation, etc.)
CREATE TABLE IF NOT EXISTS unit_events (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_id         INTEGER REFERENCES units(id) ON DELETE SET NULL,
    unit_rid        BIGINT NOT NULL,
    event_type      VARCHAR(16) NOT NULL,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
    tgid            INTEGER,
    time            TIMESTAMPTZ NOT NULL,
    metadata_json   JSONB
);

CREATE INDEX IF NOT EXISTS idx_unit_events_unit ON unit_events(unit_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_unit_events_system ON unit_events(system_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_unit_events_type ON unit_events(event_type, time DESC);

-- System decode rates
CREATE TABLE IF NOT EXISTS system_rates (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    decode_rate     REAL,
    control_channel BIGINT
);

-- Recorder status snapshots
CREATE TABLE IF NOT EXISTS recorder_status (
    id              BIGSERIAL PRIMARY KEY,
    recorder_id     INTEGER REFERENCES recorders(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    state           SMALLINT,
    freq            BIGINT,
    call_count      INTEGER,
    duration        REAL,
    squelched       BOOLEAN
);

-- Trunking messages (optional, high volume)
CREATE TABLE IF NOT EXISTS trunk_messages (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    msg_type        SMALLINT,
    msg_type_name   VARCHAR(32),
    opcode          VARCHAR(4),
    opcode_type     VARCHAR(32),
    opcode_desc     VARCHAR(128),
    meta            TEXT
);

-- Migration tracking table (for compatibility with golang-migrate)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL DEFAULT FALSE
);

-- Mark as migrated to version 1 (matches production migration)
INSERT INTO schema_migrations (version, dirty) VALUES (1, FALSE) ON CONFLICT DO NOTHING;
`

// InitSchema initializes the embedded database with the TimescaleDB-free schema.
// This should be called on first run when the database is empty.
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, Schema)
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
