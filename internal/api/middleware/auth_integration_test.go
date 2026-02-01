package middleware_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/api/middleware"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	opts := testutil.DefaultTestDBOptions()
	testDB = testutil.NewTestDBForMain(opts)
	if testDB == nil {
		os.Exit(1)
	}

	code := m.Run()

	testDB.Close()
	os.Exit(code)
}

func setupAuthTest(t *testing.T) *middleware.AuthMiddleware {
	t.Helper()
	testDB.Reset(t)
	return middleware.NewAuthMiddleware(config.AuthConfig{Enabled: true}, testDB.DB)
}

// ============================================================================
// Database-Backed API Key Tests
// ============================================================================

func TestAuthMiddleware_CreateAPIKey(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	plaintext, key, err := auth.CreateAPIKey(ctx, "Test API Key", []string{"read"}, false, nil)
	require.NoError(t, err)

	// Verify plaintext key format
	assert.True(t, strings.HasPrefix(plaintext, "tr_api_"), "plaintext should start with tr_api_")
	assert.Equal(t, 71, len(plaintext), "plaintext should be 71 chars")

	// Verify returned key
	assert.NotZero(t, key.ID)
	assert.Equal(t, "Test API Key", key.Name)
	assert.Equal(t, []string{"read"}, key.Scopes)
	assert.False(t, key.ReadOnly)
	assert.NotZero(t, key.CreatedAt)
	assert.True(t, strings.HasPrefix(key.KeyPrefix, "tr_api_"))
}

func TestAuthMiddleware_CreateAPIKey_WithExpiration(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	plaintext, key, err := auth.CreateAPIKey(ctx, "Expiring Key", nil, true, &expiresAt)
	require.NoError(t, err)

	assert.NotEmpty(t, plaintext)
	assert.True(t, key.ReadOnly)
	require.NotNil(t, key.ExpiresAt)
	assert.WithinDuration(t, expiresAt, *key.ExpiresAt, time.Second)
}

func TestAuthMiddleware_ListAPIKeys(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create multiple keys
	_, _, err := auth.CreateAPIKey(ctx, "Key 1", nil, false, nil)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	_, _, err = auth.CreateAPIKey(ctx, "Key 2", nil, false, nil)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	_, _, err = auth.CreateAPIKey(ctx, "Key 3", nil, false, nil)
	require.NoError(t, err)

	keys, err := auth.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 3)

	// Should be newest first
	assert.Equal(t, "Key 3", keys[0].Name)
	assert.Equal(t, "Key 2", keys[1].Name)
	assert.Equal(t, "Key 1", keys[2].Name)
}

func TestAuthMiddleware_ListAPIKeys_Empty(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	keys, err := auth.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestAuthMiddleware_RevokeAPIKey(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create a key
	_, key, err := auth.CreateAPIKey(ctx, "Revokable Key", nil, false, nil)
	require.NoError(t, err)
	assert.Nil(t, key.RevokedAt)

	// Revoke it
	err = auth.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Verify it's revoked by listing
	keys, err := auth.ListAPIKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].RevokedAt)
}

func TestAuthMiddleware_DeleteAPIKey(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create a key
	_, key, err := auth.CreateAPIKey(ctx, "Deletable Key", nil, false, nil)
	require.NoError(t, err)

	// Delete it
	err = auth.DeleteAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// Verify it's gone
	keys, err := auth.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// ============================================================================
// Database-Backed API Key Authentication Tests
// ============================================================================

func TestAuthMiddleware_ValidateDatabaseKey(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create a database key
	plaintext, _, err := auth.CreateAPIKey(ctx, "Auth Test Key", nil, false, nil)
	require.NoError(t, err)

	// The key should be valid when used in API key authentication
	// We test this by verifying the hash lookup works
	keyHash := middleware.HashAPIKey(plaintext)
	key, err := testDB.DB.GetAPIKeyByHash(ctx, keyHash)
	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "Auth Test Key", key.Name)
}

func TestAuthMiddleware_RevokedKeyNotValid(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create and revoke a key
	plaintext, key, err := auth.CreateAPIKey(ctx, "Soon Revoked", nil, false, nil)
	require.NoError(t, err)

	err = auth.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	// The key hash can still be found, but revoked_at is set
	keyHash := middleware.HashAPIKey(plaintext)
	foundKey, err := testDB.DB.GetAPIKeyByHash(ctx, keyHash)
	require.NoError(t, err)
	require.NotNil(t, foundKey)
	assert.NotNil(t, foundKey.RevokedAt, "revoked key should have revoked_at set")
}

func TestAuthMiddleware_ExpiredKeyCheck(t *testing.T) {
	auth := setupAuthTest(t)
	ctx := context.Background()

	// Create a key that already expired
	expiredTime := time.Now().Add(-1 * time.Hour)
	plaintext, _, err := auth.CreateAPIKey(ctx, "Already Expired", nil, false, &expiredTime)
	require.NoError(t, err)

	// The key can be found but has a past expiration
	keyHash := middleware.HashAPIKey(plaintext)
	foundKey, err := testDB.DB.GetAPIKeyByHash(ctx, keyHash)
	require.NoError(t, err)
	require.NotNil(t, foundKey)
	assert.True(t, foundKey.ExpiresAt.Before(time.Now()), "expired key should have past expiration")
}

// ============================================================================
// Config + Database Key Coexistence Tests
// ============================================================================

func TestAuthMiddleware_ConfigAndDatabaseKeys(t *testing.T) {
	testDB.Reset(t)
	ctx := context.Background()

	// Create auth with both config keys and database
	auth := middleware.NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"config_key_1", "config_key_2"},
	}, testDB.DB)

	// Create database keys
	_, _, err := auth.CreateAPIKey(ctx, "DB Key 1", nil, false, nil)
	require.NoError(t, err)
	_, _, err = auth.CreateAPIKey(ctx, "DB Key 2", nil, false, nil)
	require.NoError(t, err)

	// List should only show database keys (config keys are not in DB)
	keys, err := auth.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestAuthMiddleware_NilDatabase(t *testing.T) {
	// Auth with no database can still use config keys for authentication
	// but database key operations (CreateAPIKey, ListAPIKeys, etc.) require a database
	auth := middleware.NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"config_only_key"},
	}, nil)

	// Auth middleware is enabled and can check config keys
	assert.True(t, auth.IsEnabled())

	// Note: Calling CreateAPIKey, ListAPIKeys, etc. with nil database will panic
	// This is expected - these methods require a database connection
}
