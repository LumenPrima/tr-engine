package database

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// TalkgroupFilter specifies filters for listing talkgroups.
type TalkgroupFilter struct {
	SystemIDs []int
	Sysids    []string
	Group     *string
	Search    *string
	Limit     int
	Offset    int
	Sort      string
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

// ListTalkgroups returns talkgroups with embedded stats.
func (db *DB) ListTalkgroups(ctx context.Context, filter TalkgroupFilter) ([]TalkgroupAPI, int, error) {
	qb := newQueryBuilder()

	if len(filter.SystemIDs) > 0 {
		qb.Add("t.system_id = ANY(%s)", filter.SystemIDs)
	}
	if len(filter.Sysids) > 0 {
		qb.Add("s.sysid = ANY(%s)", filter.Sysids)
	}
	if filter.Group != nil {
		qb.Add(`t."group" = %s`, *filter.Group)
	}
	if filter.Search != nil {
		qb.Add("(t.alpha_tag ILIKE '%%' || %s || '%%' OR t.description ILIKE '%%' || %s || '%%' OR t.tag ILIKE '%%' || %s || '%%' OR t.\"group\" ILIKE '%%' || %s || '%%' OR t.tgid::text = %s)", *filter.Search)
		// This is complex — let's simplify by using a single-param search approach
		// Re-do: remove the last Add and use a simpler approach
		qb.where = qb.where[:len(qb.where)-1]
		qb.args = qb.args[:len(qb.args)-1]
		qb.argIdx--

		paramIdx := qb.argIdx
		qb.args = append(qb.args, *filter.Search)
		qb.where = append(qb.where, fmt.Sprintf(
			`(t.alpha_tag ILIKE '%%' || $%d || '%%' OR t.description ILIKE '%%' || $%d || '%%' OR t.tag ILIKE '%%' || $%d || '%%' OR t."group" ILIKE '%%' || $%d || '%%' OR t.tgid::text = $%d)`,
			paramIdx, paramIdx, paramIdx, paramIdx, paramIdx))
		qb.argIdx++
	}

	fromClause := `
		FROM talkgroups t
		JOIN systems s ON s.system_id = t.system_id AND s.deleted_at IS NULL
		LEFT JOIN LATERAL (
			SELECT count(*) AS call_count,
				count(*) FILTER (WHERE start_time > now() - interval '1 hour') AS calls_1h,
				count(*) FILTER (WHERE start_time > now() - interval '24 hours') AS calls_24h
			FROM calls c
			WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '30 days'
		) ts ON true
		LEFT JOIN LATERAL (
			SELECT count(DISTINCT u) AS unit_count
			FROM calls c, unnest(c.unit_ids) AS u
			WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '30 days'
		) us ON true
	`
	whereClause := qb.WhereClause()

	// Count
	var total int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM talkgroups t JOIN systems s ON s.system_id = t.system_id AND s.deleted_at IS NULL`+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort
	orderBy := "t.alpha_tag ASC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	dataQuery := fmt.Sprintf(`
		SELECT t.system_id, COALESCE(s.name, ''), s.sysid,
			t.tgid, COALESCE(t.alpha_tag, ''), COALESCE(t.tag, ''),
			COALESCE(t."group", ''), COALESCE(t.description, ''),
			t.mode, t.priority, t.first_seen, t.last_seen,
			COALESCE(ts.call_count, 0), COALESCE(ts.calls_1h, 0), COALESCE(ts.calls_24h, 0),
			COALESCE(us.unit_count, 0)
		%s %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, orderBy, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
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

// GetTalkgroupByComposite returns a single talkgroup by system_id and tgid.
func (db *DB) GetTalkgroupByComposite(ctx context.Context, systemID, tgid int) (*TalkgroupAPI, error) {
	var tg TalkgroupAPI
	err := db.Pool.QueryRow(ctx, `
		SELECT t.system_id, COALESCE(s.name, ''), s.sysid,
			t.tgid, COALESCE(t.alpha_tag, ''), COALESCE(t.tag, ''),
			COALESCE(t."group", ''), COALESCE(t.description, ''),
			t.mode, t.priority, t.first_seen, t.last_seen,
			(SELECT count(*) FROM calls c WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '30 days'),
			(SELECT count(*) FROM calls c WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '1 hour'),
			(SELECT count(*) FROM calls c WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '24 hours'),
			(SELECT count(DISTINCT u) FROM calls c, unnest(c.unit_ids) AS u
				WHERE c.system_id = t.system_id AND c.tgid = t.tgid AND c.start_time > now() - interval '30 days')
		FROM talkgroups t
		JOIN systems s ON s.system_id = t.system_id
		WHERE t.system_id = $1 AND t.tgid = $2
	`, systemID, tgid).Scan(
		&tg.SystemID, &tg.SystemName, &tg.Sysid,
		&tg.Tgid, &tg.AlphaTag, &tg.Tag, &tg.Group, &tg.Description,
		&tg.Mode, &tg.Priority, &tg.FirstSeen, &tg.LastSeen,
		&tg.CallCount, &tg.Calls1h, &tg.Calls24h, &tg.UnitCount,
	)
	if err != nil {
		return nil, err
	}
	return &tg, nil
}

// FindTalkgroupSystems returns systems where a talkgroup ID exists (for ambiguity resolution).
func (db *DB) FindTalkgroupSystems(ctx context.Context, tgid int) ([]AmbiguousMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.system_id, COALESCE(s.name, ''), s.sysid
		FROM talkgroups t
		JOIN systems s ON s.system_id = t.system_id AND s.deleted_at IS NULL
		WHERE t.tgid = $1
	`, tgid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []AmbiguousMatch
	for rows.Next() {
		var m AmbiguousMatch
		if err := rows.Scan(&m.SystemID, &m.SystemName, &m.Sysid); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// AmbiguousMatch represents a system where an ambiguous entity was found.
// Shared by talkgroups and units for composite ID resolution.
type AmbiguousMatch struct {
	SystemID   int    `json:"system_id"`
	SystemName string `json:"system_name"`
	Sysid      string `json:"sysid"`
}

// UpdateTalkgroupFields updates mutable talkgroup fields.
func (db *DB) UpdateTalkgroupFields(ctx context.Context, systemID, tgid int,
	alphaTag, description, group, tag *string, priority *int) error {

	atVal := ""
	if alphaTag != nil {
		atVal = *alphaTag
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

	_, err := db.Pool.Exec(ctx, `
		UPDATE talkgroups SET
			alpha_tag   = CASE WHEN $3 <> '' THEN $3 ELSE alpha_tag END,
			description = CASE WHEN $4 <> '' THEN $4 ELSE description END,
			"group"     = CASE WHEN $5 <> '' THEN $5 ELSE "group" END,
			tag         = CASE WHEN $6 <> '' THEN $6 ELSE tag END,
			priority    = CASE WHEN $7 >= 0 THEN $7 ELSE priority END
		WHERE system_id = $1 AND tgid = $2
	`, systemID, tgid, atVal, descVal, groupVal, tagVal, prioVal)
	return err
}

// ListTalkgroupUnits returns units affiliated with a talkgroup within a time window.
func (db *DB) ListTalkgroupUnits(ctx context.Context, systemID, tgid, windowMinutes, limit, offset int) ([]UnitAPI, int, error) {
	window := fmt.Sprintf("%d minutes", windowMinutes)

	var total int
	err := db.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT u)
		FROM calls c, unnest(c.unit_ids) AS u
		WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
	`, systemID, tgid, window).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid,
			u.unit_id, COALESCE(u.alpha_tag, ''), COALESCE(u.alpha_tag_source, ''),
			u.first_seen, u.last_seen,
			u.last_event_type, u.last_event_time, u.last_event_tgid
		FROM units u
		JOIN systems s ON s.system_id = u.system_id
		WHERE u.system_id = $1 AND u.unit_id IN (
			SELECT DISTINCT uid
			FROM calls c, unnest(c.unit_ids) AS uid
			WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
		)
		ORDER BY u.unit_id
		LIMIT %d OFFSET %d
	`, limit, offset), systemID, tgid, window)
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

// EncryptionStatAPI represents encryption stats per talkgroup.
type EncryptionStatAPI struct {
	SystemID      int     `json:"system_id"`
	SystemName    string  `json:"system_name,omitempty"`
	Sysid         string  `json:"sysid,omitempty"`
	Tgid          int     `json:"tgid"`
	TgAlphaTag    string  `json:"tg_alpha_tag,omitempty"`
	TgDescription string  `json:"tg_description,omitempty"`
	TgTag         string  `json:"tg_tag,omitempty"`
	TgGroup       string  `json:"tg_group,omitempty"`
	EncryptedCount int    `json:"encrypted_count"`
	ClearCount    int     `json:"clear_count"`
	TotalCount    int     `json:"total_count"`
	EncryptedPct  float64 `json:"encrypted_pct"`
}

// GetEncryptionStats returns encryption stats per talkgroup.
func (db *DB) GetEncryptionStats(ctx context.Context, hours int, sysid string) ([]EncryptionStatAPI, error) {
	qb := newQueryBuilder()
	qb.Add("c.start_time > now() - (%s || ' hours')::interval", strconv.Itoa(hours))
	if sysid != "" {
		qb.Add("s.sysid = %s", sysid)
	}

	query := fmt.Sprintf(`
		SELECT c.system_id, COALESCE(s.name, ''), s.sysid,
			c.tgid, COALESCE(t.alpha_tag, ''), COALESCE(t.description, ''),
			COALESCE(t.tag, ''), COALESCE(t."group", ''),
			count(*) FILTER (WHERE c.encrypted) AS encrypted_count,
			count(*) FILTER (WHERE NOT c.encrypted OR c.encrypted IS NULL) AS clear_count,
			count(*) AS total_count
		FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		LEFT JOIN talkgroups t ON t.system_id = c.system_id AND t.tgid = c.tgid
		%s
		GROUP BY c.system_id, s.name, s.sysid, c.tgid, t.alpha_tag, t.description, t.tag, t."group"
		ORDER BY total_count DESC
	`, qb.WhereClause())

	rows, err := db.Pool.Query(ctx, query, qb.Args()...)
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

// UpsertTalkgroup inserts or updates a talkgroup, never overwriting good data with empty strings.
func (db *DB) UpsertTalkgroup(ctx context.Context, systemID, tgid int, alphaTag, tag, group, description string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO talkgroups (system_id, tgid, alpha_tag, tag, "group", description, first_seen, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		ON CONFLICT (system_id, tgid) DO UPDATE SET
			alpha_tag   = COALESCE(NULLIF($3, ''), talkgroups.alpha_tag),
			tag         = COALESCE(NULLIF($4, ''), talkgroups.tag),
			"group"     = COALESCE(NULLIF($5, ''), talkgroups."group"),
			description = COALESCE(NULLIF($6, ''), talkgroups.description),
			last_seen   = now()
	`, systemID, tgid, alphaTag, tag, group, description)
	return err
}

// TalkgroupDirectoryRow represents a row in the talkgroup_directory reference table.
type TalkgroupDirectoryRow struct {
	SystemID    int     `json:"system_id"`
	SystemName  string  `json:"system_name,omitempty"`
	Tgid        int     `json:"tgid"`
	AlphaTag    string  `json:"alpha_tag,omitempty"`
	Mode        string  `json:"mode,omitempty"`
	Description string  `json:"description,omitempty"`
	Tag         string  `json:"tag,omitempty"`
	Category    string  `json:"category,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
}

// UpsertTalkgroupDirectory inserts or updates a talkgroup directory entry.
func (db *DB) UpsertTalkgroupDirectory(ctx context.Context, systemID, tgid int, alphaTag, mode, description, tag, category string, priority int) error {
	var prio *int
	if priority > 0 {
		prio = &priority
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO talkgroup_directory (system_id, tgid, alpha_tag, mode, description, tag, category, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (system_id, tgid) DO UPDATE SET
			alpha_tag   = COALESCE(NULLIF($3, ''), talkgroup_directory.alpha_tag),
			mode        = COALESCE(NULLIF($4, ''), talkgroup_directory.mode),
			description = COALESCE(NULLIF($5, ''), talkgroup_directory.description),
			tag         = COALESCE(NULLIF($6, ''), talkgroup_directory.tag),
			category    = COALESCE(NULLIF($7, ''), talkgroup_directory.category),
			priority    = COALESCE($8, talkgroup_directory.priority),
			imported_at = now()
	`, systemID, tgid, alphaTag, mode, description, tag, category, prio)
	return err
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

// SearchTalkgroupDirectory searches the talkgroup directory reference table.
func (db *DB) SearchTalkgroupDirectory(ctx context.Context, filter TalkgroupDirectoryFilter) ([]TalkgroupDirectoryRow, int, error) {
	qb := newQueryBuilder()

	if len(filter.SystemIDs) > 0 {
		qb.Add("td.system_id = ANY(%s)", filter.SystemIDs)
	}
	if filter.Search != nil && *filter.Search != "" {
		qb.Add("td.search_vector @@ plainto_tsquery('english', %s)", *filter.Search)
	}
	if filter.Category != nil && *filter.Category != "" {
		qb.Add("td.category = %s", *filter.Category)
	}
	if filter.Mode != nil && *filter.Mode != "" {
		qb.Add("td.mode = %s", *filter.Mode)
	}

	where := qb.WhereClause()
	args := qb.Args()

	// Count
	var total int
	countQuery := `SELECT count(*) FROM talkgroup_directory td` + where
	if err := db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	query := `
		SELECT td.system_id, COALESCE(s.name, ''), td.tgid,
			COALESCE(td.alpha_tag, ''), COALESCE(td.mode, ''),
			COALESCE(td.description, ''), COALESCE(td.tag, ''),
			COALESCE(td.category, ''), td.priority
		FROM talkgroup_directory td
		LEFT JOIN systems s ON s.system_id = td.system_id
	` + where + `
		ORDER BY td.system_id, td.tgid
		LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(filter.Offset)

	rows, err := db.Pool.Query(ctx, query, args...)
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
