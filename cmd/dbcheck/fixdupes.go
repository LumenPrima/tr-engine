package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fixDuplicateCalls(ctx context.Context, pool *pgxpool.Pool, dryRun bool) {
	// Find pairs: RECORDING row (has tr_call_id, duration=0) paired with
	// COMPLETED row (empty tr_call_id, duration>0) on same system+tgid within 2s.
	// Keep the RECORDING row (it has the tr_call_id), merge COMPLETED data into it.
	const findPairs = `
		WITH pairs AS (
			SELECT DISTINCT ON (r.call_id)
				r.call_id AS keep_id,
				r.start_time AS keep_start,
				c.call_id AS delete_id,
				c.start_time AS delete_start
			FROM calls r
			JOIN calls c ON r.tgid = c.tgid
				AND r.system_id = c.system_id
				AND ABS(EXTRACT(EPOCH FROM (r.start_time - c.start_time))) <= 2
				AND r.call_id != c.call_id
			WHERE r.tr_call_id != ''
				AND (r.duration IS NULL OR r.duration = 0)
				AND (c.tr_call_id = '' OR c.tr_call_id IS NULL)
				AND c.duration > 0
			ORDER BY r.call_id, ABS(EXTRACT(EPOCH FROM (r.start_time - c.start_time)))
		)
		SELECT keep_id, keep_start, delete_id, delete_start FROM pairs
	`

	rows, err := pool.Query(ctx, findPairs)
	if err != nil {
		fmt.Printf("Error finding pairs: %v\n", err)
		return
	}
	defer rows.Close()

	type pair struct {
		keepID, deleteID       int64
		keepStart, deleteStart interface{}
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.keepID, &p.keepStart, &p.deleteID, &p.deleteStart); err != nil {
			fmt.Printf("Error scanning pair: %v\n", err)
			return
		}
		pairs = append(pairs, p)
	}
	rows.Close()

	fmt.Printf("Found %d duplicate pairs\n", len(pairs))
	if len(pairs) == 0 {
		return
	}

	if dryRun {
		fmt.Println("Dry run — no changes made. Run with 'fix-dupes apply' to fix.")
		for i, p := range pairs {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(pairs)-10)
				break
			}
			fmt.Printf("  keep call_id=%d, delete call_id=%d\n", p.keepID, p.deleteID)
		}
		return
	}

	// Merge COMPLETED data into RECORDING rows and delete duplicates.
	const mergeSQL = `
		UPDATE calls keep
		SET
			stop_time = del.stop_time,
			duration = del.duration,
			freq = COALESCE(del.freq, keep.freq),
			freq_error = COALESCE(del.freq_error, keep.freq_error),
			signal_db = COALESCE(del.signal_db, keep.signal_db),
			noise_db = COALESCE(del.noise_db, keep.noise_db),
			error_count = COALESCE(del.error_count, keep.error_count),
			spike_count = COALESCE(del.spike_count, keep.spike_count),
			call_state = COALESCE(del.call_state, keep.call_state),
			call_state_type = COALESCE(del.call_state_type, keep.call_state_type),
			rec_state = COALESCE(del.rec_state, keep.rec_state),
			rec_state_type = COALESCE(del.rec_state_type, keep.rec_state_type),
			call_filename = COALESCE(del.call_filename, keep.call_filename),
			audio_file_path = COALESCE(del.audio_file_path, keep.audio_file_path),
			audio_file_size = COALESCE(del.audio_file_size, keep.audio_file_size),
			process_call_time = COALESCE(del.process_call_time, keep.process_call_time),
			retry_attempt = COALESCE(del.retry_attempt, keep.retry_attempt),
			call_group_id = COALESCE(del.call_group_id, keep.call_group_id),
			updated_at = now()
		FROM calls del
		WHERE keep.call_id = $1 AND keep.start_time = $2
		  AND del.call_id = $3 AND del.start_time = $4
	`

	const deleteSQL = `DELETE FROM calls WHERE call_id = $1 AND start_time = $2`

	merged := 0
	errors := 0
	for _, p := range pairs {
		tx, err := pool.Begin(ctx)
		if err != nil {
			fmt.Printf("  Error starting tx for keep=%d: %v\n", p.keepID, err)
			errors++
			continue
		}

		_, err = tx.Exec(ctx, mergeSQL, p.keepID, p.keepStart, p.deleteID, p.deleteStart)
		if err != nil {
			tx.Rollback(ctx)
			fmt.Printf("  Error merging keep=%d <- delete=%d: %v\n", p.keepID, p.deleteID, err)
			errors++
			continue
		}

		// Reassign child rows from the duplicate to the kept call
		for _, child := range []string{"call_frequencies", "call_transmissions", "transcriptions"} {
			_, err = tx.Exec(ctx,
				fmt.Sprintf("UPDATE %s SET call_id = $1, call_start_time = $2 WHERE call_id = $3 AND call_start_time = $4", child),
				p.keepID, p.keepStart, p.deleteID, p.deleteStart,
			)
			if err != nil {
				tx.Rollback(ctx)
				fmt.Printf("  Error reassigning %s for call_id=%d: %v\n", child, p.deleteID, err)
				errors++
				break
			}
		}
		if err != nil {
			continue
		}

		_, err = tx.Exec(ctx, deleteSQL, p.deleteID, p.deleteStart)
		if err != nil {
			tx.Rollback(ctx)
			fmt.Printf("  Error deleting call_id=%d: %v\n", p.deleteID, err)
			errors++
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			fmt.Printf("  Error committing keep=%d: %v\n", p.keepID, err)
			errors++
			continue
		}
		merged++
	}

	fmt.Printf("Merged %d pairs, %d errors\n", merged, errors)

	// Clean up orphaned call_groups
	tag, err := pool.Exec(ctx, `
		DELETE FROM call_groups cg
		WHERE NOT EXISTS (SELECT 1 FROM calls c WHERE c.call_group_id = cg.id)
	`)
	if err != nil {
		fmt.Printf("Error cleaning orphan call_groups: %v\n", err)
	} else {
		fmt.Printf("Deleted %d orphaned call_groups\n", tag.RowsAffected())
	}
}
