-- name: InsertPluginStatus :exec
INSERT INTO plugin_statuses (client_id, instance_id, status, "time")
VALUES ($1, $2, $3, $4);
