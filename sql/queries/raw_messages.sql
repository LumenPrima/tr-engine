-- name: InsertRawMessages :copyfrom
INSERT INTO mqtt_raw_messages (topic, payload, received_at, instance_id)
VALUES ($1, $2, $3, $4);
