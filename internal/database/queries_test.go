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

func TestGetTalkgroup_BySysidAndTGID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	tg, err := testDB.DB.GetTalkgroup(ctx, f.MetroFireSYSID, f.MetroFireTGID)
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

	call, err := testDB.DB.GetCallByID(ctx, f.Call1DBID)
	require.NoError(t, err)
	assert.Equal(t, f.Call1DBID, call.ID)
	assert.Equal(t, "1705312200_850387500_9178", call.TRCallID)
	assert.False(t, call.Encrypted)
}

func TestGetCallByTRID(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	call, err := testDB.DB.GetCallByTRID(ctx, "1705312200_850387500_9178", f.BaseTime.Add(-30*time.Second))
	require.NoError(t, err)
	assert.Equal(t, f.Call1DBID, call.ID)
}

func TestListCalls_FilterBySystem(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	systemID := f.MetroSystemID
	calls, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, nil, 100, 0)
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

	tgID := f.MetroPDMainTGID
	calls, err := testDB.DB.ListCalls(ctx, nil, nil, &tgID, nil, nil, 100, 0)
	require.NoError(t, err)

	// Metro PD Main (tgid 9178) has calls in both Metro and County systems
	// Metro has 2 calls with audio (call 5 is encrypted, no audio)
	// County has 1 call with audio
	// Total: 3 calls
	assert.Len(t, calls, 3)
}

func TestListCalls_FilterByTimeRange(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	// Get calls from last 60 seconds only
	startTime := f.BaseTime.Add(-60 * time.Second)
	endTime := f.BaseTime
	calls, err := testDB.DB.ListCalls(ctx, nil, nil, nil, &startTime, &endTime, 100, 0)
	require.NoError(t, err)

	// Should include Call 1 (30s ago) and Call 2 (60s ago), but not Call 3 (90s ago) or Call 4 (120s ago)
	assert.Len(t, calls, 2)
}

func TestListCalls_Pagination(t *testing.T) {
	f := setupTest(t)
	ctx := context.Background()

	systemID := f.MetroSystemID

	// Get first 2
	calls1, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, nil, 2, 0)
	require.NoError(t, err)
	assert.Len(t, calls1, 2)

	// Get next 2
	calls2, err := testDB.DB.ListCalls(ctx, &systemID, nil, nil, nil, nil, 2, 2)
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

	txs, err := testDB.DB.GetTransmissionsByCallID(ctx, f.Call1DBID)
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

// ============================================================================
// API Key Tests
// ============================================================================

func TestCreateAPIKey(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	key, err := testDB.DB.CreateAPIKey(ctx, "hash123", "tr_api_abc", "Test Key", []string{"read"}, false, nil)
	require.NoError(t, err)
	assert.Equal(t, "tr_api_abc", key.KeyPrefix)
	assert.Equal(t, "Test Key", key.Name)
	assert.Equal(t, []string{"read"}, key.Scopes)
	assert.False(t, key.ReadOnly)
	assert.NotZero(t, key.CreatedAt)
	assert.Nil(t, key.LastUsedAt)
	assert.Nil(t, key.ExpiresAt)
	assert.Nil(t, key.RevokedAt)
}

func TestCreateAPIKey_WithExpiration(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	expiresAt := time.Now().Add(24 * time.Hour)
	key, err := testDB.DB.CreateAPIKey(ctx, "hash456", "tr_api_def", "Expiring Key", nil, true, &expiresAt)
	require.NoError(t, err)
	assert.Equal(t, "Expiring Key", key.Name)
	assert.True(t, key.ReadOnly)
	assert.NotNil(t, key.ExpiresAt)
	assert.WithinDuration(t, expiresAt, *key.ExpiresAt, time.Second)
}

func TestCreateAPIKey_DuplicateHash(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	_, err := testDB.DB.CreateAPIKey(ctx, "duplicate_hash", "tr_api_1", "Key 1", nil, false, nil)
	require.NoError(t, err)

	// Same hash should fail
	_, err = testDB.DB.CreateAPIKey(ctx, "duplicate_hash", "tr_api_2", "Key 2", nil, false, nil)
	assert.Error(t, err)
}

func TestGetAPIKeyByHash(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create a key first
	created, err := testDB.DB.CreateAPIKey(ctx, "lookup_hash", "tr_api_xyz", "Lookup Key", []string{"admin"}, false, nil)
	require.NoError(t, err)

	// Look it up
	found, err := testDB.DB.GetAPIKeyByHash(ctx, "lookup_hash")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "Lookup Key", found.Name)
	assert.Equal(t, []string{"admin"}, found.Scopes)
}

func TestGetAPIKeyByHash_NotFound(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	key, err := testDB.DB.GetAPIKeyByHash(ctx, "nonexistent_hash")
	assert.NoError(t, err)
	assert.Nil(t, key)
}

func TestListAPIKeys(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create multiple keys
	_, err := testDB.DB.CreateAPIKey(ctx, "hash_a", "tr_api_aaa", "Key A", nil, false, nil)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond) // Ensure different created_at
	_, err = testDB.DB.CreateAPIKey(ctx, "hash_b", "tr_api_bbb", "Key B", nil, false, nil)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	_, err = testDB.DB.CreateAPIKey(ctx, "hash_c", "tr_api_ccc", "Key C", nil, false, nil)
	require.NoError(t, err)

	keys, err := testDB.DB.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 3)

	// Should be ordered by created_at DESC (newest first)
	assert.Equal(t, "Key C", keys[0].Name)
	assert.Equal(t, "Key B", keys[1].Name)
	assert.Equal(t, "Key A", keys[2].Name)
}

func TestListAPIKeys_Empty(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	keys, err := testDB.DB.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestUpdateAPIKeyLastUsed(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create a key
	key, err := testDB.DB.CreateAPIKey(ctx, "usage_hash", "tr_api_use", "Usage Key", nil, false, nil)
	require.NoError(t, err)
	assert.Nil(t, key.LastUsedAt)

	// Update last used
	err = testDB.DB.UpdateAPIKeyLastUsed(ctx, key.ID)
	require.NoError(t, err)

	// Verify it was updated
	updated, err := testDB.DB.GetAPIKeyByHash(ctx, "usage_hash")
	require.NoError(t, err)
	assert.NotNil(t, updated.LastUsedAt)
	assert.WithinDuration(t, time.Now(), *updated.LastUsedAt, 5*time.Second)
}

func TestRevokeAPIKey(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create a key
	key, err := testDB.DB.CreateAPIKey(ctx, "revoke_hash", "tr_api_rev", "Revoke Key", nil, false, nil)
	require.NoError(t, err)
	assert.Nil(t, key.RevokedAt)

	// Revoke it
	err = testDB.DB.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Verify it was revoked
	revoked, err := testDB.DB.GetAPIKeyByHash(ctx, "revoke_hash")
	require.NoError(t, err)
	assert.NotNil(t, revoked.RevokedAt)
	assert.WithinDuration(t, time.Now(), *revoked.RevokedAt, 5*time.Second)
}

func TestRevokeAPIKey_AlreadyRevoked(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create and revoke a key
	key, err := testDB.DB.CreateAPIKey(ctx, "double_revoke", "tr_api_dr", "Double Revoke", nil, false, nil)
	require.NoError(t, err)

	err = testDB.DB.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Get the revoked time
	revoked, _ := testDB.DB.GetAPIKeyByHash(ctx, "double_revoke")
	firstRevoke := *revoked.RevokedAt

	// Try to revoke again (should not update)
	time.Sleep(10 * time.Millisecond)
	err = testDB.DB.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Verify revoked_at didn't change
	revoked2, _ := testDB.DB.GetAPIKeyByHash(ctx, "double_revoke")
	assert.Equal(t, firstRevoke.Unix(), revoked2.RevokedAt.Unix())
}

func TestDeleteAPIKey(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Create a key
	key, err := testDB.DB.CreateAPIKey(ctx, "delete_hash", "tr_api_del", "Delete Key", nil, false, nil)
	require.NoError(t, err)

	// Delete it
	err = testDB.DB.DeleteAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Verify it's gone
	deleted, err := testDB.DB.GetAPIKeyByHash(ctx, "delete_hash")
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestDeleteAPIKey_NonExistent(t *testing.T) {
	_ = setupTest(t)
	ctx := context.Background()

	// Deleting non-existent key should not error
	err := testDB.DB.DeleteAPIKey(ctx, 99999)
	assert.NoError(t, err)
}

// Ensure database package is imported for side effects
var _ = database.New
