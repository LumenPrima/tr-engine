package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fixUnresolvedCalls(ctx context.Context, pool *pgxpool.Pool, dryRun bool) {
	// Phase 1: Close encrypted calls using calls_active checkpoint elapsed data.
	fixEncryptedCalls(ctx, pool, dryRun)

	// Phase 2: Merge remaining unencrypted duplicates (RECORDING + COMPLETED pairs).
	// Re-uses the same logic as fix-dupes.
	fmt.Println()
	fixDuplicateCalls(ctx, pool, dryRun)

	// Phase 3: Mark orphaned call_starts (no matching end, no checkpoint data).
	fmt.Println()
	fixOrphanedStarts(ctx, pool, dryRun)
}

func fixEncryptedCalls(ctx context.Context, pool *pgxpool.Pool, dryRun bool) {
	fmt.Println("── Phase 1: Close encrypted calls from checkpoint elapsed data ──")

	// Extract max elapsed per tr_call_id from checkpoint JSONB.
	const findSQL = `
		WITH elapsed AS (
			SELECT c.value->>'id' AS tr_call_id,
			       max((c.value->>'elapsed')::int) AS max_elapsed
			FROM call_active_checkpoints cap,
			     jsonb_array_elements(cap.active_calls->'calls') c
			WHERE cap.call_count > 0
			  AND (c.value->>'encrypted')::boolean = true
			GROUP BY c.value->>'id'
		)
		SELECT cl.call_id, cl.start_time, cl.tr_call_id, e.max_elapsed
		FROM calls cl
		JOIN elapsed e ON cl.tr_call_id = e.tr_call_id
		WHERE cl.encrypted = true
		  AND (cl.duration IS NULL OR cl.duration = 0)
	`

	rows, err := pool.Query(ctx, findSQL)
	if err != nil {
		fmt.Printf("Error finding encrypted calls: %v\n", err)
		return
	}
	defer rows.Close()

	type encCall struct {
		callID     int64
		startTime  time.Time
		trCallID   string
		maxElapsed int
	}
	var calls []encCall
	for rows.Next() {
		var c encCall
		if err := rows.Scan(&c.callID, &c.startTime, &c.trCallID, &c.maxElapsed); err != nil {
			fmt.Printf("Error scanning: %v\n", err)
			return
		}
		calls = append(calls, c)
	}
	rows.Close()

	fmt.Printf("Found %d encrypted calls with checkpoint elapsed data\n", len(calls))
	if len(calls) == 0 {
		return
	}

	if dryRun {
		fmt.Println("Dry run — no changes made.")
		for i, c := range calls {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(calls)-10)
				break
			}
			fmt.Printf("  call_id=%d  tr_call_id=%s  elapsed=%ds\n", c.callID, c.trCallID, c.maxElapsed)
		}
		return
	}

	const updateSQL = `
		UPDATE calls SET
			stop_time = $3,
			duration = $4,
			call_state_type = COALESCE(NULLIF(call_state_type, ''), 'COMPLETED'),
			updated_at = now()
		WHERE call_id = $1 AND start_time = $2
		  AND (duration IS NULL OR duration = 0)
	`

	updated := 0
	errors := 0
	for _, c := range calls {
		stopTime := c.startTime.Add(time.Duration(c.maxElapsed) * time.Second)
		duration := float32(c.maxElapsed)
		_, err := pool.Exec(ctx, updateSQL, c.callID, c.startTime, stopTime, duration)
		if err != nil {
			fmt.Printf("  Error updating call_id=%d: %v\n", c.callID, err)
			errors++
			continue
		}
		updated++
	}
	fmt.Printf("Closed %d encrypted calls, %d errors\n", updated, errors)
}

func fixOrphanedStarts(ctx context.Context, pool *pgxpool.Pool, dryRun bool) {
	fmt.Println("── Phase 3: Close orphaned call_starts (no matching end) ──")

	// Find unresolved unencrypted calls that have NO matching COMPLETED duplicate.
	// These are call_starts that never got a call_end. Estimate duration from the
	// gap to the next call on the same tgid+system, capped at 120s.
	const findSQL = `
		SELECT c.call_id, c.start_time, c.tgid, c.system_id,
		       (SELECT min(c2.start_time)
		        FROM calls c2
		        WHERE c2.tgid = c.tgid
		          AND c2.system_id = c.system_id
		          AND c2.start_time > c.start_time
		          AND c2.call_id != c.call_id) AS next_start
		FROM calls c
		WHERE (c.duration IS NULL OR c.duration = 0)
		  AND c.encrypted = false
		  AND NOT EXISTS (
		      SELECT 1 FROM calls c3
		      WHERE c3.tgid = c.tgid AND c3.system_id = c.system_id
		        AND ABS(EXTRACT(EPOCH FROM (c3.start_time - c.start_time))) <= 2
		        AND c3.call_id != c.call_id
		        AND c3.duration > 0
		  )
	`

	rows, err := pool.Query(ctx, findSQL)
	if err != nil {
		fmt.Printf("Error finding orphaned starts: %v\n", err)
		return
	}
	defer rows.Close()

	type orphan struct {
		callID    int64
		startTime time.Time
		tgid      int
		systemID  int
		nextStart *time.Time
	}
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.callID, &o.startTime, &o.tgid, &o.systemID, &o.nextStart); err != nil {
			fmt.Printf("Error scanning: %v\n", err)
			return
		}
		orphans = append(orphans, o)
	}
	rows.Close()

	fmt.Printf("Found %d orphaned call_starts\n", len(orphans))
	if len(orphans) == 0 {
		return
	}

	if dryRun {
		fmt.Println("Dry run — no changes made.")
		for i, o := range orphans {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(orphans)-10)
				break
			}
			est := "no next call"
			if o.nextStart != nil {
				gap := o.nextStart.Sub(o.startTime).Seconds()
				est = fmt.Sprintf("next call in %.0fs", gap)
			}
			fmt.Printf("  call_id=%d  tgid=%d  %s\n", o.callID, o.tgid, est)
		}
		return
	}

	const updateSQL = `
		UPDATE calls SET
			stop_time = $3,
			duration = $4,
			call_state_type = COALESCE(NULLIF(call_state_type, ''), 'COMPLETED'),
			updated_at = now()
		WHERE call_id = $1 AND start_time = $2
		  AND (duration IS NULL OR duration = 0)
	`

	updated := 0
	errors := 0
	for _, o := range orphans {
		// Estimate duration: gap to next call on same tgid, capped at 120s.
		// If no next call exists, assume a short call (5s).
		var duration float32
		if o.nextStart != nil {
			gap := float32(o.nextStart.Sub(o.startTime).Seconds())
			if gap > 120 {
				gap = 120
			}
			if gap < 1 {
				gap = 1
			}
			duration = gap
		} else {
			duration = 5
		}
		stopTime := o.startTime.Add(time.Duration(duration) * time.Second)

		_, err := pool.Exec(ctx, updateSQL, o.callID, o.startTime, stopTime, duration)
		if err != nil {
			fmt.Printf("  Error updating call_id=%d: %v\n", o.callID, err)
			errors++
			continue
		}
		updated++
	}
	fmt.Printf("Closed %d orphaned call_starts, %d errors\n", updated, errors)
}
