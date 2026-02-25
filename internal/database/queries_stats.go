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
	stats, err := db.Q.GetOverallStats(ctx)
	if err != nil {
		return nil, err
	}

	s := &StatsResponse{
		Systems:            stats.Systems,
		Talkgroups:         stats.Talkgroups,
		Units:              stats.Units,
		TotalCalls:         stats.TotalCalls,
		Calls30d:           stats.Calls30d,
		Calls24h:           stats.Calls24h,
		Calls1h:            stats.Calls1h,
		TotalDurationHours: stats.TotalDurationHours,
	}

	activities, err := db.Q.GetSystemActivity(ctx)
	if err != nil {
		return nil, err
	}

	s.SystemActivity = make([]SystemActivity, len(activities))
	for i, a := range activities {
		s.SystemActivity[i] = SystemActivity{
			SystemID:         a.SystemID,
			SystemName:       a.SystemName,
			Sysid:            a.Sysid,
			Calls1h:          a.Calls1h,
			Calls24h:         a.Calls24h,
			ActiveTalkgroups: a.ActiveTalkgroups,
			ActiveUnits:      a.ActiveUnits,
		}
	}

	return s, nil
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
	const whereClause = `
		WHERE c.call_state = 'COMPLETED'
		  AND ($1::int[] IS NULL OR c.system_id = ANY($1))
		  AND ($2::int[] IS NULL OR c.site_id = ANY($2))
		  AND ($3::int[] IS NULL OR c.tgid = ANY($3))
		  AND ($4::timestamptz IS NULL OR c.start_time >= $4)
		  AND ($5::timestamptz IS NULL OR c.start_time < $5)`
	args := []any{
		pqIntArray(filter.SystemIDs), pqIntArray(filter.SiteIDs), pqIntArray(filter.Tgids),
		filter.After, filter.Before,
	}

	// Count distinct talkgroups
	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(DISTINCT (c.system_id, c.tgid)) FROM calls c"+whereClause, args...).Scan(&total); err != nil {
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
		LIMIT $6 OFFSET $7
	`, whereClause, orderBy)

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, limit, filter.Offset)...)
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
	query := `
		SELECT d.time, d.system_id, COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			d.decode_rate, d.control_channel
		FROM decode_rates d
		LEFT JOIN systems s ON s.system_id = d.system_id
		WHERE ($1::timestamptz IS NULL OR d.time >= $1)
		  AND ($2::timestamptz IS NULL OR d.time <= $2)
		ORDER BY d.time DESC LIMIT 1000`

	rows, err := db.Pool.Query(ctx, query, filter.StartTime, filter.EndTime)
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
