package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type System struct {
	SystemID   int
	SystemType string
	Name       string
	Sysid      string
	Wacn       string
}

// FindOrCreateSystem finds an existing system by (instance_id, sys_name) via the sites table,
// or creates a new one. Returns the system_id.
func (db *DB) FindOrCreateSystem(ctx context.Context, instanceID, sysName string) (int, error) {
	// Try to find via sites table first
	var systemID int
	err := db.Pool.QueryRow(ctx, `
		SELECT s.system_id FROM sites s
		WHERE s.instance_id = $1 AND s.short_name = $2
		LIMIT 1
	`, instanceID, sysName).Scan(&systemID)

	if err == nil {
		return systemID, nil
	}

	// Create new system — we don't have sysid/wacn from MQTT, so use defaults
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO systems (system_type, name, sysid, wacn)
		VALUES ('p25', $1, '0', '0')
		RETURNING system_id
	`, sysName).Scan(&systemID)
	if err != nil {
		return 0, fmt.Errorf("create system %q: %w", sysName, err)
	}

	return systemID, nil
}

// UpdateSystemIdentity updates a system's P25 identity fields.
// Only updates fields that have non-zero/non-empty values (progressive refinement).
func (db *DB) UpdateSystemIdentity(ctx context.Context, systemID int, systemType, sysid, wacn, name string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE systems SET
			system_type = COALESCE(NULLIF($2, ''), system_type),
			sysid       = CASE WHEN $3 <> '' AND $3 <> '0' THEN $3 ELSE sysid END,
			wacn        = CASE WHEN $4 <> '' AND $4 <> '0' THEN $4 ELSE wacn END,
			name        = COALESCE(NULLIF($5, ''), name)
		WHERE system_id = $1 AND deleted_at IS NULL
	`, systemID, systemType, sysid, wacn, name)
	return err
}

// FindSystemBySysidWacn finds an active system by (sysid, wacn), excluding a given system_id.
// Returns the system_id or 0 if not found.
func (db *DB) FindSystemBySysidWacn(ctx context.Context, sysid, wacn string, excludeSystemID int) (int, error) {
	var systemID int
	err := db.Pool.QueryRow(ctx, `
		SELECT system_id FROM systems
		WHERE sysid = $1 AND wacn = $2
		  AND sysid <> '0'
		  AND system_id <> $3
		  AND deleted_at IS NULL
		LIMIT 1
	`, sysid, wacn, excludeSystemID).Scan(&systemID)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return systemID, err
}

// MergeSystems moves all child records from sourceID to targetID and soft-deletes the source.
// Returns counts of moved records for the merge log.
func (db *DB) MergeSystems(ctx context.Context, sourceID, targetID int) (callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved int, err error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Move calls
	tag, err := tx.Exec(ctx, `UPDATE calls SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move calls: %w", err)
	}
	callsMoved = int(tag.RowsAffected())

	// Move call_groups
	// First, handle conflicts: if target already has a group for (system_id, tgid, start_time),
	// reassign calls from the source group to the target group, then delete the source group.
	rows, err := tx.Query(ctx, `
		SELECT sg.id as source_group_id, tg.id as target_group_id
		FROM call_groups sg
		JOIN call_groups tg ON tg.system_id = $1 AND tg.tgid = sg.tgid AND tg.start_time = sg.start_time
		WHERE sg.system_id = $2
	`, targetID, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("find conflicting call groups: %w", err)
	}
	var cgConflicts []struct{ src, dst int }
	for rows.Next() {
		var src, dst int
		if err := rows.Scan(&src, &dst); err != nil {
			rows.Close()
			return 0, 0, 0, 0, 0, 0, err
		}
		cgConflicts = append(cgConflicts, struct{ src, dst int }{src, dst})
	}
	rows.Close()

	for _, c := range cgConflicts {
		tx.Exec(ctx, `UPDATE calls SET call_group_id = $1 WHERE call_group_id = $2`, c.dst, c.src)
		tx.Exec(ctx, `DELETE FROM call_groups WHERE id = $1`, c.src)
	}
	// Move remaining non-conflicting call_groups
	tx.Exec(ctx, `UPDATE call_groups SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)

	// Merge talkgroups (ON CONFLICT: keep best data from both)
	// Collect all rows first to avoid "conn busy" — pgx can't interleave queries on one conn.
	type tgRow struct {
		tgid                        int
		alpha, tag, group, desc string
	}
	tgRows, err := tx.Query(ctx, `SELECT tgid, COALESCE(alpha_tag,''), COALESCE(tag,''), COALESCE("group",''), COALESCE(description,'') FROM talkgroups WHERE system_id = $1`, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("read source talkgroups: %w", err)
	}
	var tgs []tgRow
	for tgRows.Next() {
		var r tgRow
		if err := tgRows.Scan(&r.tgid, &r.alpha, &r.tag, &r.group, &r.desc); err != nil {
			tgRows.Close()
			return 0, 0, 0, 0, 0, 0, err
		}
		tgs = append(tgs, r)
	}
	tgRows.Close()

	for _, r := range tgs {
		result, err := tx.Exec(ctx, `
			INSERT INTO talkgroups (system_id, tgid, alpha_tag, tag, "group", description, first_seen, last_seen)
			VALUES ($1, $2, $3, $4, $5, $6, now(), now())
			ON CONFLICT (system_id, tgid) DO UPDATE SET
				alpha_tag   = COALESCE(NULLIF($3, ''), talkgroups.alpha_tag),
				tag         = COALESCE(NULLIF($4, ''), talkgroups.tag),
				"group"     = COALESCE(NULLIF($5, ''), talkgroups."group"),
				description = COALESCE(NULLIF($6, ''), talkgroups.description)
		`, targetID, r.tgid, r.alpha, r.tag, r.group, r.desc)
		if err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("merge talkgroup %d: %w", r.tgid, err)
		}
		tgMoved++
		if result.RowsAffected() == 0 {
			tgMerged++ // was an update, not insert
		}
	}
	tx.Exec(ctx, `DELETE FROM talkgroups WHERE system_id = $1`, sourceID)

	// Merge units
	type unitRow struct {
		unitID int
		alpha  string
	}
	uRows, err := tx.Query(ctx, `SELECT unit_id, COALESCE(alpha_tag,'') FROM units WHERE system_id = $1`, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("read source units: %w", err)
	}
	var units []unitRow
	for uRows.Next() {
		var r unitRow
		if err := uRows.Scan(&r.unitID, &r.alpha); err != nil {
			uRows.Close()
			return 0, 0, 0, 0, 0, 0, err
		}
		units = append(units, r)
	}
	uRows.Close()

	for _, r := range units {
		result, err := tx.Exec(ctx, `
			INSERT INTO units (system_id, unit_id, alpha_tag, first_seen, last_seen)
			VALUES ($1, $2, $3, now(), now())
			ON CONFLICT (system_id, unit_id) DO UPDATE SET
				alpha_tag = COALESCE(NULLIF($3, ''), units.alpha_tag)
		`, targetID, r.unitID, r.alpha)
		if err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("merge unit %d: %w", r.unitID, err)
		}
		unitsMoved++
		if result.RowsAffected() == 0 {
			unitsMerged++
		}
	}
	tx.Exec(ctx, `DELETE FROM units WHERE system_id = $1`, sourceID)

	// Move unit_events
	tag, err = tx.Exec(ctx, `UPDATE unit_events SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move unit_events: %w", err)
	}
	eventsMoved = int(tag.RowsAffected())

	// Move decode_rates
	tx.Exec(ctx, `UPDATE decode_rates SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)

	// Move sites to target system
	tx.Exec(ctx, `UPDATE sites SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)

	// Combine system names: "butco" + "warco" → "butco/warco"
	var targetName, sourceName string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name,'') FROM systems WHERE system_id = $1`, targetID).Scan(&targetName)
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name,'') FROM systems WHERE system_id = $1`, sourceID).Scan(&sourceName)
	if sourceName != "" && !strings.Contains(targetName, sourceName) {
		combined := targetName + "/" + sourceName
		tx.Exec(ctx, `UPDATE systems SET name = $1 WHERE system_id = $2`, combined, targetID)
	}

	// Soft-delete source system
	tx.Exec(ctx, `UPDATE systems SET deleted_at = now() WHERE system_id = $1`, sourceID)

	// Log the merge
	tx.Exec(ctx, `
		INSERT INTO system_merge_log (source_id, target_id, calls_moved, talkgroups_moved, talkgroups_merged, units_moved, units_merged, events_moved)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sourceID, targetID, callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved)

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("commit merge: %w", err)
	}

	return callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved, nil
}

// LoadAllSystems returns all active systems.
func (db *DB) LoadAllSystems(ctx context.Context) ([]System, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT system_id, system_type, COALESCE(name, ''), sysid, wacn
		FROM systems
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []System
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.SystemID, &s.SystemType, &s.Name, &s.Sysid, &s.Wacn); err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}
