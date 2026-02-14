package database

import "context"

// InsertInstanceConfig stores a snapshot of a TR instance's configuration.
func (db *DB) InsertInstanceConfig(ctx context.Context, instanceID, captureDir, uploadServer string, callTimeout float64, logFile, instanceKey string, configJSON []byte) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO instance_configs (instance_id, capture_dir, upload_server, call_timeout, log_file, instance_key, config_json, "time")
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
	`, instanceID, captureDir, uploadServer, callTimeout, logFile, instanceKey, configJSON)
	return err
}
