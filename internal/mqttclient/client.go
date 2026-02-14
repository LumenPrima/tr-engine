package mqttclient

import (
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
	}

	clientOpts := mqtt.NewClientOptions().
		AddBroker(opts.BrokerURL).
		SetClientID(opts.ClientID).
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
		c.handler(msg.Topic(), msg.Payload())
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
