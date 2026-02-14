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
