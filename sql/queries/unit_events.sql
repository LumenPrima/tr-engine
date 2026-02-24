-- name: InsertUnitEvent :exec
INSERT INTO unit_events (
    event_type, system_id, unit_rid, "time", tgid,
    unit_alpha_tag, tg_alpha_tag, call_num, freq,
    start_time, stop_time, encrypted, emergency,
    "position", length, error_count, spike_count, sample_count,
    transmission_filename, talkgroup_patches,
    instance_id, sys_num, sys_name
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $16, $17, $18,
    $19, $20,
    $21, $22, $23
);

-- name: LoadRecentAffiliations :many
WITH latest_joins AS (
    SELECT DISTINCT ON (ue.system_id, ue.unit_rid)
        ue.system_id, ue.unit_rid, ue.tgid,
        COALESCE(u.alpha_tag, ue.unit_alpha_tag, '') AS unit_alpha_tag,
        COALESCE(tg.alpha_tag, ue.tg_alpha_tag, '') AS tg_alpha_tag,
        COALESCE(tg.description, '') AS tg_description,
        COALESCE(tg.tag, '') AS tg_tag,
        COALESCE(tg."group", '') AS tg_group,
        COALESCE(s.name, '') AS system_name, COALESCE(s.sysid, '') AS sysid,
        ue."time"
    FROM unit_events ue
    JOIN systems s ON s.system_id = ue.system_id
    LEFT JOIN units u ON u.system_id = ue.system_id AND u.unit_id = ue.unit_rid
    LEFT JOIN talkgroups tg ON tg.system_id = ue.system_id AND tg.tgid = ue.tgid
    WHERE ue.event_type = 'join'
      AND ue."time" > now() - interval '24 hours'
      AND ue.tgid IS NOT NULL
    ORDER BY ue.system_id, ue.unit_rid, ue."time" DESC
)
SELECT lj.*, EXISTS(
    SELECT 1 FROM unit_events ev
    WHERE ev.system_id = lj.system_id
      AND ev.unit_rid = lj.unit_rid
      AND ev."time" > lj."time"
      AND ev."time" > now() - interval '24 hours'
      AND (
        ev.event_type = 'off'
        OR (ev.event_type IN ('call', 'end', 'location')
            AND ev.tgid IS NOT NULL AND ev.tgid != lj.tgid)
      )
) AS went_off
FROM latest_joins lj;
