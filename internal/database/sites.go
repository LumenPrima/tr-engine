package database

import (
	"context"
	"fmt"
)

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

// SiteAPI represents a site for API responses.
type SiteAPI struct {
	SiteID     int    `json:"site_id"`
	SystemID   int    `json:"system_id"`
	ShortName  string `json:"short_name"`
	InstanceID string `json:"instance_id"`
	Nac        string `json:"nac,omitempty"`
	Rfss       *int   `json:"rfss,omitempty"`
	P25SiteID  *int   `json:"p25_site_id,omitempty"`
	SysNum     *int   `json:"sys_num,omitempty"`
}

// GetSiteByID returns a single site.
func (db *DB) GetSiteByID(ctx context.Context, siteID int) (*SiteAPI, error) {
	var s SiteAPI
	err := db.Pool.QueryRow(ctx, `
		SELECT site_id, system_id, short_name, instance_id,
			COALESCE(nac, ''), rfss, p25_site_id, sys_num
		FROM sites WHERE site_id = $1
	`, siteID).Scan(&s.SiteID, &s.SystemID, &s.ShortName, &s.InstanceID,
		&s.Nac, &s.Rfss, &s.P25SiteID, &s.SysNum)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSitesForSystem returns all sites for a given system.
func (db *DB) ListSitesForSystem(ctx context.Context, systemID int) ([]SiteAPI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT site_id, system_id, short_name, instance_id,
			COALESCE(nac, ''), rfss, p25_site_id, sys_num
		FROM sites WHERE system_id = $1
		ORDER BY site_id
	`, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []SiteAPI
	for rows.Next() {
		var s SiteAPI
		if err := rows.Scan(&s.SiteID, &s.SystemID, &s.ShortName, &s.InstanceID,
			&s.Nac, &s.Rfss, &s.P25SiteID, &s.SysNum); err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

// LoadAllSitesAPI returns all sites as SiteAPI structs.
func (db *DB) LoadAllSitesAPI(ctx context.Context) ([]SiteAPI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT site_id, system_id, short_name, instance_id,
			COALESCE(nac, ''), rfss, p25_site_id, sys_num
		FROM sites ORDER BY site_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []SiteAPI
	for rows.Next() {
		var s SiteAPI
		if err := rows.Scan(&s.SiteID, &s.SystemID, &s.ShortName, &s.InstanceID,
			&s.Nac, &s.Rfss, &s.P25SiteID, &s.SysNum); err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

// UpdateSiteFields updates mutable site fields. Only non-nil fields are updated.
func (db *DB) UpdateSiteFields(ctx context.Context, siteID int, shortName, instanceID, nac *string, rfss, p25SiteID *int) error {
	// Check existence first
	var exists bool
	err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sites WHERE site_id = $1)`, siteID).Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("site not found")
	}

	snVal := ""
	if shortName != nil {
		snVal = *shortName
	}
	instVal := ""
	if instanceID != nil {
		instVal = *instanceID
	}
	nacVal := ""
	if nac != nil {
		nacVal = *nac
	}
	rfssVal := 0
	if rfss != nil {
		rfssVal = *rfss
	}
	p25Val := 0
	if p25SiteID != nil {
		p25Val = *p25SiteID
	}

	_, err = db.Pool.Exec(ctx, `
		UPDATE sites SET
			short_name  = CASE WHEN $2 <> '' THEN $2 ELSE short_name END,
			instance_id = CASE WHEN $3 <> '' THEN $3 ELSE instance_id END,
			nac         = CASE WHEN $4 <> '' THEN $4 ELSE nac END,
			rfss        = CASE WHEN $5 > 0 THEN $5 ELSE rfss END,
			p25_site_id = CASE WHEN $6 > 0 THEN $6 ELSE p25_site_id END
		WHERE site_id = $1
	`, siteID, snVal, instVal, nacVal, rfssVal, p25Val)
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
