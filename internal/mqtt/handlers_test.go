package mqtt

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"go.uber.org/zap"
)

// mockMessage implements mqtt.Message for testing
type mockMessage struct {
	topic   string
	payload []byte
}

func (m *mockMessage) Duplicate() bool   { return false }
func (m *mockMessage) Qos() byte         { return 0 }
func (m *mockMessage) Retained() bool    { return false }
func (m *mockMessage) Topic() string     { return m.topic }
func (m *mockMessage) MessageID() uint16 { return 0 }
func (m *mockMessage) Payload() []byte   { return m.payload }
func (m *mockMessage) Ack()              {}

// mockProcessor tracks calls to processor methods for verification
type mockProcessor struct {
	mu sync.Mutex

	configCalls       []*ingest.ConfigData
	systemStatusCalls []*ingest.SystemStatusData
	rateCalls         []*ingest.RateData
	recorderCalls     []*ingest.RecorderData
	callStartCalls    []*ingest.CallEventData
	callActiveCalls   []*ingest.CallEventData
	callEndCalls      []*ingest.CallEventData
	audioCalls        []*ingest.AudioData
	unitEventCalls    []*ingest.UnitEventData
	trunkMsgCalls     []*ingest.TrunkMessageData

	// Error injection
	configErr       error
	systemStatusErr error
	rateErr         error
	recorderErr     error
	callStartErr    error
	callActiveErr   error
	callEndErr      error
	audioErr        error
	unitEventErr    error
	trunkMsgErr     error
}

func newMockProcessor() *mockProcessor {
	return &mockProcessor{}
}

func (m *mockProcessor) ProcessConfig(ctx context.Context, data *ingest.ConfigData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configCalls = append(m.configCalls, data)
	return m.configErr
}

func (m *mockProcessor) ProcessSystemStatus(ctx context.Context, data *ingest.SystemStatusData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemStatusCalls = append(m.systemStatusCalls, data)
	return m.systemStatusErr
}

func (m *mockProcessor) ProcessRate(ctx context.Context, data *ingest.RateData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateCalls = append(m.rateCalls, data)
	return m.rateErr
}

func (m *mockProcessor) ProcessRecorderStatus(ctx context.Context, data *ingest.RecorderData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorderCalls = append(m.recorderCalls, data)
	return m.recorderErr
}

func (m *mockProcessor) ProcessCallStart(ctx context.Context, data *ingest.CallEventData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callStartCalls = append(m.callStartCalls, data)
	return m.callStartErr
}

func (m *mockProcessor) ProcessCallActive(ctx context.Context, data *ingest.CallEventData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callActiveCalls = append(m.callActiveCalls, data)
	return m.callActiveErr
}

func (m *mockProcessor) ProcessCallEnd(ctx context.Context, data *ingest.CallEventData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callEndCalls = append(m.callEndCalls, data)
	return m.callEndErr
}

func (m *mockProcessor) ProcessAudio(ctx context.Context, data *ingest.AudioData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioCalls = append(m.audioCalls, data)
	return m.audioErr
}

func (m *mockProcessor) ProcessUnitEvent(ctx context.Context, data *ingest.UnitEventData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unitEventCalls = append(m.unitEventCalls, data)
	return m.unitEventErr
}

func (m *mockProcessor) ProcessTrunkMessage(ctx context.Context, data *ingest.TrunkMessageData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trunkMsgCalls = append(m.trunkMsgCalls, data)
	return m.trunkMsgErr
}

// processorAdapter wraps mockProcessor to implement the interface expected by Handlers
type processorAdapter struct {
	mock *mockProcessor
}

func (p *processorAdapter) ProcessConfig(ctx context.Context, data *ingest.ConfigData) error {
	return p.mock.ProcessConfig(ctx, data)
}

func (p *processorAdapter) ProcessSystemStatus(ctx context.Context, data *ingest.SystemStatusData) error {
	return p.mock.ProcessSystemStatus(ctx, data)
}

func (p *processorAdapter) ProcessRate(ctx context.Context, data *ingest.RateData) error {
	return p.mock.ProcessRate(ctx, data)
}

func (p *processorAdapter) ProcessRecorderStatus(ctx context.Context, data *ingest.RecorderData) error {
	return p.mock.ProcessRecorderStatus(ctx, data)
}

func (p *processorAdapter) ProcessCallStart(ctx context.Context, data *ingest.CallEventData) error {
	return p.mock.ProcessCallStart(ctx, data)
}

func (p *processorAdapter) ProcessCallActive(ctx context.Context, data *ingest.CallEventData) error {
	return p.mock.ProcessCallActive(ctx, data)
}

func (p *processorAdapter) ProcessCallEnd(ctx context.Context, data *ingest.CallEventData) error {
	return p.mock.ProcessCallEnd(ctx, data)
}

func (p *processorAdapter) ProcessAudio(ctx context.Context, data *ingest.AudioData) error {
	return p.mock.ProcessAudio(ctx, data)
}

func (p *processorAdapter) ProcessUnitEvent(ctx context.Context, data *ingest.UnitEventData) error {
	return p.mock.ProcessUnitEvent(ctx, data)
}

func (p *processorAdapter) ProcessTrunkMessage(ctx context.Context, data *ingest.TrunkMessageData) error {
	return p.mock.ProcessTrunkMessage(ctx, data)
}

func newTestHandlers(mock *mockProcessor) *Handlers {
	logger, _ := zap.NewDevelopment()
	// We need to use the real processor type, so we'll test with integration approach
	// For now, create handlers with nil processor and test message parsing
	return &Handlers{
		processor: nil,
		logger:    logger,
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected time.Time
	}{
		{
			name:     "Unix seconds",
			input:    1704067200,
			expected: time.Unix(1704067200, 0),
		},
		{
			name:     "Unix milliseconds",
			input:    1704067200000,
			expected: time.UnixMilli(1704067200000),
		},
		{
			name:     "Zero",
			input:    0,
			expected: time.Unix(0, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTimestamp(tt.input)
			assert.Equal(t, tt.expected.Unix(), result.Unix())
		})
	}
}

func TestStatusMessageParsing(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		expectType  string
		expectError bool
	}{
		{
			name:       "Config message",
			payload:    `{"type": "config", "instance_id": "test-1", "timestamp": 1704067200}`,
			expectType: "config",
		},
		{
			name:       "Systems message",
			payload:    `{"type": "systems", "instance_id": "test-1", "timestamp": 1704067200}`,
			expectType: "systems",
		},
		{
			name:       "Call start message",
			payload:    `{"type": "call_start", "instance_id": "test-1", "timestamp": 1704067200}`,
			expectType: "call_start",
		},
		{
			name:       "Call end message",
			payload:    `{"type": "call_end", "instance_id": "test-1", "timestamp": 1704067200}`,
			expectType: "call_end",
		},
		{
			name:       "Audio message",
			payload:    `{"type": "audio", "instance_id": "test-1", "timestamp": 1704067200}`,
			expectType: "audio",
		},
		{
			name:        "Invalid JSON",
			payload:     `{"type": "config", invalid}`,
			expectError: true,
		},
		{
			name:        "Empty payload",
			payload:     ``,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var base StatusMessage
			err := json.Unmarshal([]byte(tt.payload), &base)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectType, base.Type)
			}
		})
	}
}

func TestConfigMessageParsing(t *testing.T) {
	payload := `{
		"type": "config",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"config": {
			"instance_id": "recorder-1",
			"instance_key": "key123",
			"capture_dir": "/tmp/capture",
			"sources": [
				{
					"source_num": 0,
					"center": 851000000,
					"rate": 2048000,
					"driver": "osmosdr",
					"device": "rtl=0",
					"antenna": "",
					"gain": 40
				}
			],
			"systems": [
				{
					"sys_num": 0,
					"sys_name": "county",
					"system_type": "p25"
				}
			]
		}
	}`

	var msg ConfigMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "config", msg.Type)
	assert.Equal(t, "recorder-1", msg.InstanceID)
	assert.Equal(t, int64(1704067200), msg.Timestamp)
	assert.Equal(t, "key123", msg.Config.InstanceKey)
	assert.Len(t, msg.Config.Sources, 1)
	assert.Len(t, msg.Config.Systems, 1)
	assert.Equal(t, 0, msg.Config.Sources[0].SourceNum)
	assert.Equal(t, float64(851000000), msg.Config.Sources[0].CenterFreq)
	assert.Equal(t, "county", msg.Config.Systems[0].ShortName)
}

func TestSystemsMessageParsing(t *testing.T) {
	payload := `{
		"type": "systems",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"systems": [
			{
				"sys_num": 0,
				"sys_name": "county",
				"type": "p25",
				"sysid": "1234",
				"wacn": "ABCD",
				"nac": "5678",
				"rfss": 1,
				"site_id": 10
			}
		]
	}`

	var msg SystemsMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "systems", msg.Type)
	assert.Len(t, msg.Systems, 1)
	assert.Equal(t, "county", msg.Systems[0].ShortName)
	assert.Equal(t, "p25", msg.Systems[0].Type)
	assert.Equal(t, "1234", msg.Systems[0].SysID)
	assert.Equal(t, 1, msg.Systems[0].RFSS)
	assert.Equal(t, 10, msg.Systems[0].SiteID)
}

func TestRatesMessageParsing(t *testing.T) {
	payload := `{
		"type": "rates",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"rates": [
			{
				"sys_num": 0,
				"sys_name": "county",
				"decoderate": 95.5,
				"decoderate_interval": 98.2,
				"control_channel": 851012500
			}
		]
	}`

	var msg RatesMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "rates", msg.Type)
	assert.Len(t, msg.Rates, 1)
	assert.Equal(t, "county", msg.Rates[0].ShortName)
	assert.InDelta(t, 95.5, msg.Rates[0].DecodeRate, 0.01)
	assert.Equal(t, float64(851012500), msg.Rates[0].ControlChannel)
}

func TestRecordersMessageParsing(t *testing.T) {
	payload := `{
		"type": "recorders",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"recorders": [
			{
				"id": "rec-0",
				"type": "digital",
				"src_num": 0,
				"rec_num": 0,
				"rec_state": 1,
				"rec_state_type": "recording",
				"freq": 851500000,
				"count": 5,
				"duration": 12.5,
				"squelched": false
			}
		]
	}`

	var msg RecordersMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "recorders", msg.Type)
	assert.Len(t, msg.Recorders, 1)
	assert.Equal(t, "rec-0", msg.Recorders[0].ID)
	assert.Equal(t, "digital", msg.Recorders[0].Type)
	assert.Equal(t, 1, msg.Recorders[0].State)
	assert.Equal(t, float64(851500000), msg.Recorders[0].Freq)
	assert.InDelta(t, 12.5, msg.Recorders[0].Duration, 0.01)
	assert.False(t, msg.Recorders[0].Squelched)
}

func TestCallMessageParsing(t *testing.T) {
	payload := `{
		"type": "call_start",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"call": {
			"id": "call-123",
			"call_num": 456,
			"sys_num": 0,
			"sys_name": "county",
			"freq": 851500000,
			"freq_error": 50,
			"talkgroup": 12345,
			"talkgroup_alpha_tag": "Fire Dispatch",
			"talkgroup_description": "Fire Department Dispatch",
			"talkgroup_group": "Fire",
			"talkgroup_tag": "Dispatch",
			"start_time": 1704067100,
			"stop_time": 1704067200,
			"length": 100.5,
			"encrypted": false,
			"emergency": true,
			"phase2_tdma": true,
			"tdma_slot": 1,
			"analog": false,
			"conventional": false,
			"error_count": 2,
			"spike_count": 1,
			"signal": -45.5,
			"noise": -80.2
		}
	}`

	var msg CallMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "call_start", msg.Type)
	assert.Equal(t, "call-123", msg.Call.ID)
	assert.Equal(t, int64(456), msg.Call.CallNum)
	assert.Equal(t, "county", msg.Call.ShortName)
	assert.Equal(t, 12345, msg.Call.TGID)
	assert.Equal(t, "Fire Dispatch", msg.Call.TGAlphaTag)
	assert.True(t, msg.Call.Emergency)
	assert.True(t, msg.Call.Phase2TDMA)
	assert.Equal(t, 1, msg.Call.TDMASlot)
	assert.InDelta(t, 100.5, msg.Call.Length, 0.01)
	assert.Equal(t, 2, msg.Call.ErrorCount)
	assert.InDelta(t, -45.5, msg.Call.SignalDB, 0.01)
}

func TestAudioMessageParsing(t *testing.T) {
	payload := `{
		"type": "audio",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"call": {
			"audio_wav_base64": "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAAZGF0YQAAAAA=",
			"audio_m4a_base64": "",
			"metadata": {
				"freq": 851500000,
				"start_time": 1704067100,
				"stop_time": 1704067200,
				"talkgroup": 12345,
				"short_name": "county",
				"filename": "12345-1704067100.wav",
				"freqList": [
					{"freq": 851500000, "time": 1704067100, "pos": 0.0, "len": 100.0, "error_count": 0, "spike_count": 0}
				],
				"srcList": [
					{"src": 1234567, "time": 1704067100, "pos": 0.0, "emergency": 0, "tag": "Unit 1"}
				]
			}
		}
	}`

	var msg AudioMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "audio", msg.Type)
	assert.NotEmpty(t, msg.Call.AudioWavB64)
	assert.Equal(t, int64(851500000), msg.Call.Metadata.Freq)
	assert.Equal(t, 12345, msg.Call.Metadata.TGID)
	assert.Equal(t, "county", msg.Call.Metadata.ShortName)
	assert.Equal(t, "12345-1704067100.wav", msg.Call.Metadata.Filename)
	assert.Len(t, msg.Call.Metadata.FreqList, 1)
	assert.Len(t, msg.Call.Metadata.SrcList, 1)
	assert.Equal(t, int64(1234567), msg.Call.Metadata.SrcList[0].Src)
	assert.Equal(t, "Unit 1", msg.Call.Metadata.SrcList[0].Tag)
}

func TestUnitMessageParsing(t *testing.T) {
	t.Run("unit call message", func(t *testing.T) {
		payload := `{
			"type": "call",
			"instance_id": "recorder-1",
			"timestamp": 1704067200,
			"call": {
				"sys_num": 0,
				"sys_name": "county",
				"unit": 1234567,
				"unit_alpha_tag": "Engine 1",
				"talkgroup": 12345,
				"talkgroup_alpha_tag": "Fire Dispatch",
				"call_num": 456,
				"freq": 851500000,
				"start_time": 1704067100
			}
		}`

		var msg UnitCallMessage
		err := json.Unmarshal([]byte(payload), &msg)
		require.NoError(t, err)

		assert.Equal(t, "call", msg.Type)
		assert.Equal(t, int64(1234567), msg.Call.Unit)
		assert.Equal(t, "Engine 1", msg.Call.UnitAlphaTag)
		assert.Equal(t, 12345, msg.Call.TGID)
	})

	t.Run("unit end message", func(t *testing.T) {
		payload := `{
			"type": "end",
			"instance_id": "recorder-1",
			"timestamp": 1704067200,
			"end": {
				"sys_num": 0,
				"sys_name": "county",
				"unit": 1234567,
				"unit_alpha_tag": "Engine 1",
				"talkgroup": 12345,
				"position": 5.5,
				"length": 10.2,
				"start_time": 1704067100,
				"stop_time": 1704067200,
				"error_count": 1,
				"spike_count": 0
			}
		}`

		var msg UnitEndMessage
		err := json.Unmarshal([]byte(payload), &msg)
		require.NoError(t, err)

		assert.Equal(t, "end", msg.Type)
		assert.Equal(t, int64(1234567), msg.End.Unit)
		assert.InDelta(t, 5.5, msg.End.Position, 0.01)
		assert.InDelta(t, 10.2, msg.End.Length, 0.01)
	})

	t.Run("generic unit message", func(t *testing.T) {
		payload := `{
			"type": "on",
			"instance_id": "recorder-1",
			"timestamp": 1704067200,
			"sys_num": 0,
			"short_name": "county",
			"unit": 1234567,
			"unit_alpha_tag": "Engine 1",
			"talkgroup": 12345
		}`

		var msg UnitMessage
		err := json.Unmarshal([]byte(payload), &msg)
		require.NoError(t, err)

		assert.Equal(t, "on", msg.Type)
		assert.Equal(t, int64(1234567), msg.Unit)
		assert.Equal(t, "county", msg.ShortName)
	})
}

func TestTrunkingMessageParsing(t *testing.T) {
	payload := `{
		"type": "trunk",
		"instance_id": "recorder-1",
		"timestamp": 1704067200,
		"sys_num": 0,
		"short_name": "county",
		"msg_type": 1,
		"msg_type_name": "grant",
		"opcode": "0x00",
		"opcode_type": "voice_grant",
		"opcode_desc": "Voice Channel Grant",
		"meta": "talkgroup=12345"
	}`

	var msg TrunkingMessage
	err := json.Unmarshal([]byte(payload), &msg)
	require.NoError(t, err)

	assert.Equal(t, "trunk", msg.Type)
	assert.Equal(t, "county", msg.ShortName)
	assert.Equal(t, 1, msg.MsgType)
	assert.Equal(t, "grant", msg.MsgTypeName)
	assert.Equal(t, "0x00", msg.Opcode)
	assert.Equal(t, "voice_grant", msg.OpcodeType)
	assert.Equal(t, "Voice Channel Grant", msg.OpcodeDesc)
	assert.Equal(t, "talkgroup=12345", msg.Meta)
}

func TestHandleStatusMessage_InvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handlers := &Handlers{
		processor: nil,
		logger:    logger,
	}

	msg := &mockMessage{
		topic:   "trunk-recorder/status/test",
		payload: []byte(`{invalid json}`),
	}

	// Should not panic, should log error
	handlers.HandleStatusMessage(nil, msg)
}

func TestHandleUnitMessage_InvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handlers := &Handlers{
		processor: nil,
		logger:    logger,
	}

	msg := &mockMessage{
		topic:   "trunk-recorder/units/test",
		payload: []byte(`{invalid json}`),
	}

	// Should not panic, should log error
	handlers.HandleUnitMessage(nil, msg)
}

func TestHandleTrunkMessage_InvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handlers := &Handlers{
		processor: nil,
		logger:    logger,
	}

	msg := &mockMessage{
		topic:   "trunk-recorder/messages/test",
		payload: []byte(`{invalid json}`),
	}

	// Should not panic, should log error
	handlers.HandleTrunkMessage(nil, msg)
}

func TestHandleUnitMessage_InvalidUnitID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handlers := &Handlers{
		processor: nil,
		logger:    logger,
	}

	// Unit ID of 0 should be skipped
	msg := &mockMessage{
		topic:   "trunk-recorder/units/test",
		payload: []byte(`{"type": "on", "instance_id": "test", "timestamp": 1704067200, "unit": 0}`),
	}

	// Should not panic, should skip processing
	handlers.HandleUnitMessage(nil, msg)
}

func TestCallDataConversion(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handlers := &Handlers{
		processor: nil,
		logger:    logger,
	}

	call := &CallData{
		ID:            "call-123",
		CallNum:       456,
		SysNum:        0,
		ShortName:     "county",
		Freq:          851500000,
		FreqError:     50,
		TGID:          12345,
		TGAlphaTag:    "Fire Dispatch",
		TGDesc:        "Fire Department",
		TGGroup:       "Fire",
		TGTag:         "Dispatch",
		StartTime:     1704067100,
		StopTime:      1704067200,
		Length:        100.5,
		Encrypted:     false,
		Emergency:     true,
		Phase2TDMA:    true,
		TDMASlot:      1,
		Analog:        false,
		Conventional:  false,
		ErrorCount:    2,
		SpikeCount:    1,
		SignalDB:      -45.5,
		NoiseDB:       -80.2,
		RecState:      1,
		MonState:      2,
		RecStateType:  "recording",
		MonStateType:  "monitoring",
		AudioType:     "digital",
	}

	converted := handlers.convertCallData("test-instance", 1704067200, call)

	assert.Equal(t, "test-instance", converted.InstanceID)
	assert.Equal(t, "call-123", converted.CallID)
	assert.Equal(t, int64(456), converted.CallNum)
	assert.Equal(t, "county", converted.ShortName)
	assert.Equal(t, int64(851500000), converted.Freq)
	assert.Equal(t, 12345, converted.TGID)
	assert.Equal(t, "Fire Dispatch", converted.TGAlphaTag)
	assert.True(t, converted.Emergency)
	assert.True(t, converted.Phase2TDMA)
	assert.InDelta(t, 100.5, converted.Duration, 0.01)
	assert.Equal(t, 2, converted.ErrorCount)
	assert.InDelta(t, -45.5, converted.SignalDB, 0.01)
}

// Verify mock implements the necessary interface
var _ mqtt.Message = (*mockMessage)(nil)
