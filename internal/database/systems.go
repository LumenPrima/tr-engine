package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

type System struct {
	SystemID   int
	SystemType string
	Name       string
	Sysid      string
	Wacn       string
}

// FindOrCreateSystem finds an existing system by (instance_id, sys_name) via the sites table,
// or creates a new one. Returns the system_id and sysid (P25 system identifier).
func (db *DB) FindOrCreateSystem(ctx context.Context, instanceID, sysName string) (int, string, error) {
	row, err := db.Q.FindSystemViaSite(ctx, sqlcdb.FindSystemViaSiteParams{
		InstanceID: instanceID,
		ShortName:  sysName,
	})
	if err == nil {
		return row.SystemID, row.Sysid, nil
	}

	// Create new system
	systemID, err := db.Q.CreateSystem(ctx, &sysName)
	if err != nil {
		return 0, "", fmt.Errorf("create system %q: %w", sysName, err)
	}
	return systemID, "0", nil
}

// UpdateSystemIdentity updates a system's P25 identity fields.
func (db *DB) UpdateSystemIdentity(ctx context.Context, systemID int, systemType, sysid, wacn, name string) error {
	return db.Q.UpdateSystemIdentity(ctx, sqlcdb.UpdateSystemIdentityParams{
		SystemID:   systemID,
		SystemType: systemType,
		Sysid:      sysid,
		Wacn:       wacn,
		Name:       name,
	})
}

// FindSystemBySysidWacn finds an active system by (sysid, wacn), excluding a given system_id.
func (db *DB) FindSystemBySysidWacn(ctx context.Context, sysid, wacn string, excludeSystemID int) (int, error) {
	systemID, err := db.Q.FindSystemBySysidWacn(ctx, sqlcdb.FindSystemBySysidWacnParams{
		Sysid:    sysid,
		Wacn:     wacn,
		SystemID: excludeSystemID,
	})
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return systemID, err
}

// MergeSystems moves all child records from sourceID to targetID and soft-deletes the source.
// Returns counts of moved records for the merge log.
func (db *DB) MergeSystems(ctx context.Context, sourceID, targetID int, performedBy string) (callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved int, err error) {
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

	// Move call_groups — handle conflicts first
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
		if _, err := tx.Exec(ctx, `UPDATE calls SET call_group_id = $1 WHERE call_group_id = $2`, c.dst, c.src); err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("reassign call group calls: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM call_groups WHERE id = $1`, c.src); err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("delete conflicting call group: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE call_groups SET system_id = $1 WHERE system_id = $2`, targetID, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move call groups: %w", err)
	}

	// Merge talkgroups
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
			tgMerged++
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM talkgroups WHERE system_id = $1`, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("delete source talkgroups: %w", err)
	}

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
	if _, err := tx.Exec(ctx, `DELETE FROM units WHERE system_id = $1`, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("delete source units: %w", err)
	}

	// Move unit_events
	tag, err = tx.Exec(ctx, `UPDATE unit_events SET system_id = $1 WHERE system_id = $2`, targetID, sourceID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move unit_events: %w", err)
	}
	eventsMoved = int(tag.RowsAffected())

	// Move decode_rates
	if _, err := tx.Exec(ctx, `UPDATE decode_rates SET system_id = $1 WHERE system_id = $2`, targetID, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move decode_rates: %w", err)
	}

	// Move sites to target system
	if _, err := tx.Exec(ctx, `UPDATE sites SET system_id = $1 WHERE system_id = $2`, targetID, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("move sites: %w", err)
	}

	// Combine system names
	var targetName, sourceName string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name,'') FROM systems WHERE system_id = $1`, targetID).Scan(&targetName)
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name,'') FROM systems WHERE system_id = $1`, sourceID).Scan(&sourceName)
	if sourceName != "" && !strings.Contains(targetName, sourceName) {
		combined := targetName + "/" + sourceName
		if _, err := tx.Exec(ctx, `UPDATE systems SET name = $1 WHERE system_id = $2`, combined, targetID); err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("combine names: %w", err)
		}
	}

	// Soft-delete source system
	if _, err := tx.Exec(ctx, `UPDATE systems SET deleted_at = now() WHERE system_id = $1`, sourceID); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("soft-delete source: %w", err)
	}

	// Log the merge
	if _, err := tx.Exec(ctx, `
		INSERT INTO system_merge_log (source_id, target_id, calls_moved, talkgroups_moved, talkgroups_merged, units_moved, units_merged, events_moved, performed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, sourceID, targetID, callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved, performedBy); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("log merge: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("commit merge: %w", err)
	}

	return callsMoved, tgMoved, tgMerged, unitsMoved, unitsMerged, eventsMoved, nil
}

// SystemAPI represents a system with embedded sites for API responses.
type SystemAPI struct {
	SystemID   int       `json:"system_id"`
	SystemType string    `json:"system_type"`
	Name       string    `json:"name,omitempty"`
	Sysid      string    `json:"sysid"`
	Wacn       string    `json:"wacn"`
	Sites      []SiteAPI `json:"sites"`
}

// GetSystemByID returns a single system with its sites.
func (db *DB) GetSystemByID(ctx context.Context, systemID int) (*SystemAPI, error) {
	row, err := db.Q.GetSystemByID(ctx, systemID)
	if err != nil {
		return nil, err
	}
	s := &SystemAPI{
		SystemID:   row.SystemID,
		SystemType: row.SystemType,
		Name:       row.Name,
		Sysid:      row.Sysid,
		Wacn:       row.Wacn,
	}
	sites, err := db.ListSitesForSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	s.Sites = sites
	return s, nil
}

// ListSystemsWithSites returns all active systems with their sites.
func (db *DB) ListSystemsWithSites(ctx context.Context) ([]SystemAPI, error) {
	sysRows, err := db.Q.ListActiveSystems(ctx)
	if err != nil {
		return nil, err
	}

	systems := make([]SystemAPI, len(sysRows))
	for i, r := range sysRows {
		systems[i] = SystemAPI{
			SystemID:   r.SystemID,
			SystemType: r.SystemType,
			Name:       r.Name,
			Sysid:      r.Sysid,
			Wacn:       r.Wacn,
		}
	}

	// Load sites for each system
	allSites, err := db.LoadAllSitesAPI(ctx)
	if err != nil {
		return nil, err
	}
	sitesBySystem := make(map[int][]SiteAPI)
	for _, s := range allSites {
		sitesBySystem[s.SystemID] = append(sitesBySystem[s.SystemID], s)
	}
	for i := range systems {
		systems[i].Sites = sitesBySystem[systems[i].SystemID]
		if systems[i].Sites == nil {
			systems[i].Sites = []SiteAPI{}
		}
	}

	return systems, nil
}

// P25SystemAPI represents a P25 system with stats.
type P25SystemAPI struct {
	SystemID       int       `json:"system_id"`
	Name           string    `json:"name,omitempty"`
	Sysid          string    `json:"sysid"`
	Wacn           string    `json:"wacn"`
	Sites          []SiteAPI `json:"sites"`
	TalkgroupCount int       `json:"talkgroup_count"`
	UnitCount      int       `json:"unit_count"`
	Calls24h       int       `json:"calls_24h"`
}

// ListP25Systems returns P25 systems with stats.
func (db *DB) ListP25Systems(ctx context.Context) ([]P25SystemAPI, error) {
	sysRows, err := db.Q.ListP25Systems(ctx)
	if err != nil {
		return nil, err
	}

	systems := make([]P25SystemAPI, len(sysRows))
	for i, r := range sysRows {
		systems[i] = P25SystemAPI{
			SystemID:       r.SystemID,
			Name:           r.Name,
			Sysid:         r.Sysid,
			Wacn:           r.Wacn,
			TalkgroupCount: int(r.TalkgroupCount),
			UnitCount:      int(r.UnitCount),
			Calls24h:       int(r.Calls24h),
		}
	}

	// Load sites
	allSites, err := db.LoadAllSitesAPI(ctx)
	if err != nil {
		return nil, err
	}
	sitesBySystem := make(map[int][]SiteAPI)
	for _, s := range allSites {
		sitesBySystem[s.SystemID] = append(sitesBySystem[s.SystemID], s)
	}
	for i := range systems {
		systems[i].Sites = sitesBySystem[systems[i].SystemID]
		if systems[i].Sites == nil {
			systems[i].Sites = []SiteAPI{}
		}
	}

	return systems, nil
}

// UpdateSystemFields updates mutable system fields. Only non-nil fields are updated.
func (db *DB) UpdateSystemFields(ctx context.Context, systemID int, name, sysid, wacn *string) error {
	nameVal := ""
	if name != nil {
		nameVal = *name
	}
	sysidVal := ""
	if sysid != nil {
		sysidVal = *sysid
	}
	wacnVal := ""
	if wacn != nil {
		wacnVal = *wacn
	}

	return db.Q.UpdateSystemFields(ctx, sqlcdb.UpdateSystemFieldsParams{
		SystemID: systemID,
		Name:     nameVal,
		Sysid:    sysidVal,
		Wacn:     wacnVal,
	})
}

// LoadAllSystems returns all active systems.
func (db *DB) LoadAllSystems(ctx context.Context) ([]System, error) {
	rows, err := db.Q.LoadAllSystems(ctx)
	if err != nil {
		return nil, err
	}
	systems := make([]System, len(rows))
	for i, r := range rows {
		systems[i] = System{
			SystemID:   r.SystemID,
			SystemType: r.SystemType,
			Name:       r.Name,
			Sysid:      r.Sysid,
			Wacn:       r.Wacn,
		}
	}
	return systems, nil
}
