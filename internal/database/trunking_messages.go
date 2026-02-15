package database

import (
	"context"
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
