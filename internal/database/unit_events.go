package database

import (
	"context"
	"fmt"
	"time"
)

// UnitEventFilter specifies filters for listing unit events.
type UnitEventFilter struct {
	SystemID  int
	UnitID    int
	EventType *string
	Tgid      *int
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// GlobalUnitEventFilter specifies filters for system-wide unit event queries.
type GlobalUnitEventFilter struct {
	SystemID   *int
	Sysid      *string
	UnitID     *int
	EventTypes []string // multi-type support (ANY array)
	Tgid       *int
	Emergency  *bool
	StartTime  *time.Time
	EndTime    *time.Time
	Sort       string
	Limit      int
	Offset     int
}

// UnitEventAPI represents a unit event for API responses.
type UnitEventAPI struct {
	ID            int64      `json:"id"`
	EventType     string     `json:"event_type"`
	Time          time.Time  `json:"time"`
	SystemID      int        `json:"system_id"`
	SystemName    string     `json:"system_name,omitempty"`
	UnitRID       int        `json:"unit_rid"`
	UnitAlphaTag  string     `json:"unit_alpha_tag,omitempty"`
	Tgid          *int       `json:"tgid,omitempty"`
	TgAlphaTag    string     `json:"tg_alpha_tag,omitempty"`
	InstanceID    string     `json:"instance_id,omitempty"`
}

// ListUnitEvents returns unit events matching the filter.
func (db *DB) ListUnitEvents(ctx context.Context, filter UnitEventFilter) ([]UnitEventAPI, int, error) {
	qb := newQueryBuilder()
	qb.Add("ue.system_id = %s", filter.SystemID)
	qb.Add("ue.unit_rid = %s", filter.UnitID)

	if filter.EventType != nil {
		qb.Add("ue.event_type = %s", *filter.EventType)
	}
	if filter.Tgid != nil {
		qb.Add("ue.tgid = %s", *filter.Tgid)
	}
	if filter.StartTime != nil {
		qb.Add("ue.time >= %s", *filter.StartTime)
	} else {
		qb.Add("ue.time >= %s", time.Now().Add(-24*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("ue.time < %s", *filter.EndTime)
	}

	fromClause := "FROM unit_events ue"
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf(`
		SELECT ue.id, ue.event_type, ue.time, ue.system_id,
			ue.unit_rid, COALESCE(ue.unit_alpha_tag, ''),
			ue.tgid, COALESCE(ue.tg_alpha_tag, ''),
			COALESCE(ue.instance_id, '')
		%s %s
		ORDER BY ue.time DESC
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []UnitEventAPI
	for rows.Next() {
		var e UnitEventAPI
		if err := rows.Scan(
			&e.ID, &e.EventType, &e.Time, &e.SystemID,
			&e.UnitRID, &e.UnitAlphaTag,
			&e.Tgid, &e.TgAlphaTag,
			&e.InstanceID,
		); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	if events == nil {
		events = []UnitEventAPI{}
	}
	return events, total, rows.Err()
}

// ListUnitEventsGlobal returns unit events across a system with JOINs for display names.
// Caller must ensure SystemID or Sysid is set.
func (db *DB) ListUnitEventsGlobal(ctx context.Context, filter GlobalUnitEventFilter) ([]UnitEventAPI, int, error) {
	qb := newQueryBuilder()

	if filter.SystemID != nil {
		qb.Add("ue.system_id = %s", *filter.SystemID)
	}
	if filter.Sysid != nil {
		qb.Add("s.sysid = %s", *filter.Sysid)
	}
	if filter.UnitID != nil {
		qb.Add("ue.unit_rid = %s", *filter.UnitID)
	}
	if len(filter.EventTypes) > 0 {
		qb.Add("ue.event_type = ANY(%s)", filter.EventTypes)
	}
	if filter.Tgid != nil {
		qb.Add("ue.tgid = %s", *filter.Tgid)
	}
	if filter.Emergency != nil {
		qb.Add("ue.emergency = %s", *filter.Emergency)
	}
	if filter.StartTime != nil {
		qb.Add("ue.time >= %s", *filter.StartTime)
	} else {
		qb.Add("ue.time >= %s", time.Now().Add(-1*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("ue.time < %s", *filter.EndTime)
	}

	fromClause := `FROM unit_events ue
		JOIN systems s ON s.system_id = ue.system_id
		LEFT JOIN units u ON u.system_id = ue.system_id AND u.unit_id = ue.unit_rid
		LEFT JOIN talkgroups tg ON tg.system_id = ue.system_id AND tg.tgid = ue.tgid`
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "ue.time DESC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	dataQuery := fmt.Sprintf(`
		SELECT ue.id, ue.event_type, ue.time, ue.system_id, COALESCE(s.name, ''),
			ue.unit_rid, COALESCE(u.alpha_tag, ue.unit_alpha_tag, ''),
			ue.tgid, COALESCE(tg.alpha_tag, ue.tg_alpha_tag, ''),
			COALESCE(ue.instance_id, '')
		%s %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, orderBy, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []UnitEventAPI
	for rows.Next() {
		var e UnitEventAPI
		if err := rows.Scan(
			&e.ID, &e.EventType, &e.Time, &e.SystemID, &e.SystemName,
			&e.UnitRID, &e.UnitAlphaTag,
			&e.Tgid, &e.TgAlphaTag,
			&e.InstanceID,
		); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	if events == nil {
		events = []UnitEventAPI{}
	}
	return events, total, rows.Err()
}

type UnitEventRow struct {
	EventType            string
	SystemID             int
	UnitRID              int
	Time                 time.Time
	Tgid                 *int
	UnitAlphaTag         string
	TgAlphaTag           string
	CallNum              *int
	Freq                 *int64
	StartTime            *time.Time
	StopTime             *time.Time
	Encrypted            *bool
	Emergency            *bool
	Position             *float32
	Length               *float32
	ErrorCount           *int
	SpikeCount           *int
	SampleCount          *int
	TransmissionFilename string
	TalkgroupPatches     []int32
	InstanceID           string
	SysNum               *int16
	SysName              string
}

func (db *DB) InsertUnitEvent(ctx context.Context, e *UnitEventRow) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO unit_events (
			event_type, system_id, unit_rid, "time", tgid,
			unit_alpha_tag, tg_alpha_tag, call_num, freq,
			start_time, stop_time, encrypted, emergency,
			"position", length, error_count, spike_count, sample_count,
			transmission_filename, talkgroup_patches,
			instance_id, sys_num, sys_name
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20,
			$21, $22, $23
		)
	`,
		e.EventType, e.SystemID, e.UnitRID, e.Time, e.Tgid,
		e.UnitAlphaTag, e.TgAlphaTag, e.CallNum, e.Freq,
		e.StartTime, e.StopTime, e.Encrypted, e.Emergency,
		e.Position, e.Length, e.ErrorCount, e.SpikeCount, e.SampleCount,
		e.TransmissionFilename, e.TalkgroupPatches,
		e.InstanceID, e.SysNum, e.SysName,
	)
	return err
}

// AffiliationBackfillRow holds the data needed to populate an affiliation map entry from the DB.
type AffiliationBackfillRow struct {
	SystemID      int
	UnitRID       int
	Tgid          int
	UnitAlphaTag  string
	TgAlphaTag    string
	TgDescription string
	TgTag         string
	TgGroup       string
	SystemName    string
	Sysid         string
	Time          time.Time
}

// LoadRecentAffiliations returns the most recent "join" event per (system_id, unit_rid)
// from the last 24 hours, with display names from JOINed tables.
func (db *DB) LoadRecentAffiliations(ctx context.Context) ([]AffiliationBackfillRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT ON (ue.system_id, ue.unit_rid)
			ue.system_id, ue.unit_rid, ue.tgid,
			COALESCE(u.alpha_tag, ue.unit_alpha_tag, ''),
			COALESCE(tg.alpha_tag, ue.tg_alpha_tag, ''),
			COALESCE(tg.description, ''),
			COALESCE(tg.tag, ''),
			COALESCE(tg."group", ''),
			COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			ue."time"
		FROM unit_events ue
		JOIN systems s ON s.system_id = ue.system_id
		LEFT JOIN units u ON u.system_id = ue.system_id AND u.unit_id = ue.unit_rid
		LEFT JOIN talkgroups tg ON tg.system_id = ue.system_id AND tg.tgid = ue.tgid
		WHERE ue.event_type = 'join'
		  AND ue."time" > now() - interval '24 hours'
		  AND ue.tgid IS NOT NULL
		ORDER BY ue.system_id, ue.unit_rid, ue."time" DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AffiliationBackfillRow
	for rows.Next() {
		var r AffiliationBackfillRow
		if err := rows.Scan(
			&r.SystemID, &r.UnitRID, &r.Tgid,
			&r.UnitAlphaTag, &r.TgAlphaTag,
			&r.TgDescription, &r.TgTag, &r.TgGroup,
			&r.SystemName, &r.Sysid,
			&r.Time,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
