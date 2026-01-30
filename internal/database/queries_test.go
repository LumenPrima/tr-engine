package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	// Set up shared test database
	opts := testutil.DefaultTestDBOptions()
	testDB = testutil.NewTestDBForMain(opts)
	if testDB == nil {
		os.Exit(1)
	}

	code := m.Run()

	testDB.Close()
	os.Exit(code)
}

func setupTest(t *testing.T) *testutil.TestFixtures {
	t.Helper()
	testDB.Reset(t)
	return testutil.SeedTestData(t, testDB)
}

// ============================================================================
// System Tests
// ============================================================================

func TestGetSystemByShortName(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	sys, err := testDB.DB.GetSystemByShortName(ctx, "metro")
	require.NoError(t, err)
	assert.Equal(t, "metro", sys.ShortName)
	assert.Equal(t, "348", sys.SysID)
	assert.Equal(t, "p25", sys.SystemType)
}

func TestGetSystemByShortName_NotFound(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	sys, err := testDB.DB.GetSystemByShortName(ctx, "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, sys)
}

func TestListSystems(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	systems, err := testDB.DB.ListSystems(ctx)
	require.NoError(t, err)
	assert.Len(t, systems, 2)
}

// ============================================================================
// Talkgroup Tests - SYSID Scoping
// ============================================================================

func TestGetTalkgroup_BySysidAndTgid(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Get Metro PD Main (sysid:348, tgid:9178)
	tg, err := testDB.DB.GetTalkgroup(ctx, "348", 9178)
	require.NoError(t, err)
	assert.Equal(t, "Metro PD Main", tg.AlphaTag)
	assert.Equal(t, "348", tg.SYSID)
	assert.Equal(t, 9178, tg.TGID)

	// Get County PD Main (sysid:15a, tgid:9178) - same tgid, different sysid
	tg2, err := testDB.DB.GetTalkgroup(ctx, "15a", 9178)
	require.NoError(t, err)
	assert.Equal(t, "County PD Main", tg2.AlphaTag)
	assert.Equal(t, "15a", tg2.SYSID)
}

func TestGetTalkgroup_NotFound(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	tg, err := testDB.DB.GetTalkgroup(ctx, "348", 99999)
	assert.NoError(t, err)
	assert.Nil(t, tg)
}

func TestGetTalkgroupsByTGID_MultipleMatches(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// TGID 9178 exists in both systems
	tgs, err := testDB.DB.GetTalkgroupsByTGID(ctx, 9178)
	require.NoError(t, err)
	assert.Len(t, tgs, 2)

	// Verify both systems are represented
	sysids := make(map[string]bool)
	for _, tg := range tgs {
		sysids[tg.SYSID] = true
	}
	assert.True(t, sysids["348"])
	assert.True(t, sysids["15a"])
}

func TestGetTalkgroupsByTGID_SingleMatch(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// TGID 9200 only exists in metro system
	tgs, err := testDB.DB.GetTalkgroupsByTGID(ctx, 9200)
	require.NoError(t, err)
	assert.Len(t, tgs, 1)
	assert.Equal(t, "Metro Fire", tgs[0].AlphaTag)
}

func TestGetTalkgroupByID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	tg, err := testDB.DB.GetTalkgroupByID(ctx, f.MetroFireID)
	require.NoError(t, err)
	assert.Equal(t, "Metro Fire", tg.AlphaTag)
	assert.Equal(t, 9200, tg.TGID)
}

// ============================================================================
// Unit Tests - SYSID Scoping
// ============================================================================

func TestGetUnit_BySysidAndUnitID(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Get Metro unit (sysid:348, unit_id:1001234)
	unit, err := testDB.DB.GetUnit(ctx, "348", 1001234)
	require.NoError(t, err)
	assert.Equal(t, "Metro Car 123", unit.AlphaTag)
	assert.Equal(t, "348", unit.SYSID)

	// Get County unit (sysid:15a, unit_id:1001234) - same unit_id, different sysid
	unit2, err := testDB.DB.GetUnit(ctx, "15a", 1001234)
	require.NoError(t, err)
	assert.Equal(t, "County Unit 1", unit2.AlphaTag)
	assert.Equal(t, "15a", unit2.SYSID)
}

func TestGetUnit_NotFound(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	unit, err := testDB.DB.GetUnit(ctx, "348", 9999999)
	assert.NoError(t, err)
	assert.Nil(t, unit)
}

// ============================================================================
// Call Tests
// ============================================================================

func TestGetCallByID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	call, err := testDB.DB.GetCallByID(ctx, f.Call1ID)
	require.NoError(t, err)
	assert.Equal(t, f.Call1ID, call.ID)
	assert.Equal(t, "1705312200_850387500_9178", call.TRCallID)
	assert.False(t, call.Encrypted)
}

func TestGetCallByTRID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	call, err := testDB.DB.GetCallByTRID(ctx, "1705312200_850387500_9178", f.BaseTime.Add(-30*time.Second))
	require.NoError(t, err)
	assert.Equal(t, f.Call1ID, call.ID)
}

func TestListCalls_FilterBySystem(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	systemID := f.MetroSystemID
	calls, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, 100, 0)
	require.NoError(t, err)

	// Metro system has 4 calls (including encrypted one without audio)
	// But ListCalls filters to audio_path IS NOT NULL, so should be 3
	assert.Len(t, calls, 3)

	// All should be metro system
	for _, call := range calls {
		assert.Equal(t, f.MetroSystemID, call.SystemID)
	}
}

func TestListCalls_FilterByTalkgroup(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	tgID := f.MetroPDMainID
	calls, err := testDB.DB.ListCalls(ctx, nil, &tgID, nil, nil, 100, 0)
	require.NoError(t, err)

	// Metro PD Main has 2 calls with audio (call 5 is encrypted, no audio)
	assert.Len(t, calls, 2)
}

func TestListCalls_FilterByTimeRange(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	// Get calls from last 60 seconds only
	startTime := f.BaseTime.Add(-60 * time.Second)
	endTime := f.BaseTime
	calls, err := testDB.DB.ListCalls(ctx, nil, nil, &startTime, &endTime, 100, 0)
	require.NoError(t, err)

	// Should include Call 1 (30s ago) and Call 2 (60s ago), but not Call 3 (90s ago) or Call 4 (120s ago)
	assert.Len(t, calls, 2)
}

func TestListCalls_Pagination(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	systemID := f.MetroSystemID

	// Get first 2
	calls1, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, 2, 0)
	require.NoError(t, err)
	assert.Len(t, calls1, 2)

	// Get next 2
	calls2, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, 2, 2)
	require.NoError(t, err)
	assert.Len(t, calls2, 1) // Only 3 total with audio

	// IDs should not overlap
	assert.NotEqual(t, calls1[0].ID, calls2[0].ID)
	assert.NotEqual(t, calls1[1].ID, calls2[0].ID)
}

// ============================================================================
// Transmission Tests
// ============================================================================

func TestGetTransmissionsByCallID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	txs, err := testDB.DB.GetTransmissionsByCallID(ctx, f.Call1ID)
	require.NoError(t, err)
	assert.Len(t, txs, 3)

	// Verify transmissions are ordered by position
	assert.Equal(t, float32(0.0), txs[0].Position)
	assert.Equal(t, float32(5.5), txs[1].Position)
	assert.Equal(t, float32(10.5), txs[2].Position)

	// Verify unit RIDs
	assert.Equal(t, int64(1001234), txs[0].UnitRID)
	assert.Equal(t, int64(1001235), txs[1].UnitRID)
	assert.Equal(t, int64(1001234), txs[2].UnitRID)
}

// ============================================================================
// Upsert Tests
// ============================================================================

func TestUpsertTalkgroup_Insert(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	tg, err := testDB.DB.UpsertTalkgroup(ctx, "348", 9999, "New TG", "Description", "Test", "Test Tag", 5, "D")
	require.NoError(t, err)
	assert.Equal(t, "New TG", tg.AlphaTag)
	assert.Equal(t, 9999, tg.TGID)
	assert.Equal(t, "348", tg.SYSID)
}

func TestUpsertTalkgroup_Update(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Update existing talkgroup
	tg, err := testDB.DB.UpsertTalkgroup(ctx, "348", 9178, "Updated Metro PD", "New Description", "Law", "Law Dispatch", 1, "D")
	require.NoError(t, err)
	assert.Equal(t, "Updated Metro PD", tg.AlphaTag)

	// Verify it was updated
	tg2, err := testDB.DB.GetTalkgroup(ctx, "348", 9178)
	require.NoError(t, err)
	assert.Equal(t, "Updated Metro PD", tg2.AlphaTag)
}

func TestUpsertUnit_Insert(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	unit, err := testDB.DB.UpsertUnit(ctx, "348", 9999999, "New Unit", "test")
	require.NoError(t, err)
	assert.Equal(t, "New Unit", unit.AlphaTag)
	assert.Equal(t, int64(9999999), unit.UnitID)
}

func TestUpsertUnit_Update(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Update existing unit
	unit, err := testDB.DB.UpsertUnit(ctx, "348", 1001234, "Updated Car 123", "manual")
	require.NoError(t, err)
	assert.Equal(t, "Updated Car 123", unit.AlphaTag)

	// Verify it was updated
	unit2, err := testDB.DB.GetUnit(ctx, "348", 1001234)
	require.NoError(t, err)
	assert.Equal(t, "Updated Car 123", unit2.AlphaTag)
}

// ============================================================================
// P25 System Detection
// ============================================================================

func TestIsP25System(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	// Metro is P25
	isP25, err := testDB.DB.IsP25System(ctx, f.MetroSystemID)
	require.NoError(t, err)
	assert.True(t, isP25)
}

// Ensure database package is imported for side effects
var _ = database.New
