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
