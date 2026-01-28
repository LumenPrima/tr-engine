package ws

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestHub(t *testing.T) *Hub {
	logger := zap.NewNop()
	return NewHub(logger)
}

func TestNewHub(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	assert.NotNil(t, hub)
	assert.NotNil(t, hub.clients)
	assert.NotNil(t, hub.broadcast)
	assert.NotNil(t, hub.register)
	assert.NotNil(t, hub.unregister)
	assert.NotNil(t, hub.shutdown)
	assert.Equal(t, logger, hub.logger)
	assert.Equal(t, 0, len(hub.clients))
}

func TestHub_ClientCount(t *testing.T) {
	hub := newTestHub(t)

	// Initially zero
	assert.Equal(t, 0, hub.ClientCount())

	// Manually add clients (simulating registration without running hub)
	hub.mu.Lock()
	hub.clients[&Client{}] = true
	hub.clients[&Client{}] = true
	hub.mu.Unlock()

	assert.Equal(t, 2, hub.ClientCount())
}

func TestHub_RegisterUnregister(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	// Create a mock client
	client := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   make(map[string]bool),
			Systems:    make(map[string]bool),
			Talkgroups: make(map[int]bool),
		},
	}

	// Register
	hub.Register(client)
	time.Sleep(10 * time.Millisecond) // Give hub time to process

	assert.Equal(t, 1, hub.ClientCount())

	// Unregister
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_Broadcast(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	// Create clients with subscriptions
	client1 := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   map[string]bool{"calls": true},
			Systems:    make(map[string]bool),
			Talkgroups: make(map[int]bool),
		},
	}
	client2 := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   map[string]bool{"units": true},
			Systems:    make(map[string]bool),
			Talkgroups: make(map[int]bool),
		},
	}

	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(10 * time.Millisecond)

	// Broadcast a call event
	event := Event{
		Type:      "call_start",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"talkgroup": 12345},
	}
	hub.Broadcast(event)
	time.Sleep(10 * time.Millisecond)

	// Client1 should receive (subscribed to calls)
	select {
	case msg := <-client1.send:
		var received Event
		err := json.Unmarshal(msg, &received)
		require.NoError(t, err)
		assert.Equal(t, "call_start", received.Type)
	default:
		t.Error("Client1 should have received the message")
	}

	// Client2 should NOT receive (not subscribed to calls)
	select {
	case <-client2.send:
		t.Error("Client2 should not have received the message")
	default:
		// Expected
	}
}

func TestHub_BroadcastWithSystemFilter(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	// Client filtered to specific system
	client := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   map[string]bool{"calls": true},
			Systems:    map[string]bool{"county": true},
			Talkgroups: make(map[int]bool),
		},
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Event matching system
	event1 := Event{
		Type:      "call_start",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"system": "county"},
	}
	hub.Broadcast(event1)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		// Expected - system matches
	default:
		t.Error("Client should have received event matching system filter")
	}

	// Event not matching system
	event2 := Event{
		Type:      "call_start",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"system": "city"},
	}
	hub.Broadcast(event2)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		t.Error("Client should not have received event for different system")
	default:
		// Expected
	}
}

func TestHub_BroadcastWithTalkgroupFilter(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	// Client filtered to specific talkgroup
	client := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   map[string]bool{"calls": true},
			Systems:    make(map[string]bool),
			Talkgroups: map[int]bool{12345: true},
		},
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Event matching talkgroup (as float64, which is how JSON unmarshals numbers)
	event1 := Event{
		Type:      "call_start",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"talkgroup": float64(12345)},
	}
	hub.Broadcast(event1)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		// Expected
	default:
		t.Error("Client should have received event matching talkgroup filter")
	}

	// Event not matching talkgroup
	event2 := Event{
		Type:      "call_start",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"talkgroup": float64(99999)},
	}
	hub.Broadcast(event2)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		t.Error("Client should not have received event for different talkgroup")
	default:
		// Expected
	}
}

func TestHub_Shutdown(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()

	// Add clients
	client1 := &Client{send: make(chan []byte, 256), subscriptions: &Subscriptions{
		Channels:   make(map[string]bool),
		Systems:    make(map[string]bool),
		Talkgroups: make(map[int]bool),
	}}
	client2 := &Client{send: make(chan []byte, 256), subscriptions: &Subscriptions{
		Channels:   make(map[string]bool),
		Systems:    make(map[string]bool),
		Talkgroups: make(map[int]bool),
	}}

	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 2, hub.ClientCount())

	// Shutdown
	hub.Shutdown()
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_ConcurrentAccess(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	var wg sync.WaitGroup
	numClients := 20

	// Concurrently register clients
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			send: make(chan []byte, 256),
			subscriptions: &Subscriptions{
				Channels:   map[string]bool{"calls": true},
				Systems:    make(map[string]bool),
				Talkgroups: make(map[int]bool),
			},
		}
	}

	// Register all clients concurrently
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(idx int) {
			defer wg.Done()
			hub.Register(clients[idx])
		}(i)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, numClients, hub.ClientCount())

	// Concurrent broadcasts
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer wg.Done()
			event := Event{
				Type:      "call_start",
				Timestamp: time.Now().Unix(),
				Data:      map[string]interface{}{"index": idx},
			}
			hub.Broadcast(event)
		}(i)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Unregister all concurrently
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(idx int) {
			defer wg.Done()
			hub.Unregister(clients[idx])
		}(i)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_BroadcastChannelFull(t *testing.T) {
	hub := newTestHub(t)
	// Don't run the hub - let the broadcast channel fill up

	// Fill the broadcast channel
	for i := 0; i < 256; i++ {
		hub.Broadcast(Event{Type: "test", Data: i})
	}

	// This should not block (drops the event)
	hub.Broadcast(Event{Type: "dropped", Data: "overflow"})
}

func TestHub_WildcardSubscription(t *testing.T) {
	hub := newTestHub(t)
	go hub.Run()
	defer hub.Shutdown()

	// Client subscribed to all channels via wildcard
	client := &Client{
		send: make(chan []byte, 256),
		subscriptions: &Subscriptions{
			Channels:   map[string]bool{"*": true},
			Systems:    make(map[string]bool),
			Talkgroups: make(map[int]bool),
		},
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Should receive any event type
	events := []string{"call_start", "unit_event", "rate_update", "recorder_update"}
	for _, evType := range events {
		hub.Broadcast(Event{Type: evType, Data: map[string]interface{}{}})
		time.Sleep(10 * time.Millisecond)

		select {
		case <-client.send:
			// Expected
		default:
			t.Errorf("Client with wildcard should receive %s events", evType)
		}
	}
}
