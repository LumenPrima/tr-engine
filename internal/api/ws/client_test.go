package ws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewClient(t *testing.T) {
	hub := NewHub(zap.NewNop())
	logger := zap.NewNop()

	client := NewClient(hub, nil, logger)

	assert.NotNil(t, client)
	assert.Equal(t, hub, client.hub)
	assert.Nil(t, client.conn)
	assert.NotNil(t, client.send)
	assert.Equal(t, logger, client.logger)
	assert.NotNil(t, client.subscriptions)
	assert.NotNil(t, client.subscriptions.Channels)
	assert.NotNil(t, client.subscriptions.Systems)
	assert.NotNil(t, client.subscriptions.Talkgroups)
}

func TestSubscriptions_Subscribe(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	msg := SubscribeMessage{
		Action:     "subscribe",
		Channels:   []string{"calls", "units"},
		Systems:    []string{"county", "city"},
		Talkgroups: []int{12345, 67890},
	}

	client.subscribe(msg)

	assert.True(t, client.subscriptions.Channels["calls"])
	assert.True(t, client.subscriptions.Channels["units"])
	assert.True(t, client.subscriptions.Systems["county"])
	assert.True(t, client.subscriptions.Systems["city"])
	assert.True(t, client.subscriptions.Talkgroups[12345])
	assert.True(t, client.subscriptions.Talkgroups[67890])
}

func TestSubscriptions_Unsubscribe(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	// First subscribe
	client.subscribe(SubscribeMessage{
		Action:     "subscribe",
		Channels:   []string{"calls", "units", "rates"},
		Systems:    []string{"county", "city"},
		Talkgroups: []int{12345, 67890},
	})

	// Then unsubscribe some
	client.unsubscribe(SubscribeMessage{
		Action:     "unsubscribe",
		Channels:   []string{"units"},
		Systems:    []string{"city"},
		Talkgroups: []int{67890},
	})

	// Verify remaining subscriptions
	assert.True(t, client.subscriptions.Channels["calls"])
	assert.False(t, client.subscriptions.Channels["units"])
	assert.True(t, client.subscriptions.Channels["rates"])
	assert.True(t, client.subscriptions.Systems["county"])
	assert.False(t, client.subscriptions.Systems["city"])
	assert.True(t, client.subscriptions.Talkgroups[12345])
	assert.False(t, client.subscriptions.Talkgroups[67890])
}

func TestClient_HandleMessage_Subscribe(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	msg := SubscribeMessage{
		Action:   "subscribe",
		Channels: []string{"calls"},
	}
	data, _ := json.Marshal(msg)

	client.handleMessage(data)

	assert.True(t, client.subscriptions.Channels["calls"])
}

func TestClient_HandleMessage_Unsubscribe(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	// Subscribe first
	client.subscriptions.Channels["calls"] = true

	msg := SubscribeMessage{
		Action:   "unsubscribe",
		Channels: []string{"calls"},
	}
	data, _ := json.Marshal(msg)

	client.handleMessage(data)

	assert.False(t, client.subscriptions.Channels["calls"])
}

func TestClient_HandleMessage_InvalidJSON(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	// Should not panic on invalid JSON
	client.handleMessage([]byte("not valid json"))

	// Subscriptions should remain empty
	assert.Equal(t, 0, len(client.subscriptions.Channels))
}

func TestClient_HandleMessage_UnknownAction(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	msg := SubscribeMessage{
		Action:   "unknown",
		Channels: []string{"calls"},
	}
	data, _ := json.Marshal(msg)

	// Should not panic
	client.handleMessage(data)

	// Should not add subscriptions
	assert.False(t, client.subscriptions.Channels["calls"])
}

func TestClient_IsSubscribed_NoSubscriptions(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	event := Event{
		Type: "call_start",
		Data: map[string]interface{}{},
	}

	assert.False(t, client.IsSubscribed(event))
}

func TestClient_IsSubscribed_ChannelSubscribed(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["calls"] = true

	event := Event{
		Type: "call_start",
		Data: map[string]interface{}{},
	}

	assert.True(t, client.IsSubscribed(event))
}

func TestClient_IsSubscribed_ChannelNotSubscribed(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["units"] = true // Only units, not calls

	event := Event{
		Type: "call_start",
		Data: map[string]interface{}{},
	}

	assert.False(t, client.IsSubscribed(event))
}

func TestClient_IsSubscribed_WildcardChannel(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["*"] = true

	tests := []struct {
		eventType string
		expected  bool
	}{
		{"call_start", true},
		{"call_end", true},
		{"unit_event", true},
		{"rate_update", true},
		{"recorder_update", true},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			event := Event{Type: tt.eventType, Data: map[string]interface{}{}}
			assert.Equal(t, tt.expected, client.IsSubscribed(event))
		})
	}
}

func TestClient_IsSubscribed_SystemFilter(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["calls"] = true
	client.subscriptions.Systems["county"] = true

	tests := []struct {
		name     string
		data     map[string]interface{}
		expected bool
	}{
		{
			name:     "matching system",
			data:     map[string]interface{}{"system": "county"},
			expected: true,
		},
		{
			name:     "non-matching system",
			data:     map[string]interface{}{"system": "city"},
			expected: false,
		},
		{
			name:     "no system in event",
			data:     map[string]interface{}{"talkgroup": 12345},
			expected: true, // No system to filter on
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{Type: "call_start", Data: tt.data}
			assert.Equal(t, tt.expected, client.IsSubscribed(event))
		})
	}
}

func TestClient_IsSubscribed_TalkgroupFilter(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["calls"] = true
	client.subscriptions.Talkgroups[12345] = true

	tests := []struct {
		name     string
		data     map[string]interface{}
		expected bool
	}{
		{
			name:     "matching talkgroup (float64)",
			data:     map[string]interface{}{"talkgroup": float64(12345)},
			expected: true,
		},
		{
			name:     "matching talkgroup (int)",
			data:     map[string]interface{}{"talkgroup": 12345},
			expected: true,
		},
		{
			name:     "non-matching talkgroup",
			data:     map[string]interface{}{"talkgroup": float64(99999)},
			expected: false,
		},
		{
			name:     "no talkgroup in event",
			data:     map[string]interface{}{"system": "county"},
			expected: true, // No talkgroup to filter on
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{Type: "call_start", Data: tt.data}
			assert.Equal(t, tt.expected, client.IsSubscribed(event))
		})
	}
}

func TestClient_IsSubscribed_CombinedFilters(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["calls"] = true
	client.subscriptions.Systems["county"] = true
	client.subscriptions.Talkgroups[12345] = true

	tests := []struct {
		name     string
		data     map[string]interface{}
		expected bool
	}{
		{
			name:     "both match",
			data:     map[string]interface{}{"system": "county", "talkgroup": float64(12345)},
			expected: true,
		},
		{
			name:     "system matches, talkgroup doesn't",
			data:     map[string]interface{}{"system": "county", "talkgroup": float64(99999)},
			expected: false,
		},
		{
			name:     "talkgroup matches, system doesn't",
			data:     map[string]interface{}{"system": "city", "talkgroup": float64(12345)},
			expected: false,
		},
		{
			name:     "neither matches",
			data:     map[string]interface{}{"system": "city", "talkgroup": float64(99999)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{Type: "call_start", Data: tt.data}
			assert.Equal(t, tt.expected, client.IsSubscribed(event))
		})
	}
}

func TestClient_IsSubscribed_NonMapData(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())
	client.subscriptions.Channels["calls"] = true
	client.subscriptions.Systems["county"] = true // Has filters

	// Event with non-map data
	event := Event{
		Type: "call_start",
		Data: "not a map",
	}

	// Should return true since it can't filter non-map data
	assert.True(t, client.IsSubscribed(event))
}

func TestEventToChannel(t *testing.T) {
	tests := []struct {
		eventType string
		expected  string
	}{
		{"call_start", "calls"},
		{"call_end", "calls"},
		{"call_active", "calls"},
		{"unit_event", "units"},
		{"rate_update", "rates"},
		{"recorder_update", "recorders"},
		{"custom_event", "custom_event"}, // Unknown types pass through
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			assert.Equal(t, tt.expected, eventToChannel(tt.eventType))
		})
	}
}

func TestSubscribeMessage_JSON(t *testing.T) {
	msg := SubscribeMessage{
		Action:     "subscribe",
		Channels:   []string{"calls", "units"},
		Systems:    []string{"county"},
		Talkgroups: []int{12345, 67890},
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err)

	var decoded SubscribeMessage
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, msg.Action, decoded.Action)
	assert.Equal(t, msg.Channels, decoded.Channels)
	assert.Equal(t, msg.Systems, decoded.Systems)
	assert.Equal(t, msg.Talkgroups, decoded.Talkgroups)
}

func TestEvent_JSON(t *testing.T) {
	event := Event{
		Type:      "call_start",
		Timestamp: 1705341234,
		Data: map[string]interface{}{
			"talkgroup": 12345,
			"system":    "county",
		},
	}

	data, err := json.Marshal(event)
	assert.NoError(t, err)

	var decoded Event
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, event.Type, decoded.Type)
	assert.Equal(t, event.Timestamp, decoded.Timestamp)
	assert.NotNil(t, decoded.Data)
}

func TestSubscriptions_ConcurrentAccess(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	// Concurrent subscribes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			client.subscribe(SubscribeMessage{
				Action:     "subscribe",
				Channels:   []string{"calls"},
				Talkgroups: []int{idx * 1000},
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	assert.True(t, client.subscriptions.Channels["calls"])
}

func TestClient_SubscribeConfirmation(t *testing.T) {
	hub := NewHub(zap.NewNop())
	client := NewClient(hub, nil, zap.NewNop())

	msg := SubscribeMessage{
		Action:   "subscribe",
		Channels: []string{"calls"},
	}

	client.subscribe(msg)

	// Check that confirmation was sent
	select {
	case data := <-client.send:
		var response map[string]interface{}
		err := json.Unmarshal(data, &response)
		assert.NoError(t, err)
		assert.Equal(t, "subscribed", response["event"])
	default:
		t.Error("Expected subscription confirmation")
	}
}
