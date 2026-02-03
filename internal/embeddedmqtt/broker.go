// Package embeddedmqtt provides an embedded MQTT broker for self-contained deployments.
package embeddedmqtt

import (
	"fmt"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/hooks/storage/badger"
	"github.com/mochi-mqtt/server/v2/listeners"
	"go.uber.org/zap"
)

// Broker wraps the embedded MQTT broker
type Broker struct {
	server *mqtt.Server
	logger *zap.Logger
	port   int
}

// Config holds embedded MQTT broker configuration
type Config struct {
	Port     int
	Username string
	Password string
	DataPath string // Path for persistent storage (retained messages, etc.)
}

// New creates and starts a new embedded MQTT broker
func New(cfg Config, logger *zap.Logger) (*Broker, error) {
	port := cfg.Port
	if port == 0 {
		port = 1883
	}

	// Create server with default options
	server := mqtt.New(&mqtt.Options{
		InlineClient: true, // Allow the server to publish/subscribe internally
	})

	// Add persistence hook if data path provided
	if cfg.DataPath != "" {
		err := server.AddHook(new(badger.Hook), &badger.Options{
			Path: cfg.DataPath,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add persistence hook: %w", err)
		}
		logger.Info("MQTT persistence enabled", zap.String("path", cfg.DataPath))
	}

	// Add auth hook if credentials provided
	if cfg.Username != "" && cfg.Password != "" {
		err := server.AddHook(new(auth.Hook), &auth.Options{
			Ledger: &auth.Ledger{
				Auth: auth.AuthRules{
					{Username: auth.RString(cfg.Username), Password: auth.RString(cfg.Password), Allow: true},
				},
				ACL: auth.ACLRules{
					{Username: auth.RString(cfg.Username), Filters: auth.Filters{"#": auth.ReadWrite}},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add auth hook: %w", err)
		}
	} else {
		// Allow all connections (no auth)
		err := server.AddHook(new(auth.AllowHook), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to add allow hook: %w", err)
		}
	}

	// Create TCP listener
	tcp := listeners.NewTCP(listeners.Config{
		ID:      "tcp",
		Address: fmt.Sprintf(":%d", port),
	})

	err := server.AddListener(tcp)
	if err != nil {
		return nil, fmt.Errorf("failed to add TCP listener: %w", err)
	}

	// Start the server
	err = server.Serve()
	if err != nil {
		return nil, fmt.Errorf("failed to start MQTT broker: %w", err)
	}

	logger.Info("Embedded MQTT broker started", zap.Int("port", port))

	return &Broker{
		server: server,
		logger: logger,
		port:   port,
	}, nil
}

// Stop gracefully stops the embedded MQTT broker
func (b *Broker) Stop() error {
	if b.server != nil {
		b.logger.Info("Stopping embedded MQTT broker")
		return b.server.Close()
	}
	return nil
}

// Port returns the port the broker is listening on
func (b *Broker) Port() int {
	return b.port
}

// ClientCount returns the number of connected clients
func (b *Broker) ClientCount() int {
	if b.server != nil {
		return len(b.server.Clients.GetAll())
	}
	return 0
}
