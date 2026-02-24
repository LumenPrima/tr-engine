package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

func (db *DB) InsertPluginStatus(ctx context.Context, clientID, instanceID, status string, t time.Time) error {
	return db.Q.InsertPluginStatus(ctx, sqlcdb.InsertPluginStatusParams{
		ClientID:   &clientID,
		InstanceID: &instanceID,
		Status:     &status,
		Time:       pgtype.Timestamptz{Time: t, Valid: true},
	})
}
