package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

type RawMessageRow struct {
	Topic      string
	Payload    []byte
	ReceivedAt time.Time
	InstanceID string
}

// InsertRawMessages batch-inserts raw MQTT messages using CopyFrom.
func (db *DB) InsertRawMessages(ctx context.Context, rows []RawMessageRow) (int64, error) {
	params := make([]sqlcdb.InsertRawMessagesParams, len(rows))
	for i, r := range rows {
		params[i] = sqlcdb.InsertRawMessagesParams{
			Topic:      r.Topic,
			Payload:    r.Payload,
			ReceivedAt: pgtype.Timestamptz{Time: r.ReceivedAt, Valid: true},
			InstanceID: &r.InstanceID,
		}
	}
	return db.Q.InsertRawMessages(ctx, params)
}
