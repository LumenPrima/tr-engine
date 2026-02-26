package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

// TalkgroupFilter specifies filters for listing talkgroups.
type TalkgroupFilter struct {
	SystemIDs  []int
	Sysids     []string
	Group      *string
	Search     *string
	Limit      int
	Offset     int
	Sort       string
}

// TalkgroupAPI represents a talkgroup for API responses.
type TalkgroupAPI struct {
	SystemID       int        `json:"system_id"`
	SystemName     string     `json:"system_name,omitempty"`
	Sysid          string     `json:"sysid,omitempty"`
	Tgid           int        `json:"tgid"`
	AlphaTag       string     `json:"alpha_tag,omitempty"`
	Tag            string     `json:"tag,omitempty"`
	Group          string     `json:"group,omitempty"`
	Description    string     `json:"description,omitempty"`
	Mode           *string    `json:"mode,omitempty"`
	Priority       *int       `json:"priority,omitempty"`
	FirstSeen      *time.Time `json:"first_seen,omitempty"`
	LastSeen       *time.Time `json:"last_seen,omitempty"`
	CallCount      int        `json:"call_count"`
	Calls1h        int        `json:"calls_1h"`
	Calls24h       int        `json:"calls_24h"`
	UnitCount      int        `json:"unit_count"`
	RelevanceScore *int       `json:"relevance_score,omitempty"`
}

// AmbiguousMatch represents a system where an ambiguous entity was found.
// Shared by talkgroups and units for composite ID resolution.
type AmbiguousMatch struct {
	SystemID   int    `json:"system_id"`
	SystemName string `json:"system_name"`
	Sysid      string `json:"sysid"`
}

// EncryptionStatAPI represents encryption stats per talkgroup.
type EncryptionStatAPI struct {
	SystemID       int     `json:"system_id"`
	SystemName     string  `json:"system_name,omitempty"`
	Sysid          string  `json:"sysid,omitempty"`
	Tgid           int     `json:"tgid"`
	TgAlphaTag     string  `json:"tg_alpha_tag,omitempty"`
	TgDescription  string  `json:"tg_description,omitempty"`
	TgTag          string  `json:"tg_tag,omitempty"`
	TgGroup        string  `json:"tg_group,omitempty"`
	EncryptedCount int     `json:"encrypted_count"`
	ClearCount     int     `json:"clear_count"`
	TotalCount     int     `json:"total_count"`
	EncryptedPct   float64 `json:"encrypted_pct"`
}

// TalkgroupDirectoryRow represents a row in the talkgroup_directory reference table.
type TalkgroupDirectoryRow struct {
	SystemID    int    `json:"system_id"`
	SystemName  string `json:"system_name,omitempty"`
	Tgid        int    `json:"tgid"`
	AlphaTag    string `json:"alpha_tag,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Description string `json:"description,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Category    string `json:"category,omitempty"`
	Priority    *int   `json:"priority,omitempty"`
}

// TalkgroupDirectoryFilter specifies filters for listing talkgroup directory entries.
type TalkgroupDirectoryFilter struct {
	SystemIDs []int
	Search    *string
	Category  *string
	Mode      *string
	Limit     int
	Offset    int
}

func talkgroupRowToAPI(r sqlcdb.GetTalkgroupByCompositeRow) TalkgroupAPI {
	tg := TalkgroupAPI{
		SystemID:    r.SystemID,
		SystemName:  r.SystemName,
		Sysid:       r.Sysid,
		Tgid:        r.Tgid,
		AlphaTag:    r.AlphaTag,
		Tag:         r.Tag,
		Group:       r.Group,
		Description: r.Description,
		Mode:        r.Mode,
		CallCount:   r.CallCount,
		Calls1h:     r.Calls1h,
		Calls24h:    r.Calls24h,
		UnitCount:   r.UnitCount,
	}
	if r.Priority != nil {
		v := int(*r.Priority)
		tg.Priority = &v
	}
	if r.FirstSeen.Valid {
		tg.FirstSeen = &r.FirstSeen.Time
	}
	if r.LastSeen.Valid {
		tg.LastSeen = &r.LastSeen.Time
	}
	return tg
}

// GetTalkgroupByComposite returns a single talkgroup by system_id and tgid.
func (db *DB) GetTalkgroupByComposite(ctx context.Context, systemID, tgid int) (*TalkgroupAPI, error) {
	row, err := db.Q.GetTalkgroupByComposite(ctx, sqlcdb.GetTalkgroupByCompositeParams{
		SystemID: systemID,
		Tgid:     tgid,
	})
	if err != nil {
		return nil, err
	}
	tg := talkgroupRowToAPI(row)
	return &tg, nil
}

// FindTalkgroupSystems returns systems where a talkgroup ID exists (for ambiguity resolution).
func (db *DB) FindTalkgroupSystems(ctx context.Context, tgid int) ([]AmbiguousMatch, error) {
	rows, err := db.Q.FindTalkgroupSystems(ctx, tgid)
	if err != nil {
		return nil, err
	}
	matches := make([]AmbiguousMatch, len(rows))
	for i, r := range rows {
		matches[i] = AmbiguousMatch{
			SystemID:   r.SystemID,
			SystemName: r.SystemName,
			Sysid:      r.Sysid,
		}
	}
	return matches, nil
}

// UpdateTalkgroupFields updates mutable talkgroup fields.
func (db *DB) UpdateTalkgroupFields(ctx context.Context, systemID, tgid int,
	alphaTag, alphaTagSource, description, group, tag *string, priority *int) error {

	atVal := ""
	if alphaTag != nil {
		atVal = *alphaTag
	}
	atsVal := ""
	if alphaTagSource != nil {
		atsVal = *alphaTagSource
	}
	descVal := ""
	if description != nil {
		descVal = *description
	}
	groupVal := ""
	if group != nil {
		groupVal = *group
	}
	tagVal := ""
	if tag != nil {
		tagVal = *tag
	}
	prioVal := -1
	if priority != nil {
		prioVal = *priority
	}

	return db.Q.UpdateTalkgroupFields(ctx, sqlcdb.UpdateTalkgroupFieldsParams{
		AlphaTag:       atVal,
		AlphaTagSource: atsVal,
		Description:    descVal,
		TgGroup:        groupVal,
		Tag:            tagVal,
		Priority:       prioVal,
		SystemID:       systemID,
		Tgid:           tgid,
	})
}

// UpsertTalkgroup inserts or updates a talkgroup, never overwriting good data with empty strings.
// Returns the effective alpha_tag from the database (respects manual > csv > mqtt priority).
func (db *DB) UpsertTalkgroup(ctx context.Context, systemID, tgid int, alphaTag, tag, group, description string, eventTime time.Time) (string, error) {
	return db.Q.UpsertTalkgroup(ctx, sqlcdb.UpsertTalkgroupParams{
		SystemID:    systemID,
		Tgid:        tgid,
		AlphaTag:    &alphaTag,
		Tag:         &tag,
		TgGroup:     &group,
		Description: &description,
		EventTime:   pgtype.Timestamptz{Time: eventTime, Valid: true},
	})
}

// UpsertTalkgroupDirectory inserts or updates a talkgroup directory entry.
func (db *DB) UpsertTalkgroupDirectory(ctx context.Context, systemID, tgid int, alphaTag, mode, description, tag, category string, priority int) error {
	var prio *int32
	if priority > 0 {
		v := int32(priority)
		prio = &v
	}
	return db.Q.UpsertTalkgroupDirectory(ctx, sqlcdb.UpsertTalkgroupDirectoryParams{
		SystemID:    systemID,
		Tgid:        tgid,
		AlphaTag:    &alphaTag,
		Mode:        &mode,
		Description: &description,
		Tag:         &tag,
		Category:    &category,
		Priority:    prio,
	})
}

// EnrichTalkgroupsFromDirectory fills missing talkgroup fields from the directory.
// If tgid is 0, enriches all heard talkgroups in the system (bulk mode).
// If tgid > 0, enriches only that specific talkgroup (per-call mode).
func (db *DB) EnrichTalkgroupsFromDirectory(ctx context.Context, systemID, tgid int) (int64, error) {
	return db.Q.EnrichTalkgroupsFromDirectory(ctx, sqlcdb.EnrichTalkgroupsFromDirectoryParams{
		SystemID: systemID,
		Tgid:     tgid,
	})
}

// ListTalkgroups returns talkgroups with cached stats.
func (db *DB) ListTalkgroups(ctx context.Context, filter TalkgroupFilter) ([]TalkgroupAPI, int, error) {
	const whereClause = `
		WHERE ($1::int[] IS NULL OR t.system_id = ANY($1))
		  AND ($2::text[] IS NULL OR s.sysid = ANY($2))
		  AND ($3::text IS NULL OR t."group" = $3)
		  AND ($4::text IS NULL OR t.alpha_tag ILIKE '%' || $4 || '%' OR t.description ILIKE '%' || $4 || '%' OR t.tag ILIKE '%' || $4 || '%' OR t."group" ILIKE '%' || $4 || '%' OR t.tgid::text = $4)`
	args := []any{pqIntArray(filter.SystemIDs), pqStringArray(filter.Sysids), filter.Group, filter.Search}

	// Count
	var total int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM talkgroups t JOIN systems s ON s.system_id = t.system_id AND s.deleted_at IS NULL`+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort — remap stats sort fields to cached columns
	orderBy := "t.alpha_tag ASC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	dataQuery := fmt.Sprintf(`
		SELECT t.system_id, COALESCE(s.name, '') AS system_name, s.sysid,
			t.tgid, COALESCE(t.alpha_tag, '') AS alpha_tag, COALESCE(t.tag, '') AS tag,
			COALESCE(t."group", '') AS "group", COALESCE(t.description, '') AS description,
			t.mode, t.priority, t.first_seen, t.last_seen,
			t.call_count_30d, t.calls_1h, t.calls_24h, t.unit_count_30d
		FROM talkgroups t
		JOIN systems s ON s.system_id = t.system_id AND s.deleted_at IS NULL
		%s
		ORDER BY %s
		LIMIT $5 OFFSET $6
	`, whereClause, orderBy)

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var talkgroups []TalkgroupAPI
	for rows.Next() {
		var tg TalkgroupAPI
		if err := rows.Scan(
			&tg.SystemID, &tg.SystemName, &tg.Sysid,
			&tg.Tgid, &tg.AlphaTag, &tg.Tag, &tg.Group, &tg.Description,
			&tg.Mode, &tg.Priority, &tg.FirstSeen, &tg.LastSeen,
			&tg.CallCount, &tg.Calls1h, &tg.Calls24h, &tg.UnitCount,
		); err != nil {
			return nil, 0, err
		}

		// Compute relevance score if searching
		if filter.Search != nil {
			search := *filter.Search
			score := 10 // contains
			if tg.AlphaTag == search || strconv.Itoa(tg.Tgid) == search {
				score = 100 // exact
			} else if len(search) > 0 && len(tg.AlphaTag) >= len(search) && tg.AlphaTag[:len(search)] == search {
				score = 50 // prefix
			}
			tg.RelevanceScore = &score
		}

		talkgroups = append(talkgroups, tg)
	}
	if talkgroups == nil {
		talkgroups = []TalkgroupAPI{}
	}
	return talkgroups, total, rows.Err()
}

// ListTalkgroupUnits returns units affiliated with a talkgroup within a time window.
func (db *DB) ListTalkgroupUnits(ctx context.Context, systemID, tgid, windowMinutes, limit, offset int) ([]UnitAPI, int, error) {
	window := strconv.Itoa(windowMinutes) + " minutes"

	var total int
	err := db.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT u)
		FROM calls c, unnest(c.unit_ids) AS u
		WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
	`, systemID, tgid, window).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Pool.Query(ctx, `
		WITH unit_calls AS (
			SELECT uid, count(*) AS call_count
			FROM calls c, unnest(c.unit_ids) AS uid
			WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
			GROUP BY uid
		)
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid,
			u.unit_id, COALESCE(u.alpha_tag, ''), COALESCE(u.alpha_tag_source, ''),
			u.first_seen, u.last_seen,
			u.last_event_type, u.last_event_time, u.last_event_tgid,
			uc.call_count
		FROM units u
		JOIN systems s ON s.system_id = u.system_id
		JOIN unit_calls uc ON uc.uid = u.unit_id
		ORDER BY uc.call_count DESC, u.unit_id
		LIMIT $4 OFFSET $5
	`, systemID, tgid, window, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var units []UnitAPI
	for rows.Next() {
		var u UnitAPI
		if err := rows.Scan(
			&u.SystemID, &u.SystemName, &u.Sysid,
			&u.UnitID, &u.AlphaTag, &u.AlphaTagSource,
			&u.FirstSeen, &u.LastSeen,
			&u.LastEventType, &u.LastEventTime, &u.LastEventTgid,
			&u.CallCount,
		); err != nil {
			return nil, 0, err
		}
		units = append(units, u)
	}
	if units == nil {
		units = []UnitAPI{}
	}
	return units, total, rows.Err()
}

// GetEncryptionStats returns encryption stats per talkgroup.
func (db *DB) GetEncryptionStats(ctx context.Context, hours int, sysid string) ([]EncryptionStatAPI, error) {
	hoursInterval := strconv.Itoa(hours) + " hours"

	query := `
		SELECT c.system_id, COALESCE(s.name, ''), s.sysid,
			c.tgid, COALESCE(t.alpha_tag, ''), COALESCE(t.description, ''),
			COALESCE(t.tag, ''), COALESCE(t."group", ''),
			count(*) FILTER (WHERE c.encrypted) AS encrypted_count,
			count(*) FILTER (WHERE NOT c.encrypted OR c.encrypted IS NULL) AS clear_count,
			count(*) AS total_count
		FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		LEFT JOIN talkgroups t ON t.system_id = c.system_id AND t.tgid = c.tgid
		WHERE c.start_time > now() - $1::interval
		  AND ($2::text IS NULL OR s.sysid = $2)
		GROUP BY c.system_id, s.name, s.sysid, c.tgid, t.alpha_tag, t.description, t.tag, t."group"
		ORDER BY total_count DESC`

	rows, err := db.Pool.Query(ctx, query, hoursInterval, pqString(sysid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []EncryptionStatAPI
	for rows.Next() {
		var es EncryptionStatAPI
		if err := rows.Scan(
			&es.SystemID, &es.SystemName, &es.Sysid,
			&es.Tgid, &es.TgAlphaTag, &es.TgDescription, &es.TgTag, &es.TgGroup,
			&es.EncryptedCount, &es.ClearCount, &es.TotalCount,
		); err != nil {
			return nil, err
		}
		if es.TotalCount > 0 {
			es.EncryptedPct = float64(es.EncryptedCount) / float64(es.TotalCount) * 100
		}
		stats = append(stats, es)
	}
	if stats == nil {
		stats = []EncryptionStatAPI{}
	}
	return stats, rows.Err()
}

// SearchTalkgroupDirectory searches the talkgroup directory reference table.
func (db *DB) SearchTalkgroupDirectory(ctx context.Context, filter TalkgroupDirectoryFilter) ([]TalkgroupDirectoryRow, int, error) {
	// Convert empty-string filters to nil so IS NULL OR skips them
	var search, category, mode any
	if filter.Search != nil && *filter.Search != "" {
		search = *filter.Search
	}
	if filter.Category != nil && *filter.Category != "" {
		category = *filter.Category
	}
	if filter.Mode != nil && *filter.Mode != "" {
		mode = *filter.Mode
	}

	const whereClause = `
		WHERE ($1::int[] IS NULL OR td.system_id = ANY($1))
		  AND ($2::text IS NULL OR td.search_vector @@ plainto_tsquery('english', $2))
		  AND ($3::text IS NULL OR td.category = $3)
		  AND ($4::text IS NULL OR td.mode = $4)`
	args := []any{pqIntArray(filter.SystemIDs), search, category, mode}

	// Count
	var total int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM talkgroup_directory td`+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT td.system_id, COALESCE(s.name, ''), td.tgid,
			COALESCE(td.alpha_tag, ''), COALESCE(td.mode, ''),
			COALESCE(td.description, ''), COALESCE(td.tag, ''),
			COALESCE(td.category, ''), td.priority
		FROM talkgroup_directory td
		LEFT JOIN systems s ON s.system_id = td.system_id
	` + whereClause + `
		ORDER BY td.system_id, td.tgid
		LIMIT $5 OFFSET $6`

	rows, err := db.Pool.Query(ctx, query, append(args, limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []TalkgroupDirectoryRow
	for rows.Next() {
		var r TalkgroupDirectoryRow
		if err := rows.Scan(&r.SystemID, &r.SystemName, &r.Tgid,
			&r.AlphaTag, &r.Mode, &r.Description, &r.Tag, &r.Category, &r.Priority); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}

	return results, total, rows.Err()
}

// RefreshTalkgroupStats updates the cached stats columns on all talkgroups.
// Designed to run periodically from the maintenance task (every few minutes).
func (db *DB) RefreshTalkgroupStats(ctx context.Context) (int64, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE talkgroups t SET
			call_count_30d = COALESCE(cs.call_count, 0),
			calls_1h       = COALESCE(cs.calls_1h, 0),
			calls_24h      = COALESCE(cs.calls_24h, 0),
			unit_count_30d = COALESCE(us.unit_count, 0),
			stats_updated_at = now()
		FROM (
			SELECT system_id, tgid,
				count(*)::int AS call_count,
				count(*) FILTER (WHERE start_time > now() - interval '1 hour')::int AS calls_1h,
				count(*) FILTER (WHERE start_time > now() - interval '24 hours')::int AS calls_24h
			FROM calls
			WHERE start_time > now() - interval '30 days'
			GROUP BY system_id, tgid
		) cs
		RIGHT JOIN (
			SELECT system_id, tgid, count(DISTINCT unit_rid)::int AS unit_count
			FROM unit_events
			WHERE tgid IS NOT NULL AND time > now() - interval '30 days'
			GROUP BY system_id, tgid
		) us USING (system_id, tgid)
		WHERE t.system_id = us.system_id AND t.tgid = us.tgid
		  AND (t.call_count_30d IS DISTINCT FROM COALESCE(cs.call_count, 0)
			OR t.calls_1h IS DISTINCT FROM COALESCE(cs.calls_1h, 0)
			OR t.calls_24h IS DISTINCT FROM COALESCE(cs.calls_24h, 0)
			OR t.unit_count_30d IS DISTINCT FROM COALESCE(us.unit_count, 0))
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
