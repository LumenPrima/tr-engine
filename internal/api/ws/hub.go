package ws

import (
	"encoding/json"
	"sync"

	"go.uber.org/zap"
)

// Event represents a WebSocket event
type Event struct {
	Type      string      `json:"event"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Event
	register   chan *Client
	unregister chan *Client
	logger     *zap.Logger
	mu         sync.RWMutex
	shutdown   chan struct{}
}

// NewHub creates a new Hub
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
		shutdown:   make(chan struct{}),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case <-h.shutdown:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("Client connected", zap.Int("clients", len(h.clients)))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Debug("Client disconnected", zap.Int("clients", len(h.clients)))

		case event := <-h.broadcast:
			h.mu.RLock()
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Error("Failed to marshal event", zap.Error(err))
				h.mu.RUnlock()
				continue
			}

			for client := range h.clients {
				// Check if client is subscribed to this event type
				if !client.IsSubscribed(event) {
					continue
				}

				select {
				case client.send <- data:
				default:
					// Client buffer full, will be cleaned up
					h.logger.Warn("Client buffer full, dropping message")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all subscribed clients
func (h *Hub) Broadcast(event Event) {
	select {
	case h.broadcast <- event:
	default:
		h.logger.Warn("Broadcast channel full, dropping event")
	}
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Shutdown shuts down the hub
func (h *Hub) Shutdown() {
	close(h.shutdown)
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
