-- name: InsertConsoleMessage :exec
INSERT INTO console_messages (instance_id, log_time, severity, log_msg, mqtt_timestamp)
VALUES ($1, $2, $3, $4, $5);
