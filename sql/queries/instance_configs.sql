-- name: InsertInstanceConfig :exec
INSERT INTO instance_configs (instance_id, capture_dir, upload_server, call_timeout, log_file, instance_key, config_json, "time")
VALUES ($1, $2, $3, $4, $5, $6, $7, now());
