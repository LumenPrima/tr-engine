package dedup

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
	"go.uber.org/zap"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	// Use a wrapper to catch panics and ensure cleanup
	code := runTests(m)
	os.Exit(code)
}

func runTests(m *testing.M) int {
	opts := testutil.DefaultTestDBOptions()

	// Use TR_TEST_EPHEMERAL=false to keep database for debugging
	if os.Getenv("TR_TEST_EPHEMERAL") == "false" {
		opts.Ephemeral = false
		opts.PersistentPath = ".testdb"
	}

	// Use a stub that implements testing.TB for TestMain context
	testDB = testutil.NewTestDBForMain(opts)
	if testDB == nil {
		return 1
	}
	defer testDB.Close()

	return m.Run()
}

func setupTestData(t *testing.T, db *testutil.TestDB) (systemID int, sysid string, tgid int) {
	t.Helper()
	ctx := context.Background()

	// Insert test instance
	var instanceID int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO instances (instance_id, instance_key, first_seen, last_seen)
		VALUES ('test-instance', 'test-key', NOW(), NOW())
		RETURNING id
	`).Scan(&instanceID)
	require.NoError(t, err)

	// Insert test system
	sysid = "TEST"
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO systems (instance_id, sys_num, short_name, system_type, sysid)
		VALUES ($1, 1, 'TEST', 'P25', $2)
		RETURNING id
	`, instanceID, sysid).Scan(&systemID)
	require.NoError(t, err)

	// Insert test talkgroup (natural key: sysid, tgid)
	tgid = 12345
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO talkgroups (sysid, tgid, alpha_tag, first_seen, last_seen)
		VALUES ($1, $2, 'Test TG', NOW(), NOW())
	`, sysid, tgid)
	require.NoError(t, err)

	return systemID, sysid, tgid
}

func TestEngine_IsEnabled(t *testing.T) {
	db := testDB
	logger, _ := zap.NewDevelopment()

	t.Run("enabled", func(t *testing.T) {
		cfg := config.DeduplicationConfig{Enabled: true}
		engine := NewEngine(db.DB, cfg, logger)
		assert.True(t, engine.IsEnabled())
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.DeduplicationConfig{Enabled: false}
		engine := NewEngine(db.DB, cfg, logger)
		assert.False(t, engine.IsEnabled())
	})
}

func TestEngine_ScoreMatch(t *testing.T) {
	db := testDB
	logger, _ := zap.NewDevelopment()
	cfg := config.DeduplicationConfig{Enabled: true, Threshold: 0.7}
	engine := NewEngine(db.DB, cfg, logger)

	now := time.Now()
	stopTime := now.Add(30 * time.Second)
	endTime := now.Add(30 * time.Second)

	t.Run("perfect match", func(t *testing.T) {
		call := &models.Call{
			SystemID:  1,
			StartTime: now,
			StopTime:  &stopTime,
			Duration:  30.0,
			Emergency: false,
			Encrypted: false,
		}

		group := &models.CallGroup{
			SystemID:  1,
			StartTime: now,
			EndTime:   &endTime,
			Emergency: false,
			Encrypted: false,
		}

		score := engine.scoreMatch(call, group, false)
		assert.Greater(t, score, 0.9, "Perfect match should score above 0.9")
	})

	t.Run("different system", func(t *testing.T) {
		// Use different duration to ensure score is clearly below threshold
		// (no system match = -30 points)
		call := &models.Call{
			SystemID:  1,
			StartTime: now,
			StopTime:  &stopTime,
			Duration:  30.0,
		}

		groupEnd := now.Add(60 * time.Second) // Different duration
		group := &models.CallGroup{
			SystemID:  2,
			StartTime: now,
			EndTime:   &groupEnd,
		}

		score := engine.scoreMatch(call, group, false)
		assert.Less(t, score, 0.7, "Different system should score below threshold")
	})

	t.Run("no overlap", func(t *testing.T) {
		laterStop := now.Add(90 * time.Second)
		call := &models.Call{
			SystemID:  1,
			StartTime: now.Add(60 * time.Second),
			StopTime:  &laterStop,
			Duration:  30.0,
		}

		group := &models.CallGroup{
			SystemID:  1,
			StartTime: now,
			EndTime:   &endTime,
		}

		score := engine.scoreMatch(call, group, false)
		assert.Less(t, score, 0.7, "Non-overlapping calls should score below threshold")
	})
}

func TestEngine_CalculateOverlap(t *testing.T) {
	db := testDB
	logger, _ := zap.NewDevelopment()
	cfg := config.DeduplicationConfig{Enabled: true}
	engine := NewEngine(db.DB, cfg, logger)

	now := time.Now()

	t.Run("full overlap", func(t *testing.T) {
		stopTime := now.Add(30 * time.Second)
		endTime := now.Add(30 * time.Second)

		call := &models.Call{
			StartTime: now,
			StopTime:  &stopTime,
		}
		group := &models.CallGroup{
			StartTime: now,
			EndTime:   &endTime,
		}

		overlap := engine.calculateOverlap(call, group)
		assert.Equal(t, 1.0, overlap)
	})

	t.Run("half overlap", func(t *testing.T) {
		stopTime := now.Add(30 * time.Second)
		groupEnd := now.Add(15 * time.Second)

		call := &models.Call{
			StartTime: now,
			StopTime:  &stopTime,
		}
		group := &models.CallGroup{
			StartTime: now,
			EndTime:   &groupEnd,
		}

		overlap := engine.calculateOverlap(call, group)
		assert.Equal(t, 0.5, overlap)
	})

	t.Run("no overlap", func(t *testing.T) {
		callStop := now.Add(30 * time.Second)
		groupEnd := now.Add(90 * time.Second)

		call := &models.Call{
			StartTime: now,
			StopTime:  &callStop,
		}
		group := &models.CallGroup{
			StartTime: now.Add(60 * time.Second),
			EndTime:   &groupEnd,
		}

		overlap := engine.calculateOverlap(call, group)
		assert.Equal(t, 0.0, overlap)
	})
}

func TestEngine_ProcessCall_Disabled(t *testing.T) {
	db := testDB
	db.Reset(t)
	logger, _ := zap.NewDevelopment()

	cfg := config.DeduplicationConfig{Enabled: false}
	engine := NewEngine(db.DB, cfg, logger)

	call := &models.Call{StartTime: time.Now()}

	group, err := engine.ProcessCall(context.Background(), call, 12345, "test_system")
	assert.NoError(t, err)
	assert.Nil(t, group, "Should return nil group when disabled")
}

func TestHelperFunctions(t *testing.T) {
	t.Run("abs", func(t *testing.T) {
		assert.Equal(t, float32(5.0), abs(5.0))
		assert.Equal(t, float32(5.0), abs(-5.0))
		assert.Equal(t, float32(0.0), abs(0.0))
	})

	t.Run("max", func(t *testing.T) {
		assert.Equal(t, int64(10), max(5, 10))
		assert.Equal(t, int64(10), max(10, 5))
		assert.Equal(t, int64(5), max(5, 5))
	})

	t.Run("min", func(t *testing.T) {
		assert.Equal(t, int64(5), min(5, 10))
		assert.Equal(t, int64(5), min(10, 5))
		assert.Equal(t, int64(5), min(5, 5))
	})
}
