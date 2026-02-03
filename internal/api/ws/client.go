package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Client represents a WebSocket client connection
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions *Subscriptions
	logger        *zap.Logger
	mu            sync.RWMutex
}

// Subscriptions holds a client's subscription preferences
type Subscriptions struct {
	Channels   map[string]bool  // "calls", "units", "rates", "recorders"
	Systems    map[string]bool  // Filter by system short_name
	Talkgroups map[int]bool     // Filter by talkgroup ID
	Units      map[int64]bool   // Filter by unit radio ID
	mu         sync.RWMutex
}

// SubscribeMessage is sent by clients to subscribe to events
type SubscribeMessage struct {
	Action     string   `json:"action"`     // "subscribe" or "unsubscribe"
	Channels   []string `json:"channels"`   // Event channels
	Systems    []string `json:"systems"`    // System filters
	Talkgroups []int    `json:"talkgroups"` // Talkgroup filters
	Units      []int64  `json:"units"`      // Unit radio ID filters
}

// NewClient creates a new Client
func NewClient(hub *Hub, conn *websocket.Conn, logger *zap.Logger) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		logger: logger,
		subscriptions: &Subscriptions{
			Channels:   make(map[string]bool),
			Systems:    make(map[string]bool),
			Talkgroups: make(map[int]bool),
			Units:      make(map[int64]bool),
		},
	}
}

// HandleWebSocket handles a new WebSocket connection
func HandleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	client := NewClient(hub, conn, logger)
	hub.Register(client)

	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNoStatusReceived,
				websocket.CloseNormalClosure,
			) {
				c.logger.Error("WebSocket read error", zap.Error(err))
			}
			break
		}

		c.handleMessage(message)
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Batch any queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(message []byte) {
	var msg SubscribeMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		c.logger.Error("Failed to parse WebSocket message", zap.Error(err))
		return
	}

	switch msg.Action {
	case "subscribe":
		c.subscribe(msg)
	case "unsubscribe":
		c.unsubscribe(msg)
	default:
		c.logger.Debug("Unknown WebSocket action", zap.String("action", msg.Action))
	}
}

// subscribe adds subscriptions
func (c *Client) subscribe(msg SubscribeMessage) {
	c.subscriptions.mu.Lock()
	defer c.subscriptions.mu.Unlock()

	for _, ch := range msg.Channels {
		c.subscriptions.Channels[ch] = true
	}
	for _, sys := range msg.Systems {
		c.subscriptions.Systems[sys] = true
	}
	for _, tg := range msg.Talkgroups {
		c.subscriptions.Talkgroups[tg] = true
	}
	for _, u := range msg.Units {
		c.subscriptions.Units[u] = true
	}

	c.logger.Debug("Client subscribed",
		zap.Strings("channels", msg.Channels),
		zap.Strings("systems", msg.Systems),
		zap.Ints("talkgroups", msg.Talkgroups),
		zap.Int64s("units", msg.Units),
	)

	// Send confirmation with current subscription state
	response := map[string]interface{}{
		"event": "subscribed",
		"data": map[string]interface{}{
			"added":    msg,
			"channels": c.getSubscribedChannels(),
			"systems":  c.getSubscribedSystems(),
		},
	}
	if data, err := json.Marshal(response); err == nil {
		select {
		case c.send <- data:
		default:
		}
	}
}

// unsubscribe removes subscriptions
func (c *Client) unsubscribe(msg SubscribeMessage) {
	c.subscriptions.mu.Lock()
	defer c.subscriptions.mu.Unlock()

	for _, ch := range msg.Channels {
		delete(c.subscriptions.Channels, ch)
	}
	for _, sys := range msg.Systems {
		delete(c.subscriptions.Systems, sys)
	}
	for _, tg := range msg.Talkgroups {
		delete(c.subscriptions.Talkgroups, tg)
	}
	for _, u := range msg.Units {
		delete(c.subscriptions.Units, u)
	}

	c.logger.Debug("Client unsubscribed",
		zap.Strings("channels", msg.Channels),
		zap.Strings("systems", msg.Systems),
		zap.Ints("talkgroups", msg.Talkgroups),
		zap.Int64s("units", msg.Units),
	)

	// Send confirmation with current subscription state
	response := map[string]interface{}{
		"event": "unsubscribed",
		"data": map[string]interface{}{
			"removed":  msg,
			"channels": c.getSubscribedChannels(),
			"systems":  c.getSubscribedSystems(),
		},
	}
	if data, err := json.Marshal(response); err == nil {
		select {
		case c.send <- data:
		default:
		}
	}
}

// IsSubscribed checks if a client should receive an event
func (c *Client) IsSubscribed(event Event) bool {
	c.subscriptions.mu.RLock()
	defer c.subscriptions.mu.RUnlock()

	// If no channels subscribed, don't send anything
	if len(c.subscriptions.Channels) == 0 {
		return false
	}

	// Map event types to channels
	channel := eventToChannel(event.Type)
	if !c.subscriptions.Channels[channel] && !c.subscriptions.Channels["*"] {
		return false
	}

	// If no filters set, send all events for subscribed channels
	if len(c.subscriptions.Systems) == 0 && len(c.subscriptions.Talkgroups) == 0 && len(c.subscriptions.Units) == 0 {
		return true
	}

	// Extract system, talkgroup, and unit from event data
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return true // Can't filter, send it
	}

	// Check system filter
	if len(c.subscriptions.Systems) > 0 {
		if sys, ok := data["system"].(string); ok {
			if !c.subscriptions.Systems[sys] {
				return false
			}
		}
	}

	// Check talkgroup filter
	if len(c.subscriptions.Talkgroups) > 0 {
		if tg, ok := data["talkgroup"].(float64); ok {
			if !c.subscriptions.Talkgroups[int(tg)] {
				return false
			}
		} else if tg, ok := data["talkgroup"].(int); ok {
			if !c.subscriptions.Talkgroups[tg] {
				return false
			}
		}
	}

	// Check unit filter
	if len(c.subscriptions.Units) > 0 {
		if u, ok := data["unit"].(float64); ok {
			if !c.subscriptions.Units[int64(u)] {
				return false
			}
		} else if u, ok := data["unit"].(int64); ok {
			if !c.subscriptions.Units[u] {
				return false
			}
		} else if u, ok := data["unit"].(int); ok {
			if !c.subscriptions.Units[int64(u)] {
				return false
			}
		}
	}

	return true
}

// eventToChannel maps event types to subscription channels
func eventToChannel(eventType string) string {
	switch eventType {
	case "call_start", "call_end", "call_active", "audio_available":
		return "calls"
	case "unit_event":
		return "units"
	case "rate_update":
		return "rates"
	case "recorder_update":
		return "recorders"
	case "transcription_complete":
		return "transcriptions"
	default:
		return eventType
	}
}

// getSubscribedChannels returns a list of channels the client is subscribed to
// Must be called while holding subscriptions lock
func (c *Client) getSubscribedChannels() []string {
	channels := make([]string, 0, len(c.subscriptions.Channels))
	for ch := range c.subscriptions.Channels {
		channels = append(channels, ch)
	}
	return channels
}

// getSubscribedSystems returns a list of systems the client is subscribed to
// Must be called while holding subscriptions lock
func (c *Client) getSubscribedSystems() []string {
	systems := make([]string, 0, len(c.subscriptions.Systems))
	for sys := range c.subscriptions.Systems {
		systems = append(systems, sys)
	}
	return systems
}
