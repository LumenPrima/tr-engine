package database

import (
	"context"
)

type StorageStats struct {
	TotalDBBytes int64              `json:"total_db_bytes"`
	Tables       []TableSizeInfo    `json:"tables"`
	Errors       []string           `json:"errors"`
}

type TableSizeInfo struct {
	Name       string           `json:"name"`
	RowCount   int64            `json:"row_count"`
	TableBytes int64            `json:"table_bytes"`
	IndexBytes int64            `json:"index_bytes"`
	TotalBytes int64            `json:"total_bytes"`
	Partitions []PartitionInfo  `json:"partitions,omitempty"`
}

type PartitionInfo struct {
	Name       string `json:"name"`
	TotalBytes int64  `json:"total_bytes"`
}

func (db *DB) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	result := &StorageStats{}

	var totalBytes int64
	if err := db.Pool.QueryRow(ctx,
		`SELECT pg_database_size(current_database())`,
	).Scan(&totalBytes); err != nil {
		return nil, err
	}
	result.TotalDBBytes = totalBytes

	rows, err := db.Pool.Query(ctx, `
		SELECT s.relname,
		       s.n_live_tup,
		       pg_table_size(c.oid)          AS table_bytes,
		       pg_indexes_size(c.oid)        AS index_bytes,
		       pg_total_relation_size(c.oid) AS total_bytes
		FROM pg_stat_user_tables s
		JOIN pg_class c ON c.oid = s.relid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND n.nspname = 'public'
		ORDER BY pg_total_relation_size(c.oid) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var info TableSizeInfo
		if err := rows.Scan(&info.Name, &info.RowCount, &info.TableBytes, &info.IndexBytes, &info.TotalBytes); err != nil {
			result.Errors = append(result.Errors, "failed to scan table row: "+err.Error())
			continue
		}

		partitions, err := db.getPartitions(ctx, info.Name)
		if err != nil {
			result.Errors = append(result.Errors, "failed to get partitions for "+info.Name+": "+err.Error())
		} else if len(partitions) > 0 {
			info.Partitions = partitions
		}

		result.Tables = append(result.Tables, info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *DB) getPartitions(ctx context.Context, parentTable string) ([]PartitionInfo, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT c.relname,
		       pg_total_relation_size(c.oid) AS total_bytes
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = (
			SELECT oid FROM pg_class
			WHERE relname = $1
			  AND relnamespace = 'public'::regnamespace
		)
		ORDER BY c.relname
	`, parentTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var partitions []PartitionInfo
	for rows.Next() {
		var p PartitionInfo
		if err := rows.Scan(&p.Name, &p.TotalBytes); err != nil {
			return nil, err
		}
		partitions = append(partitions, p)
	}
	return partitions, rows.Err()
}
