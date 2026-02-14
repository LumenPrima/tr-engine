package database

import (
	"context"
	"time"
)

// StatsResponse contains overall system statistics.
type StatsResponse struct {
	Systems            int              `json:"systems"`
	Talkgroups         int              `json:"talkgroups"`
	Units              int              `json:"units"`
	TotalCalls         int              `json:"total_calls"`
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
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '30 days'),
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '24 hours'),
			(SELECT count(*) FROM calls WHERE start_time > now() - interval '1 hour'),
			COALESCE((SELECT sum(duration) / 3600.0 FROM calls WHERE start_time > now() - interval '30 days' AND duration IS NOT NULL), 0)
	`).Scan(&s.Systems, &s.Talkgroups, &s.Units, &s.TotalCalls, &s.Calls24h, &s.Calls1h, &s.TotalDurationHours)
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
			(SELECT count(DISTINCT ct.src) FROM call_transmissions ct
				JOIN calls c ON c.call_id = ct.call_id AND c.start_time = ct.call_start_time
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
