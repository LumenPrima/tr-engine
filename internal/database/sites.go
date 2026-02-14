package database

import "context"

type Site struct {
	SiteID     int
	SystemID   int
	InstanceID string
	ShortName  string
}

// FindOrCreateSite ensures a site exists for the given system, instance, and short_name.
func (db *DB) FindOrCreateSite(ctx context.Context, systemID int, instanceID, shortName string) (int, error) {
	var siteID int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO sites (system_id, instance_id, short_name, first_seen, last_seen)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (instance_id, short_name) DO UPDATE
			SET last_seen = now()
		RETURNING site_id
	`, systemID, instanceID, shortName).Scan(&siteID)
	return siteID, err
}

// UpdateSite updates a site's P25 identity fields from system info messages.
// Only updates fields that have non-zero/non-empty values (progressive refinement).
func (db *DB) UpdateSite(ctx context.Context, siteID int, sysNum int, nac string, rfss int, p25SiteID int, systemTypeRaw string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE sites SET
			sys_num         = CASE WHEN $2 > 0 THEN $2 ELSE sys_num END,
			nac             = CASE WHEN $3 <> '' AND $3 <> '0' THEN $3 ELSE nac END,
			rfss            = CASE WHEN $4 > 0 THEN $4 ELSE rfss END,
			p25_site_id     = CASE WHEN $5 > 0 THEN $5 ELSE p25_site_id END,
			system_type_raw = COALESCE(NULLIF($6, ''), system_type_raw),
			updated_at      = now()
		WHERE site_id = $1
	`, siteID, sysNum, nac, rfss, p25SiteID, systemTypeRaw)
	return err
}

// LoadAllSites returns all sites.
func (db *DB) LoadAllSites(ctx context.Context) ([]Site, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT site_id, system_id, instance_id, short_name FROM sites
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.SiteID, &s.SystemID, &s.InstanceID, &s.ShortName); err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}
