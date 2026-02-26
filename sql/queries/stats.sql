-- name: GetOverallStats :one
SELECT
    (SELECT count(*)::int FROM systems WHERE deleted_at IS NULL) AS systems,
    (SELECT count(*)::int FROM talkgroups) AS talkgroups,
    (SELECT count(*)::int FROM units) AS units,
    (SELECT count(*)::int FROM calls) AS total_calls,
    (SELECT count(*)::int FROM calls WHERE start_time > now() - interval '30 days') AS calls_30d,
    (SELECT count(*)::int FROM calls WHERE start_time > now() - interval '24 hours') AS calls_24h,
    (SELECT count(*)::int FROM calls WHERE start_time > now() - interval '1 hour') AS calls_1h,
    COALESCE((SELECT sum(duration) / 3600.0 FROM calls WHERE duration IS NOT NULL), 0)::float8 AS total_duration_hours;

-- name: GetSystemActivity :many
SELECT
    s.system_id, COALESCE(s.name, '') AS system_name, s.sysid,
    (SELECT count(*)::int FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '1 hour') AS calls_1h,
    (SELECT count(*)::int FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '24 hours') AS calls_24h,
    (SELECT count(DISTINCT tgid)::int FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '1 hour') AS active_talkgroups,
    (SELECT COALESCE(sum(t.unit_count_30d), 0)::int FROM talkgroups t
        WHERE t.system_id = s.system_id) AS active_units
FROM systems s
WHERE s.deleted_at IS NULL
ORDER BY s.system_id;

-- name: ListP25Systems :many
SELECT s.system_id, COALESCE(s.name, '') AS name, s.sysid, s.wacn,
    (SELECT count(*)::int FROM talkgroups t WHERE t.system_id = s.system_id) AS talkgroup_count,
    (SELECT count(*)::int FROM units u WHERE u.system_id = s.system_id) AS unit_count,
    (SELECT count(*)::int FROM calls c WHERE c.system_id = s.system_id AND c.start_time > now() - interval '24 hours') AS calls_24h
FROM systems s
WHERE s.system_type IN ('p25', 'smartnet') AND s.deleted_at IS NULL
ORDER BY s.system_id;
