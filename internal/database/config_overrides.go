package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (db *DB) GetConfigOverride(ctx context.Context, key string) (string, bool, error) {
	var value string
	var updatedAt time.Time
	err := db.Pool.QueryRow(ctx,
		`SELECT value, updated_at FROM config_overrides WHERE key = $1`, key,
	).Scan(&value, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func (db *DB) GetAllConfigOverrides(ctx context.Context) (map[string]string, error) {
	rows, err := db.Pool.Query(ctx, `SELECT key, value FROM config_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}

func (db *DB) SetConfigOverride(ctx context.Context, key, value string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO config_overrides (key, value, updated_at) VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`,
		key, value,
	)
	return err
}

func (db *DB) DeleteConfigOverride(ctx context.Context, key string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM config_overrides WHERE key = $1`, key,
	)
	return err
}
