package database

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// UnitFilter specifies filters for listing units.
type UnitFilter struct {
	Sysid        *string
	Search       *string
	ActiveWithin *int // minutes
	Talkgroups   []int
	Limit        int
	Offset       int
	Sort         string
}

// UnitAPI represents a unit for API responses.
type UnitAPI struct {
	SystemID       int        `json:"system_id"`
	SystemName     string     `json:"system_name,omitempty"`
	Sysid          string     `json:"sysid,omitempty"`
	UnitID         int        `json:"unit_id"`
	AlphaTag       string     `json:"alpha_tag,omitempty"`
	AlphaTagSource string     `json:"alpha_tag_source,omitempty"`
	FirstSeen      *time.Time `json:"first_seen,omitempty"`
	LastSeen       *time.Time `json:"last_seen,omitempty"`
	LastEventType  *string    `json:"last_event_type,omitempty"`
	LastEventTime  *time.Time `json:"last_event_time,omitempty"`
	LastEventTgid  *int       `json:"last_event_tgid,omitempty"`
	RelevanceScore *int       `json:"relevance_score,omitempty"`
}

// ListUnits returns units matching the filter.
func (db *DB) ListUnits(ctx context.Context, filter UnitFilter) ([]UnitAPI, int, error) {
	qb := newQueryBuilder()

	if filter.Sysid != nil {
		qb.Add("s.sysid = %s", *filter.Sysid)
	}
	if filter.Search != nil {
		paramIdx := qb.argIdx
		qb.args = append(qb.args, *filter.Search)
		qb.where = append(qb.where, fmt.Sprintf(
			`(u.alpha_tag ILIKE '%%' || $%d || '%%' OR u.unit_id::text = $%d)`,
			paramIdx, paramIdx))
		qb.argIdx++
	}
	if filter.ActiveWithin != nil {
		qb.Add("u.last_seen > now() - (%s || ' minutes')::interval", strconv.Itoa(*filter.ActiveWithin))
	}
	if len(filter.Talkgroups) > 0 {
		qb.Add("u.last_event_tgid = ANY(%s)", filter.Talkgroups)
	}

	fromClause := "FROM units u JOIN systems s ON s.system_id = u.system_id AND s.deleted_at IS NULL"
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "u.unit_id ASC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	dataQuery := fmt.Sprintf(`
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid,
			u.unit_id, COALESCE(u.alpha_tag, ''), COALESCE(u.alpha_tag_source, ''),
			u.first_seen, u.last_seen,
			u.last_event_type, u.last_event_time, u.last_event_tgid
		%s %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, orderBy, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
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

		if filter.Search != nil {
			search := *filter.Search
			score := 10
			if strconv.Itoa(u.UnitID) == search || u.AlphaTag == search {
				score = 100
			} else if len(search) > 0 && len(u.AlphaTag) >= len(search) && u.AlphaTag[:len(search)] == search {
				score = 50
			}
			u.RelevanceScore = &score
		}

		units = append(units, u)
	}
	if units == nil {
		units = []UnitAPI{}
	}
	return units, total, rows.Err()
}

// GetUnitByComposite returns a single unit by system_id and unit_id.
func (db *DB) GetUnitByComposite(ctx context.Context, systemID, unitID int) (*UnitAPI, error) {
	var u UnitAPI
	err := db.Pool.QueryRow(ctx, `
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid,
			u.unit_id, COALESCE(u.alpha_tag, ''), COALESCE(u.alpha_tag_source, ''),
			u.first_seen, u.last_seen,
			u.last_event_type, u.last_event_time, u.last_event_tgid
		FROM units u
		JOIN systems s ON s.system_id = u.system_id
		WHERE u.system_id = $1 AND u.unit_id = $2
	`, systemID, unitID).Scan(
		&u.SystemID, &u.SystemName, &u.Sysid,
		&u.UnitID, &u.AlphaTag, &u.AlphaTagSource,
		&u.FirstSeen, &u.LastSeen,
		&u.LastEventType, &u.LastEventTime, &u.LastEventTgid,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindUnitSystems returns systems where a unit ID exists (for ambiguity resolution).
func (db *DB) FindUnitSystems(ctx context.Context, unitID int) ([]AmbiguousMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid
		FROM units u
		JOIN systems s ON s.system_id = u.system_id AND s.deleted_at IS NULL
		WHERE u.unit_id = $1
	`, unitID)
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

// UpdateUnitFields updates mutable unit fields.
func (db *DB) UpdateUnitFields(ctx context.Context, systemID, unitID int, alphaTag, alphaTagSource *string) error {
	atVal := ""
	if alphaTag != nil {
		atVal = *alphaTag
	}
	srcVal := ""
	if alphaTagSource != nil {
		srcVal = *alphaTagSource
	}

	_, err := db.Pool.Exec(ctx, `
		UPDATE units SET
			alpha_tag        = CASE WHEN $3 <> '' THEN $3 ELSE alpha_tag END,
			alpha_tag_source = CASE WHEN $4 <> '' THEN $4 ELSE alpha_tag_source END
		WHERE system_id = $1 AND unit_id = $2
	`, systemID, unitID, atVal, srcVal)
	return err
}

// UpsertUnit inserts or updates a unit, never overwriting good data with empty strings.
func (db *DB) UpsertUnit(ctx context.Context, systemID, unitID int, alphaTag, eventType string, eventTime time.Time, tgid int) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO units (system_id, unit_id, alpha_tag, first_seen, last_seen, last_event_type, last_event_time, last_event_tgid)
		VALUES ($1, $2, $3, $5, $5, $4, $5, $6)
		ON CONFLICT (system_id, unit_id) DO UPDATE SET
			alpha_tag       = COALESCE(NULLIF($3, ''), units.alpha_tag),
			last_seen       = $5,
			last_event_type = $4,
			last_event_time = $5,
			last_event_tgid = CASE WHEN $6 > 0 THEN $6 ELSE units.last_event_tgid END
	`, systemID, unitID, alphaTag, eventType, eventTime, tgid)
	return err
}
