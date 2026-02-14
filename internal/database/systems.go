package database

import (
	"context"
	"fmt"
)

type System struct {
	SystemID   int
	SystemType string
	Name       string
	Sysid      string
	Wacn       string
}

// FindOrCreateSystem finds an existing system by (instance_id, sys_name) via the sites table,
// or creates a new one. Returns the system_id.
func (db *DB) FindOrCreateSystem(ctx context.Context, instanceID, sysName string) (int, error) {
	// Try to find via sites table first
	var systemID int
	err := db.Pool.QueryRow(ctx, `
		SELECT s.system_id FROM sites s
		WHERE s.instance_id = $1 AND s.short_name = $2
		LIMIT 1
	`, instanceID, sysName).Scan(&systemID)

	if err == nil {
		return systemID, nil
	}

	// Create new system — we don't have sysid/wacn from MQTT, so use defaults
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO systems (system_type, name, sysid, wacn)
		VALUES ('p25', $1, '0', '0')
		RETURNING system_id
	`, sysName).Scan(&systemID)
	if err != nil {
		return 0, fmt.Errorf("create system %q: %w", sysName, err)
	}

	return systemID, nil
}

// LoadAllSystems returns all active systems.
func (db *DB) LoadAllSystems(ctx context.Context) ([]System, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT system_id, system_type, COALESCE(name, ''), sysid, wacn
		FROM systems
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []System
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.SystemID, &s.SystemType, &s.Name, &s.Sysid, &s.Wacn); err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}
