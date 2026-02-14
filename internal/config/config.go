package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	DatabaseURL  string `env:"DATABASE_URL,required"`
	MQTTBrokerURL string `env:"MQTT_BROKER_URL,required"`
	MQTTTopics   string `env:"MQTT_TOPICS" envDefault:"#"`
	MQTTClientID string `env:"MQTT_CLIENT_ID" envDefault:"tr-engine"`

	HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`

	AuthToken string `env:"AUTH_TOKEN"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
