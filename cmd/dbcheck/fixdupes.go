package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fixDuplicateCalls(ctx context.Context, pool *pgxpool.Pool, dryRun bool) {
	// Find pairs: a no-audio row paired with a has-audio row on same
	// system+tgid within 5s. Covers:
	//   1. call_end created a row, then audio handler created another (most common)
	//   2. RECORDING + COMPLETED dupes from call ID shift
	// Keep the row with audio (it has src_list/freq_list and the audio file).
	const findPairs = `
		WITH pairs AS (
			SELECT DISTINCT ON (r.call_id)
				c.call_id AS keep_id,
				c.start_time AS keep_start,
				r.call_id AS delete_id,
				r.start_time AS delete_start
			FROM calls r
			JOIN calls c ON r.tgid = c.tgid
				AND r.system_id = c.system_id
				AND ABS(EXTRACT(EPOCH FROM (r.start_time - c.start_time))) <= 5
				AND r.call_id != c.call_id
			WHERE r.audio_file_path IS NULL
				AND c.audio_file_path IS NOT NULL
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

	// Merge: keep the completed row, copy any missing fields from the deleted row.
	// Notably, preserve the tr_call_id from whichever row has it.
	const mergeSQL = `
		UPDATE calls keep
		SET
			tr_call_id = COALESCE(NULLIF(keep.tr_call_id, ''), del.tr_call_id),
			stop_time = COALESCE(keep.stop_time, del.stop_time),
			duration = COALESCE(keep.duration, del.duration),
			freq = COALESCE(keep.freq, del.freq),
			freq_error = COALESCE(keep.freq_error, del.freq_error),
			signal_db = COALESCE(keep.signal_db, del.signal_db),
			noise_db = COALESCE(keep.noise_db, del.noise_db),
			error_count = COALESCE(keep.error_count, del.error_count),
			spike_count = COALESCE(keep.spike_count, del.spike_count),
			call_state = COALESCE(keep.call_state, del.call_state),
			call_state_type = COALESCE(NULLIF(keep.call_state_type, ''), del.call_state_type),
			rec_state = COALESCE(keep.rec_state, del.rec_state),
			rec_state_type = COALESCE(NULLIF(keep.rec_state_type, ''), del.rec_state_type),
			call_filename = COALESCE(keep.call_filename, del.call_filename),
			audio_file_path = COALESCE(keep.audio_file_path, del.audio_file_path),
			audio_file_size = COALESCE(keep.audio_file_size, del.audio_file_size),
			process_call_time = COALESCE(keep.process_call_time, del.process_call_time),
			retry_attempt = COALESCE(keep.retry_attempt, del.retry_attempt),
			call_group_id = COALESCE(keep.call_group_id, del.call_group_id),
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

		// COALESCE src_list/freq_list/unit_ids from deleted row onto kept row
		_, err = tx.Exec(ctx, `
			UPDATE calls keep SET
				src_list  = COALESCE(keep.src_list, del.src_list),
				freq_list = COALESCE(keep.freq_list, del.freq_list),
				unit_ids  = COALESCE(keep.unit_ids, del.unit_ids)
			FROM calls del
			WHERE keep.call_id = $1 AND keep.start_time = $2
			  AND del.call_id = $3 AND del.start_time = $4
		`, p.keepID, p.keepStart, p.deleteID, p.deleteStart)
		if err != nil {
			tx.Rollback(ctx)
			fmt.Printf("  Error merging src/freq data for call_id=%d: %v\n", p.deleteID, err)
			errors++
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
