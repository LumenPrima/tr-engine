package database

import (
	"context"
	"time"
)

func (db *DB) InsertConsoleMessage(ctx context.Context, instanceID string, logTime time.Time, severity, logMsg string, mqttTimestamp time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO console_messages (instance_id, log_time, severity, log_msg, mqtt_timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`, instanceID, logTime, severity, logMsg, mqttTimestamp)
	return err
}
