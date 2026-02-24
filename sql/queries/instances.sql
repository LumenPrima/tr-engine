-- name: UpsertInstance :one
INSERT INTO instances (instance_id, first_seen, last_seen)
VALUES ($1, now(), now())
ON CONFLICT (instance_id) DO UPDATE
    SET last_seen = now()
RETURNING id;
