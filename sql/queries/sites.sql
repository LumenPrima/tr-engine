-- name: FindOrCreateSite :one
INSERT INTO sites (system_id, instance_id, short_name, first_seen, last_seen)
VALUES ($1, $2, $3, now(), now())
ON CONFLICT (instance_id, short_name) DO UPDATE
    SET last_seen = now()
RETURNING site_id;

-- name: UpdateSite :exec
UPDATE sites SET
    sys_num         = CASE WHEN @sys_num::smallint > 0 THEN @sys_num ELSE sys_num END,
    nac             = CASE WHEN @nac::text <> '' AND @nac <> '0' THEN @nac ELSE nac END,
    rfss            = CASE WHEN @rfss::smallint > 0 THEN @rfss ELSE rfss END,
    p25_site_id     = CASE WHEN @p25_site_id::smallint > 0 THEN @p25_site_id ELSE p25_site_id END,
    system_type_raw = COALESCE(NULLIF(@system_type_raw::text, ''), system_type_raw),
    updated_at      = now()
WHERE site_id = @site_id;

-- name: GetSiteByID :one
SELECT site_id, system_id, short_name, instance_id,
    COALESCE(nac, '') AS nac, rfss, p25_site_id, sys_num
FROM sites WHERE site_id = $1;

-- name: ListSitesForSystem :many
SELECT site_id, system_id, short_name, instance_id,
    COALESCE(nac, '') AS nac, rfss, p25_site_id, sys_num
FROM sites WHERE system_id = $1
ORDER BY site_id;

-- name: LoadAllSitesAPI :many
SELECT site_id, system_id, short_name, instance_id,
    COALESCE(nac, '') AS nac, rfss, p25_site_id, sys_num
FROM sites ORDER BY site_id;

-- name: UpdateSiteFields :exec
UPDATE sites SET
    short_name  = CASE WHEN @short_name::text <> '' THEN @short_name ELSE short_name END,
    instance_id = CASE WHEN @new_instance_id::text <> '' THEN @new_instance_id ELSE instance_id END,
    nac         = CASE WHEN @nac::text <> '' THEN @nac ELSE nac END,
    rfss        = CASE WHEN @rfss::smallint > 0 THEN @rfss ELSE rfss END,
    p25_site_id = CASE WHEN @p25_site_id::smallint > 0 THEN @p25_site_id ELSE p25_site_id END
WHERE site_id = @site_id;

-- name: SiteExists :one
SELECT EXISTS(SELECT 1 FROM sites WHERE site_id = $1);

-- name: LoadAllSites :many
SELECT s.site_id, s.system_id, s.instance_id, s.short_name, COALESCE(sys.sysid, '') AS sysid
FROM sites s
JOIN systems sys ON sys.system_id = s.system_id;
