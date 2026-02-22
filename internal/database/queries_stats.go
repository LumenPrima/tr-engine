package database

import (
	"context"
	"fmt"
	"time"
)

// StatsResponse contains overall system statistics.
type StatsResponse struct {
	Systems            int              `json:"systems"`
	Talkgroups         int              `json:"talkgroups"`
	Units              int              `json:"units"`
	TotalCalls         int              `json:"total_calls"`
	Calls30d           int              `json:"calls_30d"`
	Calls24h           int              `json:"calls_24h"`
	Calls1h            int              `json:"calls_1h"`
	TotalDurationHours float64          `json:"total_duration_hours"`
	SystemActivity     []SystemActivity `json:"system_activity"`
}

// SystemActivity contains per-system activity breakdown.
type SystemActivity struct {
	SystemID         int    `json:"system_id"`
	SystemName       string `json:"system_name"`
	Sysid            string `json:"sysid"`
	Calls1h          int    `json:"calls_1h"`
	Calls24h         int    `json:"calls_24h"`
	ActiveTalkgroups int    `json:"active_talkgroups"`
	ActiveUnits      int    `json:"active_units"`
}

// GetStats returns overall system statistics.
func (db *DB) GetStats(ctx context.Context) (*StatsResponse, error) {
	var s StatsResponse
	err := db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM systems WHERE deleted_at IS NULL),
			(SELECT count(*) FROM talkgroups),
			(SELECT count(*) FROM units),
			(SELECT count(*) FROM calls),
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '30 days'),
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '24 hours'),
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '1 hour'),
			COALESCE((SELECT sum(duration) / 3600.0 FROM calls WHERE duration IS NOT NULL), 0)
	`).Scan(&s.Systems, &s.Talkgroups, &s.Units, &s.TotalCalls, &s.Calls30d, &s.Calls24h, &s.Calls1h, &s.TotalDurationHours)
	if err != nil {
		return nil, err
	}

	// Per-system activity
	rows, err := db.Pool.Query(ctx, `
		SELECT
			s.system_id, COALESCE(s.name, ''), s.sysid,
			(SELECT count(*) FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '1 hour'),
			(SELECT count(*) FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '24 hours'),
			(SELECT count(DISTINCT tgid) FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '1 hour'),
			(SELECT count(DISTINCT u) FROM calls c, unnest(c.unit_ids) AS u
				WHERE c.system_id = s.system_id AND c.start_time > now() - interval '1 hour')
		FROM systems s
		WHERE s.deleted_at IS NULL
		ORDER BY s.system_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sa SystemActivity
		if err := rows.Scan(&sa.SystemID, &sa.SystemName, &sa.Sysid,
			&sa.Calls1h, &sa.Calls24h, &sa.ActiveTalkgroups, &sa.ActiveUnits); err != nil {
			return nil, err
		}
		s.SystemActivity = append(s.SystemActivity, sa)
	}
	if s.SystemActivity == nil {
		s.SystemActivity = []SystemActivity{}
	}

	return &s, rows.Err()
}

// TalkgroupActivityFilter specifies filters for the talkgroup activity summary.
type TalkgroupActivityFilter struct {
	SystemIDs []int
	SiteIDs   []int
	Tgids     []int
	After     *time.Time
	Before    *time.Time
	Limit     int
	Offset    int
	SortField string // "calls", "duration", "tgid"
}

// TalkgroupActivity represents call counts grouped by talkgroup.
type TalkgroupActivity struct {
	SystemID       int       `json:"system_id"`
	SystemName     string    `json:"system_name"`
	Tgid           int       `json:"tgid"`
	TgAlphaTag     string    `json:"tg_alpha_tag,omitempty"`
	TgDescription  string    `json:"tg_description,omitempty"`
	TgTag          string    `json:"tg_tag,omitempty"`
	TgGroup        string    `json:"tg_group,omitempty"`
	CallCount      int       `json:"call_count"`
	TotalDuration  float64   `json:"total_duration"`
	EmergencyCount int       `json:"emergency_count"`
	FirstCall      time.Time `json:"first_call"`
	LastCall       time.Time `json:"last_call"`
}

// GetTalkgroupActivity returns call counts grouped by talkgroup for a time range.
func (db *DB) GetTalkgroupActivity(ctx context.Context, filter TalkgroupActivityFilter) ([]TalkgroupActivity, int, error) {
	qb := newQueryBuilder()
	qb.AddRaw("c.call_state = 'COMPLETED'")

	if len(filter.SystemIDs) > 0 {
		qb.Add("c.system_id = ANY(%s)", filter.SystemIDs)
	}
	if len(filter.SiteIDs) > 0 {
		qb.Add("c.site_id = ANY(%s)", filter.SiteIDs)
	}
	if len(filter.Tgids) > 0 {
		qb.Add("c.tgid = ANY(%s)", filter.Tgids)
	}
	if filter.After != nil {
		qb.Add("c.start_time >= %s", *filter.After)
	}
	if filter.Before != nil {
		qb.Add("c.start_time < %s", *filter.Before)
	}

	whereClause := qb.WhereClause()

	// Count distinct talkgroups
	var total int
	countQuery := "SELECT count(DISTINCT (c.system_id, c.tgid)) FROM calls c" + whereClause
	if err := db.Pool.QueryRow(ctx, countQuery, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort
	orderBy := "count(*) DESC"
	switch filter.SortField {
	case "duration":
		orderBy = "COALESCE(sum(c.duration), 0) DESC"
	case "tgid":
		orderBy = "c.tgid ASC"
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	dataQuery := fmt.Sprintf(`
		SELECT c.system_id, COALESCE(c.system_name, ''),
			c.tgid, COALESCE(c.tg_alpha_tag, ''), COALESCE(c.tg_description, ''),
			COALESCE(c.tg_tag, ''), COALESCE(c.tg_group, ''),
			count(*), COALESCE(sum(c.duration), 0),
			count(*) FILTER (WHERE c.emergency),
			min(c.start_time), max(c.start_time)
		FROM calls c
		%s
		GROUP BY c.system_id, COALESCE(c.system_name, ''),
			c.tgid, COALESCE(c.tg_alpha_tag, ''), COALESCE(c.tg_description, ''),
			COALESCE(c.tg_tag, ''), COALESCE(c.tg_group, '')
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, whereClause, orderBy, limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []TalkgroupActivity
	for rows.Next() {
		var a TalkgroupActivity
		if err := rows.Scan(
			&a.SystemID, &a.SystemName,
			&a.Tgid, &a.TgAlphaTag, &a.TgDescription,
			&a.TgTag, &a.TgGroup,
			&a.CallCount, &a.TotalDuration,
			&a.EmergencyCount,
			&a.FirstCall, &a.LastCall,
		); err != nil {
			return nil, 0, err
		}
		results = append(results, a)
	}
	if results == nil {
		results = []TalkgroupActivity{}
	}
	return results, total, rows.Err()
}

// DecodeRateFilter specifies time range for decode rate queries.
type DecodeRateFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
}

// DecodeRateAPI represents a decode rate for API responses.
type DecodeRateAPI struct {
	Time          time.Time `json:"time"`
	SystemID      *int      `json:"system_id,omitempty"`
	SystemName    string    `json:"system_name,omitempty"`
	Sysid         string    `json:"sysid,omitempty"`
	DecodeRate    float32   `json:"decode_rate"`
	TotalMessages int64     `json:"total_messages"`
}

// GetDecodeRates returns decode rate measurements.
func (db *DB) GetDecodeRates(ctx context.Context, filter DecodeRateFilter) ([]DecodeRateAPI, error) {
	qb := newQueryBuilder()

	query := `
		SELECT d.time, d.system_id, COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			d.decode_rate, d.control_channel
		FROM decode_rates d
		LEFT JOIN systems s ON s.system_id = d.system_id
	`

	if filter.StartTime != nil {
		qb.Add("d.time >= %s", *filter.StartTime)
	} else {
		qb.Add("d.time >= %s", time.Now().Add(-24*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("d.time <= %s", *filter.EndTime)
	}

	query += qb.WhereClause() + " ORDER BY d.time DESC LIMIT 1000"
	rows, err := db.Pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rates []DecodeRateAPI
	for rows.Next() {
		var r DecodeRateAPI
		if err := rows.Scan(&r.Time, &r.SystemID, &r.SystemName, &r.Sysid,
			&r.DecodeRate, &r.TotalMessages); err != nil {
			return nil, err
		}
		rates = append(rates, r)
	}
	if rates == nil {
		rates = []DecodeRateAPI{}
	}
	return rates, rows.Err()
}
