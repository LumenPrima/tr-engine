package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateMonthlyPartition calls the SQL helper to create a monthly partition.
// The function is idempotent — returns "(already exists)" if the partition exists.
func (db *DB) CreateMonthlyPartition(ctx context.Context, table string, partitionStart time.Time) (string, error) {
	var result string
	err := db.Pool.QueryRow(ctx,
		`SELECT create_monthly_partition($1, $2::date)`,
		table, partitionStart.Format("2006-01-02"),
	).Scan(&result)
	return result, err
}

// CreateWeeklyPartition calls the SQL helper to create a weekly partition.
// The function auto-aligns to Monday and is idempotent.
func (db *DB) CreateWeeklyPartition(ctx context.Context, table string, partitionStart time.Time) (string, error) {
	var result string
	err := db.Pool.QueryRow(ctx,
		`SELECT create_weekly_partition($1, $2::date)`,
		table, partitionStart.Format("2006-01-02"),
	).Scan(&result)
	return result, err
}

// DecimateResult holds the counts returned by decimate_state_table().
type DecimateResult struct {
	Deleted1w int64 // rows deleted in 1-week–1-month window (kept 1/min)
	Deleted1m int64 // rows deleted in >1-month window (kept 1/hour)
}

// DecimateStateTable calls the SQL helper to thin out old state rows.
// Phase 1 (1 week – 1 month): keep 1 per minute.
// Phase 2 (> 1 month): keep 1 per hour.
func (db *DB) DecimateStateTable(ctx context.Context, table, timeColumn string) (DecimateResult, error) {
	var r DecimateResult
	err := db.Pool.QueryRow(ctx,
		`SELECT * FROM decimate_state_table($1, $2)`,
		table, timeColumn,
	).Scan(&r.Deleted1w, &r.Deleted1m)
	return r, err
}

// PurgeOlderThan deletes rows older than the given retention period.
// Table and column names are hardcoded by callers (not user input).
func (db *DB) PurgeOlderThan(ctx context.Context, table, timeColumn string, retention time.Duration) (int64, error) {
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE %s < now() - $1::interval`,
		pgx.Identifier{table}.Sanitize(), pgx.Identifier{timeColumn}.Sanitize(),
	)
	tag, err := db.Pool.Exec(ctx, query, retention)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// expiredPartition is a child partition whose data is older than a cutoff.
type expiredPartition struct {
	name  string
	upper time.Time // exclusive upper bound of the partition's range
}

// expiredPartitions returns the child partitions of parentTable whose upper
// bound is strictly before cutoff, along with each bound.
//
// The bound is cast to timestamptz inside SQL so the Go side always receives a
// real time.Time, regardless of the partition column's type. (These tables are
// partitioned on timestamptz columns, so pg_get_expr renders bounds like
// '2026-05-18 00:00:00+00'; an earlier date-only time.Parse layout failed on
// that and silently skipped every partition, so nothing was ever dropped.)
// Partitions whose bound can't be extracted — DEFAULT partitions, or an
// open-ended TO (MAXVALUE) — yield a NULL bound and are never returned, so they
// are never auto-dropped.
func (db *DB) expiredPartitions(ctx context.Context, parentTable string, cutoff time.Time) ([]expiredPartition, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT c.relname,
		       (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \(''([^'']+)''\)'))[1]::timestamptz AS upper_bound
		FROM pg_inherits i
		JOIN pg_class p ON i.inhparent = p.oid
		JOIN pg_class c ON i.inhrelid = c.oid
		WHERE p.relname = $1
		  AND c.relpartbound IS NOT NULL
		ORDER BY c.relname
	`, parentTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expired []expiredPartition
	for rows.Next() {
		var name string
		var upper *time.Time // nullable: NULL for DEFAULT / MAXVALUE bounds
		if err := rows.Scan(&name, &upper); err != nil {
			return nil, err
		}
		if upper != nil && upper.Before(cutoff) {
			expired = append(expired, expiredPartition{name: name, upper: *upper})
		}
	}
	return expired, rows.Err()
}

// DropOldWeeklyPartitions finds and drops weekly partitions whose upper bound
// is older than the given duration. Returns the names of dropped partitions.
func (db *DB) DropOldWeeklyPartitions(ctx context.Context, parentTable string, olderThan time.Duration) ([]string, error) {
	if olderThan <= 0 {
		return nil, nil // retention disabled
	}
	expired, err := db.expiredPartitions(ctx, parentTable, time.Now().Add(-olderThan))
	if err != nil {
		return nil, err
	}
	var dropped []string
	for _, p := range expired {
		if _, err := db.Pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, pgx.Identifier{p.name}.Sanitize())); err != nil {
			return dropped, fmt.Errorf("drop %s: %w", p.name, err)
		}
		dropped = append(dropped, p.name)
	}
	return dropped, nil
}

// CallRetentionResult reports what DropOldCallPartitions removed.
type CallRetentionResult struct {
	CallPartitionsDropped  []string // dropped partitions of the calls parent table
	ChildPartitionsDropped []string // dropped call_frequencies / call_transmissions partitions
	TranscriptionsDeleted  int64    // transcriptions rows deleted (table is not partitioned)
}

// DropOldCallPartitions enforces monthly retention on the foreign-key-coupled
// "call family" (calls + call_frequencies + call_transmissions + transcriptions),
// dropping whole monthly partitions for calendar months fully older than
// olderThan. A no-op when olderThan <= 0 (retention disabled).
//
// The work is done in foreign-key-safe order, validated against PostgreSQL's
// behaviour (a plain DROP of a referenced calls partition is refused because the
// child FK constraints depend on it; DETACH first, then DROP, succeeds once the
// referencing rows are gone):
//
//  1. Drop the referencing child partitions (call_frequencies, call_transmissions).
//     Nothing references these tables, so a plain partition DROP is safe.
//  2. Delete transcriptions rows below the partition boundary. transcriptions is
//     not partitioned, so it needs a row DELETE. The boundary is the newest
//     dropped calls-partition upper bound — NOT the raw cutoff — so transcripts
//     belonging to a not-yet-expired boundary month's calls are never deleted.
//  3. DETACH then DROP the calls partitions themselves.
func (db *DB) DropOldCallPartitions(ctx context.Context, olderThan time.Duration) (CallRetentionResult, error) {
	var res CallRetentionResult
	if olderThan <= 0 {
		return res, nil // retention disabled
	}
	cutoff := time.Now().Add(-olderThan)

	// Determine which calls partitions are fully expired. This set defines the
	// boundary up to which the entire call family is removed.
	callExpired, err := db.expiredPartitions(ctx, "calls", cutoff)
	if err != nil {
		return res, fmt.Errorf("scan calls partitions: %w", err)
	}
	if len(callExpired) == 0 {
		return res, nil // no whole month is expired yet
	}
	// boundary = newest dropped calls-partition upper bound. Everything in the
	// call family strictly before this timestamp is being removed.
	boundary := callExpired[0].upper
	for _, p := range callExpired[1:] {
		if p.upper.After(boundary) {
			boundary = p.upper
		}
	}

	// 1. Drop referencing child partitions (no table references these).
	for _, child := range []string{"call_frequencies", "call_transmissions"} {
		expired, err := db.expiredPartitions(ctx, child, cutoff)
		if err != nil {
			return res, fmt.Errorf("scan %s partitions: %w", child, err)
		}
		for _, p := range expired {
			if _, err := db.Pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, pgx.Identifier{p.name}.Sanitize())); err != nil {
				return res, fmt.Errorf("drop %s: %w", p.name, err)
			}
			res.ChildPartitionsDropped = append(res.ChildPartitionsDropped, p.name)
		}
	}

	// 2. Delete transcriptions rows for the expired calls (table not partitioned).
	tag, err := db.Pool.Exec(ctx, `DELETE FROM transcriptions WHERE call_start_time < $1`, boundary)
	if err != nil {
		return res, fmt.Errorf("delete transcriptions: %w", err)
	}
	res.TranscriptionsDeleted = tag.RowsAffected()

	// 3. Detach + drop the referenced calls partitions. A plain DROP is blocked
	// while the child FK constraints depend on the partition, so detach first.
	for _, p := range callExpired {
		ident := pgx.Identifier{p.name}.Sanitize()
		if _, err := db.Pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE calls DETACH PARTITION %s`, ident)); err != nil {
			return res, fmt.Errorf("detach %s: %w", p.name, err)
		}
		if _, err := db.Pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, ident)); err != nil {
			return res, fmt.Errorf("drop %s: %w", p.name, err)
		}
		res.CallPartitionsDropped = append(res.CallPartitionsDropped, p.name)
	}

	return res, nil
}
