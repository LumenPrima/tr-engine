package mqttclient

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
)

type MessageHandler func(topic string, payload []byte)

type Client struct {
	conn      mqtt.Client
	topics    []string
	connected atomic.Bool
	log       zerolog.Logger
	handler   MessageHandler
	msgCh     chan mqttMessage
	done      chan struct{}
}

type mqttMessage struct {
	topic   string
	payload []byte
}

type Options struct {
	BrokerURL string
	ClientID  string
	Topics    string
	Username  string
	Password  string
	Log       zerolog.Logger
}

func Connect(opts Options) (*Client, error) {
	c := &Client{
		topics: parseTopics(opts.Topics),
		log:    opts.Log,
		msgCh:  make(chan mqttMessage, 1000),
		done:   make(chan struct{}),
	}

	clientID := opts.ClientID
	if clientID == "" {
		clientID = "tr-engine"
	}
	// Append random suffix to avoid client ID collisions with other instances
	var suffix [4]byte
	rand.Read(suffix[:])
	clientID = clientID + "-" + hex.EncodeToString(suffix[:])

	c.log.Info().Str("client_id", clientID).Msg("connecting with client ID")

	clientOpts := mqtt.NewClientOptions().
		AddBroker(opts.BrokerURL).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOrderMatters(false).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(c.onConnectionLost).
		SetDefaultPublishHandler(c.onMessage)

	if opts.Username != "" {
		clientOpts.SetUsername(opts.Username)
	}
	if opts.Password != "" {
		clientOpts.SetPassword(opts.Password)
	}

	c.conn = mqtt.NewClient(clientOpts)
	token := c.conn.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) SetMessageHandler(h MessageHandler) {
	c.handler = h
	go c.dispatchLoop()
}

func (c *Client) dispatchLoop() {
	for {
		select {
		case msg := <-c.msgCh:
			if c.handler != nil {
				c.handler(msg.topic, msg.payload)
			}
		case <-c.done:
			return
		}
	}
}

func (c *Client) onConnect(client mqtt.Client) {
	c.connected.Store(true)
	c.log.Info().Strs("topics", c.topics).Msg("mqtt connected, subscribing")

	filters := make(map[string]byte, len(c.topics))
	for _, t := range c.topics {
		filters[t] = 0
	}
	token := client.SubscribeMultiple(filters, nil)
	token.Wait()
	if err := token.Error(); err != nil {
		c.log.Error().Err(err).Msg("mqtt subscribe failed")
	}
}

func (c *Client) onConnectionLost(_ mqtt.Client, err error) {
	c.connected.Store(false)
	c.log.Warn().Err(err).Msg("mqtt connection lost, will auto-reconnect")
}

func (c *Client) onMessage(_ mqtt.Client, msg mqtt.Message) {
	if c.handler != nil {
		payload := append([]byte(nil), msg.Payload()...)
		select {
		case c.msgCh <- mqttMessage{topic: msg.Topic(), payload: payload}:
		default:
			c.log.Warn().
				Str("topic", msg.Topic()).
				Int("payload_size", len(payload)).
				Msg("mqtt ingest queue full, dropping message")
		}
		return
	}
	c.log.Debug().
		Str("topic", msg.Topic()).
		Int("payload_size", len(msg.Payload())).
		Msg("mqtt message received")
}

func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

func (c *Client) Close() {
	c.log.Info().Msg("disconnecting mqtt client")
	close(c.done)
	c.conn.Disconnect(1000)
}

func parseTopics(raw string) []string {
	var topics []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		return []string{"#"}
	}
	return topics
}
