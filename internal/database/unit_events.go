package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
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
	SystemIDs  []int
	Sysids     []string
	UnitIDs    []int
	EventTypes []string // multi-type support (ANY array)
	Tgids      []int
	Emergency  *bool
	StartTime  *time.Time
	EndTime    *time.Time
	Sort       string
	Limit      int
	Offset     int
}

// UnitEventAPI represents a unit event for API responses.
type UnitEventAPI struct {
	ID            int64     `json:"id"`
	EventType     string    `json:"event_type"`
	Time          time.Time `json:"time"`
	SystemID      int       `json:"system_id"`
	SystemName    string    `json:"system_name,omitempty"`
	UnitRID       int       `json:"unit_rid"`
	UnitAlphaTag  string    `json:"unit_alpha_tag,omitempty"`
	Tgid          *int      `json:"tgid,omitempty"`
	TgAlphaTag    string    `json:"tg_alpha_tag,omitempty"`
	TgDescription string    `json:"tg_description,omitempty"`
	InstanceID    string    `json:"instance_id,omitempty"`
}

// ListUnitEvents returns unit events matching the filter.
func (db *DB) ListUnitEvents(ctx context.Context, filter UnitEventFilter) ([]UnitEventAPI, int, error) {
	startTime := filter.StartTime
	if startTime == nil {
		t := time.Now().Add(-24 * time.Hour)
		startTime = &t
	}

	const fromClause = `FROM unit_events ue
		LEFT JOIN talkgroups tg ON tg.system_id = ue.system_id AND tg.tgid = ue.tgid`
	const whereClause = `
		WHERE ue.system_id = $1
		  AND ue.unit_rid = $2
		  AND ($3::text IS NULL OR ue.event_type = $3)
		  AND ($4::int IS NULL OR ue.tgid = $4)
		  AND ue.time >= $5
		  AND ($6::timestamptz IS NULL OR ue.time < $6)`
	args := []any{filter.SystemID, filter.UnitID, filter.EventType, filter.Tgid, *startTime, filter.EndTime}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := `
		SELECT ue.id, ue.event_type, ue.time, ue.system_id,
			ue.unit_rid, COALESCE(ue.unit_alpha_tag, ''),
			ue.tgid, COALESCE(tg.alpha_tag, ue.tg_alpha_tag, ''),
			COALESCE(tg.description, ''),
			COALESCE(ue.instance_id, '')
		` + fromClause + whereClause + `
		ORDER BY ue.time DESC
		LIMIT $7 OFFSET $8`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
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
			&e.Tgid, &e.TgAlphaTag, &e.TgDescription,
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
	startTime := filter.StartTime
	if startTime == nil {
		t := time.Now().Add(-1 * time.Hour)
		startTime = &t
	}

	const fromClause = `FROM unit_events ue
		JOIN systems s ON s.system_id = ue.system_id
		LEFT JOIN units u ON u.system_id = ue.system_id AND u.unit_id = ue.unit_rid
		LEFT JOIN talkgroups tg ON tg.system_id = ue.system_id AND tg.tgid = ue.tgid`
	const whereClause = `
		WHERE ($1::int[] IS NULL OR ue.system_id = ANY($1))
		  AND ($2::text[] IS NULL OR s.sysid = ANY($2))
		  AND ($3::int[] IS NULL OR ue.unit_rid = ANY($3))
		  AND ($4::text[] IS NULL OR ue.event_type = ANY($4))
		  AND ($5::int[] IS NULL OR ue.tgid = ANY($5))
		  AND ($6::boolean IS NULL OR ue.emergency = $6)
		  AND ue.time >= $7
		  AND ($8::timestamptz IS NULL OR ue.time < $8)`
	args := []any{
		pqIntArray(filter.SystemIDs), pqStringArray(filter.Sysids),
		pqIntArray(filter.UnitIDs), pqStringArray(filter.EventTypes),
		pqIntArray(filter.Tgids), filter.Emergency,
		*startTime, filter.EndTime,
	}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
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
			COALESCE(tg.description, ''),
			COALESCE(ue.instance_id, '')
		%s %s
		ORDER BY %s
		LIMIT $9 OFFSET $10
	`, fromClause, whereClause, orderBy)

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
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
			&e.Tgid, &e.TgAlphaTag, &e.TgDescription,
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
	return db.Q.InsertUnitEvent(ctx, sqlcdb.InsertUnitEventParams{
		EventType:            e.EventType,
		SystemID:             e.SystemID,
		UnitRid:              e.UnitRID,
		Time:                 pgtype.Timestamptz{Time: e.Time, Valid: true},
		Tgid:                 ptrIntToInt32(e.Tgid),
		UnitAlphaTag:         &e.UnitAlphaTag,
		TgAlphaTag:           &e.TgAlphaTag,
		CallNum:              ptrIntToInt32(e.CallNum),
		Freq:                 e.Freq,
		StartTime:            pgtzPtr(e.StartTime),
		StopTime:             pgtzPtr(e.StopTime),
		Encrypted:            e.Encrypted,
		Emergency:            e.Emergency,
		Position:             e.Position,
		Length:               e.Length,
		ErrorCount:           ptrIntToInt32(e.ErrorCount),
		SpikeCount:           ptrIntToInt32(e.SpikeCount),
		SampleCount:          ptrIntToInt32(e.SampleCount),
		TransmissionFilename: &e.TransmissionFilename,
		TalkgroupPatches:     int32sToInts(e.TalkgroupPatches),
		InstanceID:           &e.InstanceID,
		SysNum:               e.SysNum,
		SysName:              &e.SysName,
	})
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
	WentOff       bool // true if an "off" event occurred after this join
}

// LoadRecentAffiliations returns the most recent "join" event per (system_id, unit_rid)
// from the last 24 hours, with display names from JOINed tables.
func (db *DB) LoadRecentAffiliations(ctx context.Context) ([]AffiliationBackfillRow, error) {
	rows, err := db.Q.LoadRecentAffiliations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AffiliationBackfillRow, len(rows))
	for i, r := range rows {
		tgid := 0
		if r.Tgid != nil {
			tgid = int(*r.Tgid)
		}
		result[i] = AffiliationBackfillRow{
			SystemID:      r.SystemID,
			UnitRID:       r.UnitRid,
			Tgid:          tgid,
			UnitAlphaTag:  r.UnitAlphaTag,
			TgAlphaTag:    r.TgAlphaTag,
			TgDescription: r.TgDescription,
			TgTag:         r.TgTag,
			TgGroup:       r.TgGroup,
			SystemName:    r.SystemName,
			Sysid:         r.Sysid,
			Time:          r.Time.Time,
			WentOff:       r.WentOff,
		}
	}
	return result, nil
}
