-- name: GetOverallStats :one
SELECT
    (SELECT count(*)::int FROM systems WHERE deleted_at IS NULL) AS systems,
    (SELECT count(*)::int FROM talkgroups) AS talkgroups,
    (SELECT count(*)::int FROM units) AS units,
    (SELECT COALESCE(sum(n_live_tup), 0)::int FROM pg_stat_user_tables
     WHERE relname LIKE 'calls_y%') AS total_calls,
    (SELECT count(*)::int FROM calls WHERE start_time > now() - interval '30 days') AS calls_30d,
    (SELECT COALESCE(sum(calls_24h), 0)::int FROM talkgroups) AS calls_24h,
    (SELECT COALESCE(sum(calls_1h), 0)::int FROM talkgroups) AS calls_1h,
    COALESCE((SELECT sum(duration) / 3600.0 FROM calls
     WHERE start_time > now() - interval '30 days' AND duration IS NOT NULL), 0)::float8 AS total_duration_hours;

-- name: GetSystemActivity :many
SELECT
    s.system_id, COALESCE(s.name, '') AS system_name, s.sysid,
    COALESCE((SELECT sum(t.calls_1h) FROM talkgroups t WHERE t.system_id = s.system_id), 0)::int AS calls_1h,
    COALESCE((SELECT sum(t.calls_24h) FROM talkgroups t WHERE t.system_id = s.system_id), 0)::int AS calls_24h,
    (SELECT count(*)::int FROM talkgroups t WHERE t.system_id = s.system_id AND t.calls_1h > 0) AS active_talkgroups,
    COALESCE((SELECT sum(t.unit_count_30d) FROM talkgroups t WHERE t.system_id = s.system_id), 0)::int AS active_units
FROM systems s
WHERE s.deleted_at IS NULL
ORDER BY s.system_id;

-- name: ListP25Systems :many
SELECT s.system_id, COALESCE(s.name, '') AS name, s.sysid, s.wacn,
    (SELECT count(*)::int FROM talkgroups t WHERE t.system_id = s.system_id) AS talkgroup_count,
    (SELECT count(*)::int FROM units u WHERE u.system_id = s.system_id) AS unit_count,
    COALESCE((SELECT sum(t.calls_24h) FROM talkgroups t WHERE t.system_id = s.system_id), 0)::int AS calls_24h
FROM systems s
WHERE s.system_type IN ('p25', 'smartnet') AND s.deleted_at IS NULL
ORDER BY s.system_id;
