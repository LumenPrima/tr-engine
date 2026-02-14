package database

import "context"

// UpsertInstance ensures the instance exists and returns its numeric id.
func (db *DB) UpsertInstance(ctx context.Context, instanceID string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO instances (instance_id, first_seen, last_seen)
		VALUES ($1, now(), now())
		ON CONFLICT (instance_id) DO UPDATE
			SET last_seen = now()
		RETURNING id
	`, instanceID).Scan(&id)
	return id, err
}
