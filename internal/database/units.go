package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
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
	LastEventTgTag string     `json:"last_event_tg_tag,omitempty"`
	RelevanceScore *int       `json:"relevance_score,omitempty"`
}

func unitRowToAPI(r sqlcdb.GetUnitByCompositeRow) UnitAPI {
	u := UnitAPI{
		SystemID:       r.SystemID,
		SystemName:     r.SystemName,
		Sysid:          r.Sysid,
		UnitID:         r.UnitID,
		AlphaTag:       r.AlphaTag,
		AlphaTagSource: r.AlphaTagSource,
		LastEventType:  r.LastEventType,
		LastEventTgTag: r.LastEventTgTag,
	}
	if r.FirstSeen.Valid {
		u.FirstSeen = &r.FirstSeen.Time
	}
	if r.LastSeen.Valid {
		u.LastSeen = &r.LastSeen.Time
	}
	if r.LastEventTime.Valid {
		u.LastEventTime = &r.LastEventTime.Time
	}
	if r.LastEventTgid != nil {
		v := int(*r.LastEventTgid)
		u.LastEventTgid = &v
	}
	return u
}

// ListUnits returns units matching the filter.
func (db *DB) ListUnits(ctx context.Context, filter UnitFilter) ([]UnitAPI, int, error) {
	var activeWithin any
	if filter.ActiveWithin != nil {
		activeWithin = strconv.Itoa(*filter.ActiveWithin) + " minutes"
	}

	const fromClause = `FROM units u
		JOIN systems s ON s.system_id = u.system_id AND s.deleted_at IS NULL
		LEFT JOIN talkgroups tg ON tg.system_id = u.system_id AND tg.tgid = u.last_event_tgid`
	const whereClause = `
		WHERE ($1::text IS NULL OR s.sysid = $1)
		  AND ($2::text IS NULL OR u.alpha_tag ILIKE '%' || $2 || '%' OR u.unit_id::text = $2)
		  AND ($3::text IS NULL OR u.last_seen > now() - $3::interval)
		  AND ($4::int[] IS NULL OR u.last_event_tgid = ANY($4))`
	args := []any{filter.Sysid, filter.Search, activeWithin, pqIntArray(filter.Talkgroups)}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
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
			u.last_event_type, u.last_event_time, u.last_event_tgid,
			COALESCE(tg.alpha_tag, '')
		%s %s
		ORDER BY %s
		LIMIT $5 OFFSET $6
	`, fromClause, whereClause, orderBy)

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
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
			&u.LastEventTgTag,
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
	row, err := db.Q.GetUnitByComposite(ctx, sqlcdb.GetUnitByCompositeParams{
		SystemID: systemID,
		UnitID:   unitID,
	})
	if err != nil {
		return nil, err
	}
	u := unitRowToAPI(row)
	return &u, nil
}

// FindUnitSystems returns systems where a unit ID exists (for ambiguity resolution).
func (db *DB) FindUnitSystems(ctx context.Context, unitID int) ([]AmbiguousMatch, error) {
	rows, err := db.Q.FindUnitSystems(ctx, unitID)
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
	return db.Q.UpdateUnitFields(ctx, sqlcdb.UpdateUnitFieldsParams{
		AlphaTag:       atVal,
		AlphaTagSource: srcVal,
		SystemID:       systemID,
		UnitID:         unitID,
	})
}

// UpsertUnit inserts or updates a unit, never overwriting good data with empty strings.
func (db *DB) UpsertUnit(ctx context.Context, systemID, unitID int, alphaTag, eventType string, eventTime time.Time, tgid int) error {
	tgid32 := int32(tgid)
	return db.Q.UpsertUnit(ctx, sqlcdb.UpsertUnitParams{
		SystemID:  systemID,
		UnitID:    unitID,
		AlphaTag:  &alphaTag,
		EventType: &eventType,
		EventTime: pgtype.Timestamptz{Time: eventTime, Valid: true},
		Tgid:      &tgid32,
	})
}
