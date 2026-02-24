package database

import "context"

// UpsertInstance ensures the instance exists and returns its numeric id.
func (db *DB) UpsertInstance(ctx context.Context, instanceID string) (int, error) {
	return db.Q.UpsertInstance(ctx, instanceID)
}
