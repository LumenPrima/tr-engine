package database

import (
	"context"
	"fmt"
	"strings"
)

// migration defines a single idempotent schema migration.
type migration struct {
	name  string
	sql   string
	check string // query that returns true if the migration is already applied
}

// migrations is the ordered list of schema migrations to apply.
// Each must be idempotent (use IF NOT EXISTS, IF EXISTS, etc.).
var migrations = []migration{
	{
		name:  "add calls.incidentdata",
		sql:   `ALTER TABLE calls ADD COLUMN IF NOT EXISTS incidentdata jsonb`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'calls' AND column_name = 'incidentdata')`,
	},
	{
		name:  "add unit_events.incidentdata",
		sql:   `ALTER TABLE unit_events ADD COLUMN IF NOT EXISTS incidentdata jsonb`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'unit_events' AND column_name = 'incidentdata')`,
	},
	{
		name:  "add talkgroups.alpha_tag_source",
		sql:   `ALTER TABLE talkgroups ADD COLUMN IF NOT EXISTS alpha_tag_source text`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'talkgroups' AND column_name = 'alpha_tag_source')`,
	},
	{
		name: "add talkgroup stats cache columns",
		sql: `ALTER TABLE talkgroups
			ADD COLUMN IF NOT EXISTS call_count_30d int NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS calls_1h int NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS calls_24h int NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS unit_count_30d int NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS stats_updated_at timestamptz`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'talkgroups' AND column_name = 'call_count_30d')`,
	},
	{
		name: "replace unique sysid/wacn index with non-unique",
		sql: `DROP INDEX IF EXISTS uq_systems_sysid_wacn;
CREATE INDEX IF NOT EXISTS idx_systems_sysid_wacn ON systems (sysid, wacn)
    WHERE system_type IN ('p25', 'smartnet') AND deleted_at IS NULL AND sysid <> '0'`,
		check: `SELECT NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'uq_systems_sysid_wacn')`,
	},
	{
		name:  "add decode_rates time-only index",
		sql:   `CREATE INDEX IF NOT EXISTS idx_decode_rates_time ON decode_rates ("time" DESC)`,
		check: `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_decode_rates_time')`,
	},
	{
		name:  "add recorder_snapshots time-only index",
		sql:   `CREATE INDEX IF NOT EXISTS idx_recorder_snapshots_time ON recorder_snapshots ("time" DESC)`,
		check: `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_recorder_snapshots_time')`,
	},
	{
		name: "expand system_type CHECK for conventional variants",
		sql: `ALTER TABLE systems DROP CONSTRAINT IF EXISTS systems_system_type_check;
ALTER TABLE systems ADD CONSTRAINT systems_system_type_check
    CHECK (system_type IN ('p25', 'smartnet', 'conventional', 'conventionalP25', 'conventionalDMR', 'conventionalSIGMF'))`,
		check: `SELECT EXISTS (
    SELECT 1 FROM information_schema.check_constraints
    WHERE constraint_name = 'systems_system_type_check'
      AND check_clause LIKE '%conventionalP25%'
)`,
	},
	{
		name:  "add provider_ms to transcriptions",
		sql:   `ALTER TABLE transcriptions ADD COLUMN IF NOT EXISTS provider_ms int`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'transcriptions' AND column_name = 'provider_ms')`,
	},
	{
		name: "add empty to transcription_status check constraints",
		sql: `
			ALTER TABLE calls DROP CONSTRAINT IF EXISTS calls_transcription_status_check;
			ALTER TABLE calls ADD CONSTRAINT calls_transcription_status_check
				CHECK (transcription_status IN ('none', 'auto', 'reviewed', 'verified', 'excluded', 'empty'));
			ALTER TABLE call_groups DROP CONSTRAINT IF EXISTS call_groups_transcription_status_check;
			ALTER TABLE call_groups ADD CONSTRAINT call_groups_transcription_status_check
				CHECK (transcription_status IN ('none', 'auto', 'reviewed', 'verified', 'excluded', 'empty'))`,
		check: `SELECT EXISTS (
			SELECT 1 FROM information_schema.check_constraints
			WHERE constraint_name = 'calls_transcription_status_check'
			  AND check_clause LIKE '%empty%')`,
	},
	{
		name: "create users table",
		sql: `CREATE TABLE IF NOT EXISTS users (
			id            serial       PRIMARY KEY,
			username      text         UNIQUE NOT NULL,
			password_hash text         NOT NULL,
			role          text         NOT NULL DEFAULT 'viewer'
			              CHECK (role IN ('viewer', 'editor', 'admin')),
			enabled       boolean      NOT NULL DEFAULT true,
			created_at    timestamptz  NOT NULL DEFAULT now(),
			updated_at    timestamptz  NOT NULL DEFAULT now()
		);
		CREATE TRIGGER trg_users_updated_at
			BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at()`,
		check: `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'users')`,
	},
	{
		name: "add display_name and last_login to users",
		sql: `ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';
		      ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login timestamptz`,
		check: `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'display_name')`,
	},
	{
		name: "create config_overrides table",
		sql: `CREATE TABLE IF NOT EXISTS config_overrides (
			key        text PRIMARY KEY,
			value      text NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		check: `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'config_overrides')`,
	},
	{
		name: "create api_keys table",
		sql: `CREATE TABLE IF NOT EXISTS api_keys (
			id                 serial       PRIMARY KEY,
			key_hash           text         UNIQUE NOT NULL,
			key_prefix         text         NOT NULL,
			user_id            int          REFERENCES users(id) ON DELETE CASCADE,
			role               text         NOT NULL DEFAULT 'viewer'
			                   CHECK (role IN ('viewer', 'editor', 'admin')),
			label              text         NOT NULL DEFAULT '',
			is_service_account boolean      NOT NULL DEFAULT false,
			created_at         timestamptz  NOT NULL DEFAULT now(),
			last_used_at       timestamptz
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
		CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash)`,
		check: `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'api_keys')`,
	},
}

// Migrate runs all pending schema migrations.
// For each migration, it first checks whether the change is already present.
// If not, it attempts to apply it. If the apply fails (e.g. insufficient
// privileges), the error is returned — the caller should treat this as fatal
// since the application's queries depend on these columns existing.
func (db *DB) Migrate(ctx context.Context) error {
	var pending []migration
	for _, m := range migrations {
		if m.check != "" {
			var exists bool
			if err := db.Pool.QueryRow(ctx, m.check).Scan(&exists); err == nil && exists {
				continue
			}
		}
		pending = append(pending, m)
	}

	if len(pending) == 0 {
		return nil
	}

	// Try to apply each pending migration
	applied := 0
	for _, m := range pending {
		if _, err := db.Pool.Exec(ctx, m.sql); err != nil {
			return &MigrationError{
				failed:  m,
				pending: pending[applied:],
				err:     err,
			}
		}
		db.log.Info().Str("migration", m.name).Msg("schema migration applied")
		applied++
	}
	db.log.Info().Int("applied", applied).Msg("schema migrations complete")
	return nil
}

// MigrationError is returned when a migration fails.
// It includes the SQL needed to apply all remaining migrations manually.
type MigrationError struct {
	failed  migration
	pending []migration
	err     error
}

func (e *MigrationError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "migration %q failed: %v\n\n", e.failed.name, e.err)
	b.WriteString("Run the following SQL as a database superuser to fix this:\n\n")
	for _, m := range e.pending {
		fmt.Fprintf(&b, "  %s;\n", m.sql)
	}
	b.WriteString("\nThen restart tr-engine.")
	return b.String()
}

func (e *MigrationError) Unwrap() error {
	return e.err
}
