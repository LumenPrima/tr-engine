package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

type TrunkingMessageRow struct {
	SystemID     *int
	SysNum       int16
	SysName      string
	TrunkMsg     int
	TrunkMsgType string
	Opcode       string
	OpcodeType   string
	OpcodeDesc   string
	Meta         []byte // jsonb; nil for null
	Time         time.Time
	InstanceID   string
}

// TrunkingMessageFilter specifies filters for listing trunking messages.
type TrunkingMessageFilter struct {
	SystemIDs  []int
	Opcode     *string
	OpcodeType *string
	StartTime  *time.Time
	EndTime    *time.Time
	Limit      int
	Offset     int
}

// TrunkingMessageAPI represents a trunking message for API responses.
type TrunkingMessageAPI struct {
	ID           int64     `json:"id"`
	SystemID     *int      `json:"system_id,omitempty"`
	SysName      string    `json:"sys_name"`
	TrunkMsg     int       `json:"trunk_msg"`
	TrunkMsgType string    `json:"trunk_msg_type"`
	Opcode       string    `json:"opcode"`
	OpcodeType   string    `json:"opcode_type"`
	OpcodeDesc   string    `json:"opcode_desc"`
	Meta         []byte    `json:"meta,omitempty"`
	Time         time.Time `json:"time"`
	InstanceID   string    `json:"instance_id"`
}

// ListTrunkingMessages returns trunking messages matching the filter.
func (db *DB) ListTrunkingMessages(ctx context.Context, filter TrunkingMessageFilter) ([]TrunkingMessageAPI, int, error) {
	startTime := filter.StartTime
	if startTime == nil {
		t := time.Now().Add(-1 * time.Hour)
		startTime = &t
	}

	const whereClause = `
		WHERE ($1::int[] IS NULL OR tm.system_id = ANY($1))
		  AND ($2::text IS NULL OR tm.opcode = $2)
		  AND ($3::text IS NULL OR tm.opcode_type = $3)
		  AND tm."time" >= $4
		  AND ($5::timestamptz IS NULL OR tm."time" < $5)`
	args := []any{pqIntArray(filter.SystemIDs), filter.Opcode, filter.OpcodeType, *startTime, filter.EndTime}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM trunking_messages tm"+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := `
		SELECT tm.id, tm.system_id, COALESCE(tm.sys_name, ''), tm.trunk_msg,
			COALESCE(tm.trunk_msg_type, ''), COALESCE(tm.opcode, ''),
			COALESCE(tm.opcode_type, ''), COALESCE(tm.opcode_desc, ''),
			tm.meta, tm."time", COALESCE(tm.instance_id, '')
		FROM trunking_messages tm` + whereClause + `
		ORDER BY tm."time" DESC
		LIMIT $6 OFFSET $7`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []TrunkingMessageAPI
	for rows.Next() {
		var m TrunkingMessageAPI
		if err := rows.Scan(
			&m.ID, &m.SystemID, &m.SysName, &m.TrunkMsg,
			&m.TrunkMsgType, &m.Opcode,
			&m.OpcodeType, &m.OpcodeDesc,
			&m.Meta, &m.Time, &m.InstanceID,
		); err != nil {
			return nil, 0, err
		}
		messages = append(messages, m)
	}
	if messages == nil {
		messages = []TrunkingMessageAPI{}
	}
	return messages, total, rows.Err()
}

// InsertTrunkingMessages batch-inserts trunking messages using CopyFrom.
func (db *DB) InsertTrunkingMessages(ctx context.Context, rows []TrunkingMessageRow) (int64, error) {
	params := make([]sqlcdb.InsertTrunkingMessagesParams, len(rows))
	for i, r := range rows {
		var sysID *int32
		if r.SystemID != nil {
			v := int32(*r.SystemID)
			sysID = &v
		}
		trunkMsg := int32(r.TrunkMsg)
		params[i] = sqlcdb.InsertTrunkingMessagesParams{
			SystemID:     sysID,
			SysNum:       &r.SysNum,
			SysName:      &r.SysName,
			TrunkMsg:     &trunkMsg,
			TrunkMsgType: &r.TrunkMsgType,
			Opcode:       &r.Opcode,
			OpcodeType:   &r.OpcodeType,
			OpcodeDesc:   &r.OpcodeDesc,
			Meta:         r.Meta,
			Time:         pgtype.Timestamptz{Time: r.Time, Valid: true},
			InstanceID:   &r.InstanceID,
		}
	}
	return db.Q.InsertTrunkingMessages(ctx, params)
}
