package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for tr-engine
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	MQTT          MQTTConfig          `mapstructure:"mqtt"`
	Storage       StorageConfig       `mapstructure:"storage"`
	Deduplication DeduplicationConfig `mapstructure:"deduplication"`
	Logging       LoggingConfig       `mapstructure:"logging"`
}

// ServerConfig holds HTTP/WebSocket server configuration
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig holds PostgreSQL connection configuration
type DatabaseConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	Name           string `mapstructure:"name"`
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	MaxConnections int    `mapstructure:"max_connections"`
	SSLMode        string `mapstructure:"ssl_mode"`

	// Embedded runs an embedded PostgreSQL instance instead of connecting to external
	Embedded bool `mapstructure:"embedded"`
	// EmbeddedDataPath is the directory for embedded PostgreSQL data (default: ./data/postgres)
	EmbeddedDataPath string `mapstructure:"embedded_data_path"`
}

// MQTTConfig holds MQTT broker configuration
type MQTTConfig struct {
	Broker   string     `mapstructure:"broker"`
	ClientID string     `mapstructure:"client_id"`
	Username string     `mapstructure:"username"`
	Password string     `mapstructure:"password"`
	Topics   MQTTTopics `mapstructure:"topics"`
	QoS      int        `mapstructure:"qos"`

	// Embedded runs an embedded MQTT broker instead of connecting to external
	Embedded bool `mapstructure:"embedded"`
	// EmbeddedPort is the port for the embedded MQTT broker (default: 1883)
	EmbeddedPort int `mapstructure:"embedded_port"`
}

// MQTTTopics holds the MQTT topic patterns
type MQTTTopics struct {
	Status   string `mapstructure:"status"`
	Units    string `mapstructure:"units"`
	Messages string `mapstructure:"messages"`
}

// StorageConfig holds file storage configuration
type StorageConfig struct {
	AudioPath string `mapstructure:"audio_path"`
}

// DeduplicationConfig holds deduplication settings
type DeduplicationConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	TimeWindowSeconds int     `mapstructure:"time_window_seconds"`
	Threshold         float64 `mapstructure:"threshold"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		// Config file is optional if environment variables are set
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only warn if file was specified but not found
			if configPath != "" && configPath != "config.yaml" {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	// Environment variable overrides
	v.SetEnvPrefix("TR_ENGINE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Also support direct environment variables for common settings
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand environment variables in sensitive fields
	cfg.Database.Password = expandEnv(cfg.Database.Password)
	cfg.MQTT.Password = expandEnv(cfg.MQTT.Password)

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "tr_engine")
	v.SetDefault("database.user", "tr_engine")
	v.SetDefault("database.max_connections", 50)
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.embedded", false)
	v.SetDefault("database.embedded_data_path", "./data/postgres")

	// MQTT defaults (matches trunk-recorder MQTT plugin defaults)
	v.SetDefault("mqtt.broker", "tcp://localhost:1883")
	v.SetDefault("mqtt.client_id", "tr-engine")
	v.SetDefault("mqtt.topics.status", "feeds/#")
	v.SetDefault("mqtt.topics.units", "units/#")
	v.SetDefault("mqtt.topics.messages", "")
	v.SetDefault("mqtt.qos", 1)
	v.SetDefault("mqtt.embedded", false)
	v.SetDefault("mqtt.embedded_port", 1883)

	// Storage defaults
	v.SetDefault("storage.audio_path", "/data/tr-engine/audio")

	// Deduplication defaults
	v.SetDefault("deduplication.enabled", true)
	v.SetDefault("deduplication.time_window_seconds", 3)
	v.SetDefault("deduplication.threshold", 0.7)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
}

func bindEnvVars(v *viper.Viper) {
	// Database
	v.BindEnv("database.host", "DB_HOST")
	v.BindEnv("database.port", "DB_PORT")
	v.BindEnv("database.name", "DB_NAME")
	v.BindEnv("database.user", "DB_USER")
	v.BindEnv("database.password", "DB_PASSWORD")

	// MQTT
	v.BindEnv("mqtt.broker", "MQTT_BROKER")
	v.BindEnv("mqtt.username", "MQTT_USERNAME")
	v.BindEnv("mqtt.password", "MQTT_PASSWORD")

	// Storage
	v.BindEnv("storage.audio_path", "AUDIO_PATH")
}

// expandEnv expands ${VAR} or $VAR in string values
func expandEnv(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envVar := s[2 : len(s)-1]
		return os.Getenv(envVar)
	}
	if strings.HasPrefix(s, "$") {
		return os.Getenv(s[1:])
	}
	return s
}

// ConnectionString returns a PostgreSQL connection string
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode,
	)
}
