package database

import "context"

// columnExists checks if a column exists in a table (for graceful schema migration handling).
func (db *DB) columnExists(ctx context.Context, table, column string) bool {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
		table, column).Scan(&exists)
	return err == nil && exists
}

// tableExists checks if a table exists in the database.
func (db *DB) tableExists(ctx context.Context, table string) bool {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = $1)`,
		table).Scan(&exists)
	return err == nil && exists
}

// IS NULL OR helpers — convert empty Go values to nil so PostgreSQL
// sees NULL and the ($1::type IS NULL OR ...) pattern skips the filter.

func pqIntArray(s []int) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

func pqStringArray(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

func pqString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
