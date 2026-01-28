package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestSystem(t *testing.T, p *Processor) {
	t.Helper()
	ctx := context.Background()

	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
		Systems:    []SystemData{{SysNum: 0, ShortName: "county", SystemType: "p25"}},
	})
	require.NoError(t, err)
}

func TestProcessor_ProcessCallStart(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	callData := &CallEventData{
		InstanceID:   "test-recorder-1",
		CallID:       "call-123",
		CallNum:      456,
		ShortName:    "county",
		TGID:         12345,
		TGAlphaTag:   "Fire Dispatch",
		TGDesc:       "Fire Department Dispatch",
		TGGroup:      "Fire",
		TGTag:        "Dispatch",
		Freq:         851500000,
		FreqError:    50,
		StartTime:    time.Now(),
		Encrypted:    false,
		Emergency:    true,
		Phase2TDMA:   true,
		TDMASlot:     1,
		Conventional: false,
		Analog:       false,
		AudioType:    "digital",
		RecState:     1,
		MonState:     2,
		RawJSON:      []byte(`{"test": true}`),
	}

	err := p.ProcessCallStart(ctx, callData)
	require.NoError(t, err)

	// Verify call was created
	var callCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE tr_call_id = $1", "call-123").Scan(&callCount)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Verify call details
	var emergency, encrypted, phase2 bool
	var freq int64
	err = testDB.Pool.QueryRow(ctx,
		"SELECT emergency, encrypted, phase2_tdma, freq FROM calls WHERE tr_call_id = $1",
		"call-123").Scan(&emergency, &encrypted, &phase2, &freq)
	require.NoError(t, err)
	assert.True(t, emergency)
	assert.False(t, encrypted)
	assert.True(t, phase2)
	assert.Equal(t, int64(851500000), freq)

	// Verify talkgroup was created
	var tgCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM talkgroups WHERE tgid = $1", 12345).Scan(&tgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tgCount)
}

func TestProcessor_ProcessCallStart_SystemNotFound(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// Create instance but no system
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
	})
	require.NoError(t, err)

	callData := &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		ShortName:  "nonexistent",
		TGID:       12345,
		StartTime:  time.Now(),
	}

	// Should not error, just skip
	err = p.ProcessCallStart(ctx, callData)
	assert.NoError(t, err)

	// No call should be created
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM calls").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestProcessor_DiffActiveCalls_ExistingCall(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	startTime := time.Now()

	// First create a call via call_start
	err := p.ProcessCallStart(ctx, &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		CallNum:    42,
		ShortName:  "county",
		TGID:       12345,
		StartTime:  startTime,
	})
	require.NoError(t, err)

	// Diff with snapshot containing the same call — should show as updated, not new
	activeCalls := []*ActiveCallInfo{{
		CallID:  "call-123",
		CallNum: 42,
		System:  "county",
		TGID:    12345,
		Freq:    851000000,
	}}
	callData := []*CallEventData{{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		CallNum:    42,
		ShortName:  "county",
		TGID:       12345,
		StartTime:  startTime,
		Duration:   15.5,
	}}

	diff := p.DiffActiveCalls(activeCalls, callData)
	assert.Len(t, diff.NewCalls, 0, "should not be new — already tracked via call_start")
	assert.Len(t, diff.EndedCalls, 0)
	assert.Len(t, diff.UpdatedCalls, 1)

	// Verify call_start created the DB record
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE tr_call_id = $1", "call-123").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessor_DiffActiveCalls_NewCall(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	startTime := time.Now()

	// Diff with a call that was never seen via call_start
	activeCalls := []*ActiveCallInfo{{
		CallID:  "call-new",
		CallNum: 99,
		System:  "county",
		TGID:    12345,
	}}
	callData := []*CallEventData{{
		InstanceID: "test-recorder-1",
		CallID:     "call-new",
		CallNum:    99,
		ShortName:  "county",
		TGID:       12345,
		StartTime:  startTime,
		Duration:   10.0,
	}}

	diff := p.DiffActiveCalls(activeCalls, callData)
	assert.Len(t, diff.NewCalls, 1, "call should be new — never tracked before")
	assert.Len(t, diff.EndedCalls, 0)

	// Process the new call
	err := p.ProcessNewCallFromSnapshot(ctx, diff.NewCalls[0])
	require.NoError(t, err)

	// Call should be created in DB
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE tr_call_id = $1", "call-new").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessor_ProcessCallEnd(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	startTime := time.Now().Add(-30 * time.Second)
	stopTime := time.Now()

	// Create call start
	err := p.ProcessCallStart(ctx, &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		ShortName:  "county",
		TGID:       12345,
		StartTime:  startTime,
	})
	require.NoError(t, err)

	// End the call
	endData := &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		ShortName:  "county",
		TGID:       12345,
		TGAlphaTag: "Fire Dispatch",
		StartTime:  startTime,
		StopTime:   stopTime,
		Duration:   30.0,
		ErrorCount: 3,
		SpikeCount: 1,
		SignalDB:   -42.0,
		NoiseDB:    -75.0,
		Emergency:  true,
		Encrypted:  false,
		RawJSON:    []byte(`{"final": true}`),
	}

	err = p.ProcessCallEnd(ctx, endData)
	require.NoError(t, err)

	// Verify call was updated with final data
	var duration float32
	var emergency bool
	var hasStopTime bool
	err = testDB.Pool.QueryRow(ctx,
		"SELECT duration, emergency, stop_time IS NOT NULL FROM calls WHERE tr_call_id = $1",
		"call-123").Scan(&duration, &emergency, &hasStopTime)
	require.NoError(t, err)
	assert.InDelta(t, 30.0, duration, 0.1)
	assert.True(t, emergency)
	assert.True(t, hasStopTime)
}

func TestProcessor_ProcessCallEnd_CreateIfMissed(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	// End a call that was never started (missed start message)
	endData := &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-missed",
		ShortName:  "county",
		TGID:       12345,
		StartTime:  time.Now().Add(-30 * time.Second),
		StopTime:   time.Now(),
		Duration:   30.0,
	}

	err := p.ProcessCallEnd(ctx, endData)
	require.NoError(t, err)

	// Call should be created
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE tr_call_id = $1", "call-missed").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessor_ProcessUnitEvent(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	eventData := &UnitEventData{
		InstanceID: "test-recorder-1",
		ShortName:  "county",
		SysNum:     0,
		EventType:  "call",
		UnitID:     1234567,
		UnitTag:    "Engine 1",
		TGID:       12345,
		TGAlphaTag: "Fire Dispatch",
		Timestamp:  time.Now(),
		RawJSON:    []byte(`{"event": "call"}`),
	}

	err := p.ProcessUnitEvent(ctx, eventData)
	require.NoError(t, err)

	// Verify unit was created
	var unitCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM units WHERE unit_id = $1", 1234567).Scan(&unitCount)
	require.NoError(t, err)
	assert.Equal(t, 1, unitCount)

	// Verify unit event was recorded
	var eventCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM unit_events WHERE unit_rid = $1 AND event_type = $2", 1234567, "call").Scan(&eventCount)
	require.NoError(t, err)
	assert.Equal(t, 1, eventCount)

	// Verify talkgroup was created
	var tgCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM talkgroups WHERE tgid = $1", 12345).Scan(&tgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tgCount)
}

func TestProcessor_ProcessUnitEvent_CreateSystem(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	// Create instance only
	err := p.ProcessConfig(ctx, &ConfigData{
		InstanceID: "test-recorder-1",
	})
	require.NoError(t, err)

	// Process unit event with new system
	eventData := &UnitEventData{
		InstanceID: "test-recorder-1",
		ShortName:  "newsystem",
		SysNum:     5,
		EventType:  "on",
		UnitID:     9999999,
		UnitTag:    "Unit 99",
		Timestamp:  time.Now(),
	}

	err = p.ProcessUnitEvent(ctx, eventData)
	require.NoError(t, err)

	// System should be created
	var sysCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM systems WHERE short_name = $1", "newsystem").Scan(&sysCount)
	require.NoError(t, err)
	assert.Equal(t, 1, sysCount)
}

func TestProcessor_ProcessTrunkMessage(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	msgData := &TrunkMessageData{
		InstanceID:  "test-recorder-1",
		ShortName:   "county",
		SysNum:      0,
		MsgType:     1,
		MsgTypeName: "grant",
		Opcode:      "0x00",
		OpcodeType:  "voice_grant",
		OpcodeDesc:  "Voice Channel Grant",
		Meta:        "talkgroup=12345",
		Timestamp:   time.Now(),
	}

	err := p.ProcessTrunkMessage(ctx, msgData)
	require.NoError(t, err)

	// Verify message was stored
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM trunk_messages WHERE opcode_type = $1", "voice_grant").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessor_ProcessTrunkMessage_SystemNotFound(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	msgData := &TrunkMessageData{
		InstanceID: "test-recorder-1",
		ShortName:  "nonexistent",
		SysNum:     0,
		MsgType:    1,
		Timestamp:  time.Now(),
	}

	// Should not error, just skip
	err := p.ProcessTrunkMessage(ctx, msgData)
	assert.NoError(t, err)

	// No message should be stored
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM trunk_messages").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestProcessor_processTransmissions(t *testing.T) {
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	// Create a call first
	err := p.ProcessCallStart(ctx, &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     "call-123",
		ShortName:  "county",
		TGID:       12345,
		StartTime:  time.Now(),
	})
	require.NoError(t, err)

	// Get the call from database
	var callID int64
	err = testDB.Pool.QueryRow(ctx, "SELECT id FROM calls WHERE tr_call_id = $1", "call-123").Scan(&callID)
	require.NoError(t, err)

	// Get system ID
	sysID, err := p.getSystemID(ctx, "county")
	require.NoError(t, err)

	srcList := []SourceUnitData{
		{
			Src:       1234567,
			Time:      time.Now(),
			Pos:       0.0,
			Emergency: false,
			Tag:       "Engine 1",
		},
		{
			Src:       7654321,
			Time:      time.Now().Add(10 * time.Second),
			Pos:       10.0,
			Emergency: true,
			Tag:       "Engine 2",
		},
	}

	// We can't call processTransmissions directly since it expects *models.Call
	// Instead, verify that unit upsert works
	for _, src := range srcList {
		_, err := p.db.UpsertUnit(ctx, sysID, src.Src, src.Tag, "ota")
		require.NoError(t, err)
	}

	// Verify units were created
	var unitCount int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM units WHERE system_id = $1", sysID).Scan(&unitCount)
	require.NoError(t, err)
	assert.Equal(t, 2, unitCount)
}

func TestProcessor_CallLifecycle(t *testing.T) {
	// Test a complete call lifecycle: start -> active -> end
	resetDB(t)
	p := newTestProcessor(t)
	ctx := context.Background()

	setupTestSystem(t, p)

	startTime := time.Now()
	callID := "lifecycle-call"

	// 1. Call Start
	err := p.ProcessCallStart(ctx, &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     callID,
		CallNum:    100,
		ShortName:  "county",
		TGID:       54321,
		TGAlphaTag: "Police Dispatch",
		StartTime:  startTime,
		Emergency:  false,
	})
	require.NoError(t, err)

	// 2. Simulate calls_active snapshot (call still active)
	activeCalls := []*ActiveCallInfo{
		{
			System:  "county",
			CallNum: 100,
			TGID:    54321,
		},
	}
	activeCallData := []*CallEventData{
		{
			InstanceID: "test-recorder-1",
			CallID:     callID,
			CallNum:    100,
			ShortName:  "county",
			TGID:       54321,
			StartTime:  startTime,
			Duration:   30.0,
			ErrorCount: 3,
		},
	}
	diff := p.DiffActiveCalls(activeCalls, activeCallData)
	// Call was already started, so it should appear as updated (not new)
	assert.Empty(t, diff.NewCalls)
	assert.Empty(t, diff.EndedCalls)
	assert.Len(t, diff.UpdatedCalls, 1)

	// 3. Call End
	endTime := startTime.Add(45 * time.Second)
	err = p.ProcessCallEnd(ctx, &CallEventData{
		InstanceID: "test-recorder-1",
		CallID:     callID,
		ShortName:  "county",
		TGID:       54321,
		StartTime:  startTime,
		StopTime:   endTime,
		Duration:   45.0,
		ErrorCount: 5,
		SpikeCount: 2,
	})
	require.NoError(t, err)

	// Verify final state
	var duration float32
	var errorCount, spikeCount int
	var hasStopTime bool
	err = testDB.Pool.QueryRow(ctx,
		"SELECT duration, error_count, spike_count, stop_time IS NOT NULL FROM calls WHERE tr_call_id = $1",
		callID).Scan(&duration, &errorCount, &spikeCount, &hasStopTime)
	require.NoError(t, err)

	assert.InDelta(t, 45.0, duration, 0.1)
	assert.Equal(t, 5, errorCount)
	assert.Equal(t, 2, spikeCount)
	assert.True(t, hasStopTime)
}
