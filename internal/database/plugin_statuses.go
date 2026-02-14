package database

import (
	"context"
	"time"
)

func (db *DB) InsertPluginStatus(ctx context.Context, clientID, instanceID, status string, t time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO plugin_statuses (client_id, instance_id, status, "time")
		VALUES ($1, $2, $3, $4)
	`, clientID, instanceID, status, t)
	return err
}
