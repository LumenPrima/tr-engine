package ingest

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
	"go.uber.org/zap"
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

func resetDB(t *testing.T) {
	t.Helper()
	testDB.Reset(t)
}

func newTestProcessor(t *testing.T) *Processor {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	return NewProcessor(testDB.DB, nil, nil, logger)
}

func TestNewProcessor(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	p := NewProcessor(testDB.DB, nil, nil, logger)

	assert.NotNil(t, p)
	assert.NotNil(t, p.instances)
	assert.NotNil(t, p.systems)
	assert.Equal(t, testDB.DB, p.db)
}

func TestProcessor_ProcessConfig(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	configData := &ConfigData{
		InstanceID:  "test-recorder-1",
		InstanceKey: "key123",
		Sources: []SourceData{
			{
				SourceNum:  0,
				CenterFreq: 851000000,
				Rate:       2048000,
				Driver:     "osmosdr",
				Device:     "rtl=0",
				Antenna:    "",
				Gain:       40,
			},
		},
		Systems: []SystemData{
			{
				SysNum:     0,
				ShortName:  "county",
				SystemType: "p25",
			},
		},
		ConfigJSON: json.RawMessage(`{"test": true}`),
	}

	err := p.ProcessConfig(ctx, configData)
	require.NoError(t, err)

	// Verify instance was cached
	p.instanceLock.RLock()
	instanceID, ok := p.instances["test-recorder-1"]
	p.instanceLock.RUnlock()
	assert.True(t, ok)
	assert.Greater(t, instanceID, 0)

	// Verify system was cached
	p.systemLock.RLock()
	cachedSystem, ok := p.systems["county"]
	p.systemLock.RUnlock()
	assert.True(t, ok)
	assert.NotNil(t, cachedSystem)
	assert.Greater(t, cachedSystem.ID, 0)

	// Verify data in database
	var instanceCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM instances WHERE instance_id = $1", "test-recorder-1").Scan(&instanceCount)
	require.NoError(t, err)
	assert.Equal(t, 1, instanceCount)

	var systemCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM systems WHERE short_name = $1", "county").Scan(&systemCount)
	require.NoError(t, err)
	assert.Equal(t, 1, systemCount)

	var sourceCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE source_num = $1", 0).Scan(&sourceCount)
	require.NoError(t, err)
	assert.Equal(t, 1, sourceCount)
}

func TestProcessor_ProcessConfig_Update(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// First config
	configData := &ConfigData{
		InstanceID:  "test-recorder-1",
		InstanceKey: "key123",
		Systems: []SystemData{
			{SysNum: 0, ShortName: "county", SystemType: "p25"},
		},
	}
	err := p.ProcessConfig(ctx, configData)
	require.NoError(t, err)

	// Update config with new instance key
	configData.InstanceKey = "key456"
	err = p.ProcessConfig(ctx, configData)
	require.NoError(t, err)

	// Should still have only one instance
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM instances").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Instance key should be updated
	var key string
	err = testDB.Pool.QueryRow(ctx, "SELECT instance_key FROM instances WHERE instance_id = $1", "test-recorder-1").Scan(&key)
	require.NoError(t, err)
	assert.Equal(t, "key456", key)
}

func TestProcessor_ProcessSystemStatus(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// First create an instance via config
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
	})
	require.NoError(t, err)

	statusData := &SystemStatusData{
		InstanceID: "test-recorder-1",
		SysNum:     0,
		ShortName:  "county",
		SystemType: "p25",
		SysID:      "1234",
		WACN:       "ABCD",
		NAC:        "5678",
		RFSS:       1,
		SiteID:     10,
		Timestamp:  time.Now(),
	}

	err = p.ProcessSystemStatus(ctx, statusData)
	require.NoError(t, err)

	// Verify system was cached
	p.systemLock.RLock()
	_, ok := p.systems["county"]
	p.systemLock.RUnlock()
	assert.True(t, ok)

	// Verify in database
	var sysID, wacn string
	err = testDB.Pool.QueryRow(ctx, "SELECT sysid, wacn FROM systems WHERE short_name = $1", "county").Scan(&sysID, &wacn)
	require.NoError(t, err)
	assert.Equal(t, "1234", sysID)
	assert.Equal(t, "ABCD", wacn)
}

func TestProcessor_ProcessRate(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// First set up instance and system
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
		Systems:    []SystemData{{SysNum: 0, ShortName: "county", SystemType: "p25"}},
	})
	require.NoError(t, err)

	rateData := &RateData{
		InstanceID:     "test-recorder-1",
		SysNum:         0,
		ShortName:      "county",
		DecodeRate:     95.5,
		ControlChannel: 851012500,
		Timestamp:      time.Now(),
	}

	err = p.ProcessRate(ctx, rateData)
	require.NoError(t, err)

	// Verify rate was stored
	var rate float32
	var channel int64
	err = testDB.Pool.QueryRow(ctx, "SELECT decode_rate, control_channel FROM system_rates ORDER BY id DESC LIMIT 1").Scan(&rate, &channel)
	require.NoError(t, err)
	assert.InDelta(t, 95.5, rate, 0.1)
	assert.Equal(t, int64(851012500), channel)
}

func TestProcessor_ProcessRate_SystemNotFound(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// Try to process rate for non-existent system
	rateData := &RateData{
		InstanceID:     "test-recorder-1",
		SysNum:         0,
		ShortName:      "nonexistent",
		DecodeRate:     95.5,
		ControlChannel: 851012500,
		Timestamp:      time.Now(),
	}

	// Should not error, just skip
	err := p.ProcessRate(ctx, rateData)
	assert.NoError(t, err)

	// No rate should be stored
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM system_rates").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestProcessor_ProcessRecorderStatus(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// First set up instance
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
	})
	require.NoError(t, err)

	recorderData := &RecorderData{
		InstanceID: "test-recorder-1",
		RecNum:     0,
		RecType:    "digital",
		SourceNum:  0,
		State:      1,
		Freq:       851500000,
		Duration:   12.5,
		Squelched:  false,
		Timestamp:  time.Now(),
	}

	err = p.ProcessRecorderStatus(ctx, recorderData)
	require.NoError(t, err)

	// Verify recorder was created
	var recCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM recorders WHERE rec_num = 0").Scan(&recCount)
	require.NoError(t, err)
	assert.Equal(t, 1, recCount)

	// Verify status was stored
	var state int16
	var freq int64
	err = testDB.Pool.QueryRow(ctx, "SELECT state, freq FROM recorder_status ORDER BY id DESC LIMIT 1").Scan(&state, &freq)
	require.NoError(t, err)
	assert.Equal(t, int16(1), state)
	assert.Equal(t, int64(851500000), freq)
}

func TestProcessor_getOrCreateInstance(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// First call should create instance
	id1, err := p.getOrCreateInstance(ctx, "test-instance")
	require.NoError(t, err)
	assert.Greater(t, id1, 0)

	// Second call should return cached value
	id2, err := p.getOrCreateInstance(ctx, "test-instance")
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	// Verify only one instance in database
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM instances").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessor_getSystemID(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// Create instance and system
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
		Systems:    []SystemData{{SysNum: 0, ShortName: "county", SystemType: "p25"}},
	})
	require.NoError(t, err)

	// Get system ID (should be cached from ProcessConfig)
	id, err := p.getSystemID(ctx, "county")
	require.NoError(t, err)
	assert.Greater(t, id, 0)

	// Clear cache to test database lookup
	p.systemLock.Lock()
	delete(p.systems, "county")
	p.systemLock.Unlock()

	// Should still find it in database
	id2, err := p.getSystemID(ctx, "county")
	require.NoError(t, err)
	assert.Equal(t, id, id2)

	// Nonexistent system should return 0
	id3, err := p.getSystemID(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, 0, id3)
}

func TestProcessor_CacheConcurrency(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// Create initial instance
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "concurrent-test",
	})
	require.NoError(t, err)

	// Concurrent reads from cache
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, err := p.getOrCreateInstance(ctx, "concurrent-test")
				assert.NoError(t, err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestProcessor_GetDB(t *testing.T) {
	p := newTestProcessor(t)
	assert.Equal(t, testDB.DB, p.GetDB())
}
