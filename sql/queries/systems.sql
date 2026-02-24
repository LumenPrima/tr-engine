-- name: FindSystemViaSite :one
SELECT s.system_id, COALESCE(sys.sysid, '') AS sysid
FROM sites s
JOIN systems sys ON sys.system_id = s.system_id
WHERE s.instance_id = $1 AND s.short_name = $2
LIMIT 1;

-- name: CreateSystem :one
INSERT INTO systems (system_type, name, sysid, wacn)
VALUES ('p25', $1, '0', '0')
RETURNING system_id;

-- name: UpdateSystemIdentity :exec
UPDATE systems SET
    system_type = COALESCE(NULLIF(@system_type::text, ''), system_type),
    sysid       = CASE WHEN @sysid::text <> '' AND @sysid <> '0' THEN @sysid ELSE sysid END,
    wacn        = CASE WHEN @wacn::text <> '' AND @wacn <> '0' THEN @wacn ELSE wacn END,
    name        = COALESCE(NULLIF(@name::text, ''), name)
WHERE system_id = @system_id AND deleted_at IS NULL;

-- name: FindSystemBySysidWacn :one
SELECT system_id FROM systems
WHERE sysid = $1 AND wacn = $2
  AND sysid <> '0'
  AND system_id <> $3
  AND deleted_at IS NULL
LIMIT 1;

-- name: GetSystemByID :one
SELECT system_id, system_type, COALESCE(name, '') AS name, sysid, wacn
FROM systems WHERE system_id = $1 AND deleted_at IS NULL;

-- name: ListActiveSystems :many
SELECT system_id, system_type, COALESCE(name, '') AS name, sysid, wacn
FROM systems
WHERE deleted_at IS NULL
ORDER BY system_id;

-- name: UpdateSystemFields :exec
UPDATE systems SET
    name  = CASE WHEN @name::text <> '' THEN @name ELSE name END,
    sysid = CASE WHEN @sysid::text <> '' THEN @sysid ELSE sysid END,
    wacn  = CASE WHEN @wacn::text <> '' THEN @wacn ELSE wacn END
WHERE system_id = @system_id AND deleted_at IS NULL;

-- name: LoadAllSystems :many
SELECT system_id, system_type, COALESCE(name, '') AS name, sysid, wacn
FROM systems
WHERE deleted_at IS NULL;
