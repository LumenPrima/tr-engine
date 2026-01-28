package mqtt

import (
	"context"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"go.uber.org/zap"
)

// Client manages the MQTT connection
type Client struct {
	config    config.MQTTConfig
	client    mqtt.Client
	processor *ingest.Processor
	logger    *zap.Logger
	handlers  *Handlers

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewClient creates a new MQTT client
func NewClient(cfg config.MQTTConfig, processor *ingest.Processor, logger *zap.Logger) (*Client, error) {
	c := &Client{
		config:    cfg,
		processor: processor,
		logger:    logger,
		handlers:  NewHandlers(processor, logger),
	}

	return c, nil
}

// Connect establishes the MQTT connection and subscribes to topics
func (c *Client) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	opts := mqtt.NewClientOptions().
		AddBroker(c.config.Broker).
		SetClientID(c.config.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetKeepAlive(60 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetCleanSession(true).
		SetOrderMatters(false).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(c.onConnectionLost).
		SetReconnectingHandler(c.onReconnecting)

	if c.config.Username != "" {
		opts.SetUsername(c.config.Username)
	}
	if c.config.Password != "" {
		opts.SetPassword(c.config.Password)
	}

	c.client = mqtt.NewClient(opts)

	c.logger.Info("Connecting to MQTT broker", zap.String("broker", c.config.Broker))

	token := c.client.Connect()
	if !token.WaitTimeout(30 * time.Second) {
		return fmt.Errorf("connection timeout")
	}
	if token.Error() != nil {
		return fmt.Errorf("connection error: %w", token.Error())
	}

	return nil
}

// Disconnect gracefully disconnects from the MQTT broker
func (c *Client) Disconnect() {
	if c.cancel != nil {
		c.cancel()
	}

	c.wg.Wait()

	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect(1000)
	}

	c.logger.Info("Disconnected from MQTT broker")
}

func (c *Client) onConnect(client mqtt.Client) {
	c.logger.Info("Connected to MQTT broker")

	// Subscribe to status topics
	if c.config.Topics.Status != "" {
		c.subscribe(c.config.Topics.Status, c.handlers.HandleStatusMessage)
	}

	// Subscribe to unit topics
	if c.config.Topics.Units != "" {
		c.subscribe(c.config.Topics.Units, c.handlers.HandleUnitMessage)
	}

	// Subscribe to message topics (optional, high volume)
	if c.config.Topics.Messages != "" {
		c.subscribe(c.config.Topics.Messages, c.handlers.HandleTrunkMessage)
	}
}

func (c *Client) subscribe(topic string, handler mqtt.MessageHandler) {
	token := c.client.Subscribe(topic, byte(c.config.QoS), handler)
	if token.WaitTimeout(10 * time.Second) && token.Error() != nil {
		c.logger.Error("Failed to subscribe",
			zap.String("topic", topic),
			zap.Error(token.Error()),
		)
	} else {
		c.logger.Info("Subscribed to topic", zap.String("topic", topic))
	}
}

func (c *Client) onConnectionLost(client mqtt.Client, err error) {
	c.logger.Warn("Connection lost", zap.Error(err))
}

func (c *Client) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	c.logger.Info("Reconnecting to MQTT broker")
}
