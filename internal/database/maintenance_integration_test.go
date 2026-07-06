package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// These tests exercise the partition-retention paths against a real PostgreSQL.
// They are skipped unless TEST_DATABASE_URL points at a throwaway database (the
// schema is applied and partitions are dropped, so never point this at anything
// you care about). To run against an ephemeral container:
//
//	docker run -d --rm -e POSTGRES_PASSWORD=test -e POSTGRES_USER=test \
//	    -e POSTGRES_DB=trengine_test -p 55432:5432 postgres:17-alpine
//	TEST_DATABASE_URL='postgres://test:test@localhost:55432/trengine_test?sslmode=disable' \
//	    go test ./internal/database/ -run 'Retention|TimestamptzBounds' -v

func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	db, err := Connect(ctx, url, zerolog.Nop())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)

	schema, err := os.ReadFile("../../schema.sql")
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if err := db.InitSchema(ctx, schema); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db *DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func count(t *testing.T, db *DB, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var reg *string
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT to_regclass('public.' || $1)::text`, name).Scan(&reg); err != nil {
		t.Fatalf("to_regclass(%s): %v", name, err)
	}
	return reg != nil
}

func createMonthlyPart(t *testing.T, db *DB, table string, monthStart time.Time) {
	t.Helper()
	mustExec(t, db, `SELECT create_monthly_partition($1, $2::date)`, table, monthStart.Format("2006-01-02"))
}

// insertCall inserts a call and its child/transcription rows at startTime,
// returning the new call_id. monthStart partitions must already exist.
func insertCall(t *testing.T, db *DB, systemID, tgid int, startTime time.Time) int64 {
	t.Helper()
	var callID int64
	err := db.Pool.QueryRow(context.Background(),
		`INSERT INTO calls (system_id, tgid, start_time) VALUES ($1, $2, $3) RETURNING call_id`,
		systemID, tgid, startTime).Scan(&callID)
	if err != nil {
		t.Fatalf("insert call: %v", err)
	}
	mustExec(t, db, `INSERT INTO call_frequencies (call_id, call_start_time, freq) VALUES ($1, $2, 851000000)`, callID, startTime)
	mustExec(t, db, `INSERT INTO call_transmissions (call_id, call_start_time, src) VALUES ($1, $2, 12345)`, callID, startTime)
	mustExec(t, db, `INSERT INTO transcriptions (call_id, call_start_time, source) VALUES ($1, $2, 'auto')`, callID, startTime)
	return callID
}

// TestCallRetention covers the monthly call-family retention end to end,
// including the boundary subtlety: transcriptions belonging to a not-yet-expired
// month must survive even when their timestamp is older than the raw cutoff.
//
// Month layout (relative to now):
//
//	M_old   = 2 months ago   -> fully expired, must be dropped
//	M_bound = last month      -> NOT fully expired (its upper bound is the start
//	                             of the current month), must be kept
//	M_curr  = current month   -> kept
//
// Retention is chosen so the raw cutoff lands ~15 days before the current
// month's start (i.e. inside M_bound). A naive implementation deleting
// transcriptions by raw cutoff would wrongly delete M_bound's early-month
// transcription; the correct implementation deletes by the partition boundary
// (M_old's upper bound), preserving it.
func TestCallRetention(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	s0 := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC) // start of current month
	sBound := s0.AddDate(0, -1, 0)                                    // start of last month (M_bound)
	sOld := s0.AddDate(0, -2, 0)                                      // start of 2-months-ago (M_old)
	// cutoff = now - retention = s0 - 15d  ->  s_old < cutoff <= s_bound's upper? see header.
	retention := now.Sub(s0) + 15*24*time.Hour

	for _, m := range []time.Time{sOld, sBound, s0} {
		createMonthlyPart(t, db, "calls", m)
		createMonthlyPart(t, db, "call_frequencies", m)
		createMonthlyPart(t, db, "call_transmissions", m)
	}

	var systemID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO systems (system_type, name) VALUES ('p25', 'retention-test') RETURNING system_id`).Scan(&systemID); err != nil {
		t.Fatalf("insert system: %v", err)
	}

	// Scope assertions to this run's own system_id / call_ids so the test is
	// isolated from any data left by a prior run against the same database
	// (partitions and the transcriptions DELETE are global by nature).
	callOld := insertCall(t, db, systemID, 100, sOld)    // expect dropped + transcription deleted
	callBound := insertCall(t, db, systemID, 101, sBound) // expect kept + transcription preserved
	callCurr := insertCall(t, db, systemID, 102, s0)      // expect kept

	res, err := db.DropOldCallPartitions(ctx, retention)
	if err != nil {
		t.Fatalf("DropOldCallPartitions: %v", err)
	}

	// Exactly one calendar month (M_old) is fully expired.
	if len(res.CallPartitionsDropped) != 1 {
		t.Errorf("CallPartitionsDropped = %v, want exactly 1 (M_old)", res.CallPartitionsDropped)
	}
	// Its call_frequencies + call_transmissions partitions also drop.
	if len(res.ChildPartitionsDropped) != 2 {
		t.Errorf("ChildPartitionsDropped = %v, want 2 (freq+trans for M_old)", res.ChildPartitionsDropped)
	}
	if res.TranscriptionsDeleted < 1 {
		t.Errorf("TranscriptionsDeleted = %d, want >= 1 (the M_old transcription)", res.TranscriptionsDeleted)
	}

	// M_old data is gone from every call-family table (scoped to this system).
	if c := count(t, db, `SELECT count(*) FROM calls WHERE system_id = $1 AND start_time >= $2 AND start_time < $3`, systemID, sOld, sBound); c != 0 {
		t.Errorf("calls rows remaining in M_old = %d, want 0", c)
	}
	if c := count(t, db, `SELECT count(*) FROM transcriptions WHERE call_id = $1`, callOld); c != 0 {
		t.Errorf("M_old transcription still present (call_id=%d), want deleted", callOld)
	}

	// Boundary correctness: M_bound's transcription survives even though its
	// timestamp is older than the raw cutoff — deletion is bounded by the
	// dropped-partition boundary, not the raw cutoff.
	if c := count(t, db, `SELECT count(*) FROM calls WHERE call_id = $1 AND start_time = $2`, callBound, sBound); c != 1 {
		t.Errorf("M_bound call count = %d, want 1 (must not be dropped)", c)
	}
	if c := count(t, db, `SELECT count(*) FROM transcriptions WHERE call_id = $1`, callBound); c != 1 {
		t.Errorf("M_bound transcription count = %d, want 1 (boundary preservation)", c)
	}
	if c := count(t, db, `SELECT count(*) FROM calls WHERE call_id = $1 AND start_time = $2`, callCurr, s0); c != 1 {
		t.Errorf("M_curr call count = %d, want 1", c)
	}
}

// TestDropOldCallPartitionsDisabled verifies retention is a no-op when disabled.
func TestDropOldCallPartitionsDisabled(t *testing.T) {
	db := testDB(t)
	res, err := db.DropOldCallPartitions(context.Background(), 0)
	if err != nil {
		t.Fatalf("DropOldCallPartitions(0): %v", err)
	}
	if len(res.CallPartitionsDropped) != 0 || len(res.ChildPartitionsDropped) != 0 || res.TranscriptionsDeleted != 0 {
		t.Errorf("disabled retention should be a no-op, got %+v", res)
	}
}

// TestDropOldWeeklyPartitionsParsesTimestamptzBounds is the Bug 1 regression
// guard: mqtt_raw_messages is partitioned on a timestamptz column, so its
// partition bounds render as full timestamps (e.g. '2026-05-18 00:00:00+00').
// The previous date-only time.Parse layout failed on those and silently skipped
// every partition. This test proves expired weekly partitions are actually
// dropped, recent ones are kept, and a DEFAULT partition (NULL bound) is never
// touched.
func TestDropOldWeeklyPartitionsParsesTimestamptzBounds(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	oldWeek := now.AddDate(0, 0, -42) // 6 weeks ago — well past a 7-day retention

	mustExec(t, db, `SELECT create_weekly_partition($1, $2::date)`, "mqtt_raw_messages", oldWeek.Format("2006-01-02"))
	mustExec(t, db, `SELECT create_weekly_partition($1, $2::date)`, "mqtt_raw_messages", now.Format("2006-01-02"))
	// DEFAULT partition: its relpartbound is DEFAULT (no TO bound), so the
	// extractor yields NULL and it must never be considered for dropping.
	mustExec(t, db, `CREATE TABLE IF NOT EXISTS mqtt_raw_messages_testdefault PARTITION OF mqtt_raw_messages DEFAULT`)

	// Names follow create_weekly_partition's IYYY_IW convention.
	var oldName, curName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT 'mqtt_raw_messages_w' || to_char($1::date, 'IYYY') || '_' || to_char($1::date, 'IW')`,
		oldWeek).Scan(&oldName); err != nil {
		t.Fatalf("derive old name: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT 'mqtt_raw_messages_w' || to_char($1::date, 'IYYY') || '_' || to_char($1::date, 'IW')`,
		now).Scan(&curName); err != nil {
		t.Fatalf("derive current name: %v", err)
	}

	// expiredPartitions must parse the timestamptz bound and flag the old one,
	// while never returning the DEFAULT partition.
	exp, err := db.expiredPartitions(ctx, "mqtt_raw_messages", now.Add(-168*time.Hour))
	if err != nil {
		t.Fatalf("expiredPartitions: %v", err)
	}
	var sawOld, sawDefault bool
	for _, p := range exp {
		if p.name == oldName {
			sawOld = true
		}
		if p.name == "mqtt_raw_messages_testdefault" {
			sawDefault = true
		}
	}
	if !sawOld {
		t.Errorf("expiredPartitions did not flag old partition %q (timestamptz bound parse failed?); got %d entries", oldName, len(exp))
	}
	if sawDefault {
		t.Error("expiredPartitions returned the DEFAULT partition (NULL bound must be skipped)")
	}

	dropped, err := db.DropOldWeeklyPartitions(ctx, "mqtt_raw_messages", 168*time.Hour)
	if err != nil {
		t.Fatalf("DropOldWeeklyPartitions: %v", err)
	}
	var droppedOld bool
	for _, n := range dropped {
		if n == oldName {
			droppedOld = true
		}
		if n == curName || n == "mqtt_raw_messages_testdefault" {
			t.Errorf("dropped a partition that should be kept: %q", n)
		}
	}
	if !droppedOld {
		t.Errorf("old partition %q was not dropped (Bug 1 regression)", oldName)
	}
	if tableExists(t, db, oldName) {
		t.Errorf("old partition %q still exists after drop", oldName)
	}
	if !tableExists(t, db, curName) {
		t.Errorf("current-week partition %q was removed but should be kept", curName)
	}
	if !tableExists(t, db, "mqtt_raw_messages_testdefault") {
		t.Error("DEFAULT partition was removed but must be kept")
	}
}
