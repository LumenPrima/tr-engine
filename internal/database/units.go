package database

import (
	"context"
	"time"
)

// UpsertUnit inserts or updates a unit, never overwriting good data with empty strings.
func (db *DB) UpsertUnit(ctx context.Context, systemID, unitID int, alphaTag, eventType string, eventTime time.Time, tgid int) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO units (system_id, unit_id, alpha_tag, first_seen, last_seen, last_event_type, last_event_time, last_event_tgid)
		VALUES ($1, $2, $3, $5, $5, $4, $5, $6)
		ON CONFLICT (system_id, unit_id) DO UPDATE SET
			alpha_tag       = COALESCE(NULLIF($3, ''), units.alpha_tag),
			last_seen       = $5,
			last_event_type = $4,
			last_event_time = $5,
			last_event_tgid = CASE WHEN $6 > 0 THEN $6 ELSE units.last_event_tgid END
	`, systemID, unitID, alphaTag, eventType, eventTime, tgid)
	return err
}
