-- name: GetUnitByComposite :one
SELECT u.system_id, COALESCE(s.name, '') AS system_name, s.sysid,
    u.unit_id, COALESCE(u.alpha_tag, '') AS alpha_tag, COALESCE(u.alpha_tag_source, '') AS alpha_tag_source,
    u.first_seen, u.last_seen,
    u.last_event_type, u.last_event_time, u.last_event_tgid,
    COALESCE(tg.alpha_tag, '') AS last_event_tg_tag
FROM units u
JOIN systems s ON s.system_id = u.system_id
LEFT JOIN talkgroups tg ON tg.system_id = u.system_id AND tg.tgid = u.last_event_tgid
WHERE u.system_id = $1 AND u.unit_id = $2;

-- name: FindUnitSystems :many
SELECT u.system_id, COALESCE(s.name, '') AS system_name, s.sysid
FROM units u
JOIN systems s ON s.system_id = u.system_id AND s.deleted_at IS NULL
WHERE u.unit_id = $1;

-- name: UpdateUnitFields :exec
UPDATE units SET
    alpha_tag        = CASE WHEN @alpha_tag::text <> '' THEN @alpha_tag ELSE alpha_tag END,
    alpha_tag_source = CASE WHEN @alpha_tag_source::text <> '' THEN @alpha_tag_source ELSE alpha_tag_source END
WHERE system_id = @system_id AND unit_id = @unit_id;

-- name: UpsertUnit :exec
INSERT INTO units (system_id, unit_id, alpha_tag, first_seen, last_seen, last_event_type, last_event_time, last_event_tgid)
VALUES (@system_id, @unit_id, @alpha_tag, @event_time, @event_time, @event_type, @event_time, @tgid)
ON CONFLICT (system_id, unit_id) DO UPDATE SET
    alpha_tag       = CASE WHEN COALESCE(units.alpha_tag_source, '') = 'manual' THEN units.alpha_tag
                           ELSE COALESCE(NULLIF(@alpha_tag, ''), units.alpha_tag) END,
    first_seen      = LEAST(units.first_seen, @event_time),
    last_seen       = GREATEST(units.last_seen, @event_time),
    last_event_type = CASE WHEN @event_time >= units.last_event_time THEN @event_type ELSE units.last_event_type END,
    last_event_time = GREATEST(units.last_event_time, @event_time),
    last_event_tgid = CASE WHEN @event_time >= units.last_event_time AND @tgid > 0 THEN @tgid ELSE units.last_event_tgid END;
