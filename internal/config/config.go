package config

import (
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string `env:"DATABASE_URL,required"`
	MQTTBrokerURL string `env:"MQTT_BROKER_URL,required"`
	MQTTTopics    string `env:"MQTT_TOPICS" envDefault:"#"`
	MQTTClientID  string `env:"MQTT_CLIENT_ID" envDefault:"tr-engine"`
	MQTTUsername  string `env:"MQTT_USERNAME"`
	MQTTPassword  string `env:"MQTT_PASSWORD"`

	AudioDir string `env:"AUDIO_DIR" envDefault:"./audio"`

	HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`

	AuthToken string `env:"AUTH_TOKEN"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
}

// Overrides holds CLI flag values that take priority over env vars.
type Overrides struct {
	EnvFile       string
	HTTPAddr      string
	LogLevel      string
	DatabaseURL   string
	MQTTBrokerURL string
	AudioDir      string
}

// Load reads configuration from .env file, environment variables, and CLI overrides.
// Priority: CLI flags > environment variables > .env file > struct defaults.
func Load(overrides Overrides) (*Config, error) {
	// Load .env file (silent if missing)
	envFile := overrides.EnvFile
	if envFile == "" {
		envFile = ".env"
	}
	if _, err := os.Stat(envFile); err == nil {
		_ = godotenv.Load(envFile)
	}

	// Parse environment variables into config struct
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// Apply CLI overrides (non-empty values win)
	if overrides.HTTPAddr != "" {
		cfg.HTTPAddr = overrides.HTTPAddr
	}
	if overrides.LogLevel != "" {
		cfg.LogLevel = overrides.LogLevel
	}
	if overrides.DatabaseURL != "" {
		cfg.DatabaseURL = overrides.DatabaseURL
	}
	if overrides.MQTTBrokerURL != "" {
		cfg.MQTTBrokerURL = overrides.MQTTBrokerURL
	}
	if overrides.AudioDir != "" {
		cfg.AudioDir = overrides.AudioDir
	}

	return cfg, nil
}
