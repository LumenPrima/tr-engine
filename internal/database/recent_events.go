package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type RecentEventAPI struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Time     time.Time `json:"time"`
	SystemID *int      `json:"system_id"`
	SiteID   *int      `json:"site_id"`
	Tgid     *int      `json:"tgid"`
	UnitID   *int      `json:"unit_id"`
}

type RecentEventFilter struct {
	SystemIDs []int
	SiteIDs   []int
	Tgids     []int
	UnitIDs   []int
	Types     []string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

func (db *DB) GetRecentEvents(ctx context.Context, filter RecentEventFilter) ([]RecentEventAPI, int, error) {
	startTime, endTime, err := normalizeRecentEventRange(filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, 0, err
	}

	const recentEventsCTE = `
		WITH recent_events AS (
			SELECT
				'call:' || c.call_id::text AS id,
				'call' AS type,
				COALESCE(c.stop_time, c.start_time) AS time,
				c.system_id,
				c.site_id,
				c.tgid,
				NULL::int AS unit_id
			FROM calls c
			WHERE ($1::int[] IS NULL OR c.system_id = ANY($1))
			  AND ($2::int[] IS NULL OR c.site_id = ANY($2))
			  AND ($3::int[] IS NULL OR c.tgid = ANY($3))
			  AND ($4::int[] IS NULL OR c.unit_ids && $4::int[])
			  AND ($5::text[] IS NULL OR 'call' = ANY($5))
			  AND COALESCE(c.stop_time, c.start_time) >= $6
			  AND COALESCE(c.stop_time, c.start_time) < $7

			UNION ALL

			SELECT
				'unit_event:' || ue.id::text AS id,
				'unit_event' AS type,
				ue.time,
				ue.system_id,
				ue.site_id,
				ue.tgid,
				ue.unit_rid AS unit_id
			FROM unit_events ue
			WHERE ($1::int[] IS NULL OR ue.system_id = ANY($1))
			  AND ($2::int[] IS NULL OR ue.site_id = ANY($2))
			  AND ($3::int[] IS NULL OR ue.tgid = ANY($3))
			  AND ($4::int[] IS NULL OR ue.unit_rid = ANY($4))
			  AND ($5::text[] IS NULL OR 'unit_event' = ANY($5))
			  AND ue.time >= $6
			  AND ue.time < $7
		)
	`

	args := []any{
		pqIntArray(filter.SystemIDs),
		pqIntArray(filter.SiteIDs),
		pqIntArray(filter.Tgids),
		pqIntArray(filter.UnitIDs),
		pqStringArray(filter.Types),
		startTime,
		endTime,
	}

	var total int
	countQuery := recentEventsCTE + `SELECT count(*) FROM recent_events`
	if err := db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := recentEventsCTE + `
		SELECT id, type, time, system_id, site_id, tgid, unit_id
		FROM recent_events
		ORDER BY time DESC
		LIMIT $8 OFFSET $9`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events, err := scanRecentEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func normalizeRecentEventRange(startTime, endTime *time.Time) (time.Time, time.Time, error) {
	effectiveEnd := time.Now().UTC()
	if endTime != nil {
		effectiveEnd = endTime.UTC()
	}

	effectiveStart := effectiveEnd.Add(-time.Hour)
	if startTime != nil {
		effectiveStart = startTime.UTC()
	}

	if effectiveStart.After(effectiveEnd) {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time must be before end_time")
	}
	if effectiveEnd.Sub(effectiveStart) > 24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("time range cannot exceed 24 hours")
	}
	return effectiveStart, effectiveEnd, nil
}

func scanRecentEvents(rows pgx.Rows) ([]RecentEventAPI, error) {
	var events []RecentEventAPI
	for rows.Next() {
		var event RecentEventAPI
		if err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.Time,
			&event.SystemID,
			&event.SiteID,
			&event.Tgid,
			&event.UnitID,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if events == nil {
		events = []RecentEventAPI{}
	}
	return events, rows.Err()
}
