package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	ctx := context.Background()

	if len(os.Args) > 1 && os.Args[1] == "cleanup" {
		tag, _ := pool.Exec(ctx, "DELETE FROM sites WHERE short_name = ''")
		fmt.Printf("Deleted %d bogus sites\n", tag.RowsAffected())
		tag, _ = pool.Exec(ctx, "DELETE FROM systems WHERE name IS NULL OR name = ''")
		fmt.Printf("Deleted %d bogus systems\n", tag.RowsAffected())
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "calls" {
		investigateCalls(ctx, pool)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "fix-dupes" {
		dryRun := !(len(os.Args) > 2 && os.Args[2] == "apply")
		fixDuplicateCalls(ctx, pool, dryRun)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "fix-unresolved" {
		dryRun := !(len(os.Args) > 2 && os.Args[2] == "apply")
		fixUnresolvedCalls(ctx, pool, dryRun)
		return
	}

	// Default: table counts
	tables := []string{
		"instances", "systems", "sites", "talkgroups", "units",
		"calls", "call_groups",
		"unit_events", "recorder_snapshots", "decode_rates",
		"mqtt_raw_messages", "plugin_statuses", "call_active_checkpoints",
	}
	fmt.Println("Table                    Count")
	fmt.Println("─────────────────────────────────")
	for _, t := range tables {
		var count int64
		pool.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&count)
		fmt.Printf("%-25s %d\n", t, count)
	}
}

func investigateCalls(ctx context.Context, pool *pgxpool.Pool) {
	// 1. Calls per call_group
	fmt.Println("── Call Group Size Distribution ──")
	rows, _ := pool.Query(ctx, `
		SELECT calls_per_group, count(*) as num_groups
		FROM (
			SELECT call_group_id, count(*) as calls_per_group
			FROM calls
			WHERE call_group_id IS NOT NULL
			GROUP BY call_group_id
		) sub
		GROUP BY calls_per_group
		ORDER BY calls_per_group
	`)
	defer rows.Close()
	for rows.Next() {
		var size, count int
		rows.Scan(&size, &count)
		fmt.Printf("  %d call(s) per group: %d groups\n", size, count)
	}

	// 2. How many calls have NO call_group_id?
	var noGroup int64
	pool.QueryRow(ctx, "SELECT count(*) FROM calls WHERE call_group_id IS NULL").Scan(&noGroup)
	fmt.Printf("\n  Calls with no call_group_id: %d\n", noGroup)

	// 3. How many orphan call_groups (no calls reference them)?
	var orphan int64
	pool.QueryRow(ctx, `
		SELECT count(*) FROM call_groups cg
		WHERE NOT EXISTS (SELECT 1 FROM calls c WHERE c.call_group_id = cg.id)
	`).Scan(&orphan)
	fmt.Printf("  Orphan call_groups (no calls): %d\n", orphan)

	// 4. Calls per system
	fmt.Println("\n── Calls Per System ──")
	rows2, _ := pool.Query(ctx, `
		SELECT system_id, system_name, count(*) FROM calls GROUP BY system_id, system_name ORDER BY system_id
	`)
	defer rows2.Close()
	for rows2.Next() {
		var sysid, count int
		var name string
		rows2.Scan(&sysid, &name, &count)
		fmt.Printf("  system=%d (%s): %d calls\n", sysid, name, count)
	}

	// 5. How calls were created (call_start vs call_end backfill)
	fmt.Println("\n── Call Creation Method ──")
	var withStop, withoutStop int64
	pool.QueryRow(ctx, "SELECT count(*) FROM calls WHERE stop_time IS NOT NULL AND duration > 0").Scan(&withStop)
	pool.QueryRow(ctx, "SELECT count(*) FROM calls WHERE stop_time IS NULL OR duration IS NULL OR duration = 0").Scan(&withoutStop)
	fmt.Printf("  With stop_time+duration (completed): %d\n", withStop)
	fmt.Printf("  Without (start-only or backfilled):  %d\n", withoutStop)

	// 6. Look at duplicate tgid+start_time across systems (should-be-grouped calls)
	fmt.Println("\n── Same tgid+start_time Across Systems (potential cross-site dupes) ──")
	rows3, _ := pool.Query(ctx, `
		SELECT c1.call_id, c1.system_name, c1.tgid, c1.start_time, c1.call_group_id,
		       c2.call_id, c2.system_name, c2.call_group_id
		FROM calls c1
		JOIN calls c2 ON c1.tgid = c2.tgid AND c1.start_time = c2.start_time AND c1.call_id < c2.call_id
		ORDER BY c1.start_time DESC
		LIMIT 20
	`)
	defer rows3.Close()
	found := false
	for rows3.Next() {
		found = true
		var id1, id2 int64
		var sys1, sys2 string
		var tgid int
		var startTime interface{}
		var cg1, cg2 *int
		rows3.Scan(&id1, &sys1, &tgid, &startTime, &cg1, &id2, &sys2, &cg2)
		g1, g2 := "nil", "nil"
		if cg1 != nil { g1 = fmt.Sprintf("%d", *cg1) }
		if cg2 != nil { g2 = fmt.Sprintf("%d", *cg2) }
		fmt.Printf("  call %d (%s, grp=%s) <-> call %d (%s, grp=%s) tgid=%d\n", id1, sys1, g1, id2, sys2, g2, tgid)
	}
	if !found {
		fmt.Println("  (none found)")
	}

	// 7. Sample call_groups with their call count
	fmt.Println("\n── Sample Call Groups (first 15) ──")
	rows4, _ := pool.Query(ctx, `
		SELECT cg.id, cg.system_id, cg.tgid, cg.start_time, cg.tg_alpha_tag,
		       count(c.call_id) as call_count
		FROM call_groups cg
		LEFT JOIN calls c ON c.call_group_id = cg.id
		GROUP BY cg.id, cg.system_id, cg.tgid, cg.start_time, cg.tg_alpha_tag
		ORDER BY cg.id
		LIMIT 15
	`)
	defer rows4.Close()
	for rows4.Next() {
		var id, sysid, tgid, callCount int
		var startTime interface{}
		var alpha *string
		rows4.Scan(&id, &sysid, &tgid, &startTime, &alpha, &callCount)
		a := ""
		if alpha != nil { a = *alpha }
		fmt.Printf("  group=%d sys=%d tgid=%d %q calls=%d start=%v\n", id, sysid, tgid, a, callCount, startTime)
	}
}
