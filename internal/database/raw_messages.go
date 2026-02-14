package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type RawMessageRow struct {
	Topic      string
	Payload    []byte
	ReceivedAt time.Time
	InstanceID string
}

// InsertRawMessages batch-inserts raw MQTT messages using CopyFrom.
func (db *DB) InsertRawMessages(ctx context.Context, rows []RawMessageRow) (int64, error) {
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{r.Topic, r.Payload, r.ReceivedAt, r.InstanceID}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"mqtt_raw_messages"},
		[]string{"topic", "payload", "received_at", "instance_id"},
		pgx.CopyFromRows(copyRows),
	)
}
