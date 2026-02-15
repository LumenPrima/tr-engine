package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	SystemID   *int
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
	qb := newQueryBuilder()

	if filter.SystemID != nil {
		qb.Add("tm.system_id = %s", *filter.SystemID)
	}
	if filter.Opcode != nil {
		qb.Add("tm.opcode = %s", *filter.Opcode)
	}
	if filter.OpcodeType != nil {
		qb.Add("tm.opcode_type = %s", *filter.OpcodeType)
	}
	if filter.StartTime != nil {
		qb.Add(`tm."time" >= %s`, *filter.StartTime)
	} else {
		qb.Add(`tm."time" >= %s`, time.Now().Add(-1*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add(`tm."time" < %s`, *filter.EndTime)
	}

	fromClause := "FROM trunking_messages tm"
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf(`
		SELECT tm.id, tm.system_id, COALESCE(tm.sys_name, ''), tm.trunk_msg,
			COALESCE(tm.trunk_msg_type, ''), COALESCE(tm.opcode, ''),
			COALESCE(tm.opcode_type, ''), COALESCE(tm.opcode_desc, ''),
			tm.meta, tm."time", COALESCE(tm.instance_id, '')
		%s %s
		ORDER BY tm."time" DESC
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
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
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{
			r.SystemID, r.SysNum, r.SysName, r.TrunkMsg, r.TrunkMsgType,
			r.Opcode, r.OpcodeType, r.OpcodeDesc, r.Meta, r.Time,
			r.InstanceID,
		}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"trunking_messages"},
		[]string{
			"system_id", "sys_num", "sys_name", "trunk_msg", "trunk_msg_type",
			"opcode", "opcode_type", "opcode_desc", "meta", "time",
			"instance_id",
		},
		pgx.CopyFromRows(copyRows),
	)
}
