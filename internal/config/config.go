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
	Transcription TranscriptionConfig `mapstructure:"transcription"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Logging       LoggingConfig       `mapstructure:"logging"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled   bool              `mapstructure:"enabled"`
	Dashboard DashboardAuth     `mapstructure:"dashboard"`
	APIKeys   []string          `mapstructure:"api_keys"`
}

// DashboardAuth holds dashboard login credentials
type DashboardAuth struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
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
	// Mode determines how audio is ingested:
	// - "copy" (default): receive audio via MQTT and save to audio_path
	// - "external": reference TR's files via MQTT notifications
	// - "watch": watch audio_path for new files (no MQTT needed)
	Mode string `mapstructure:"mode"`
	// LogPath is the path to TR's log directory (optional, for watch mode)
	// When set, enables real-time call tracking from log events
	LogPath string `mapstructure:"log_path"`
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

// TranscriptionConfig holds speech-to-text transcription configuration
type TranscriptionConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	Provider    string  `mapstructure:"provider"`    // "openai", "http", "embedded"
	Concurrency int     `mapstructure:"concurrency"` // Number of parallel workers
	RetryCount  int     `mapstructure:"retry_count"` // Max retries on failure
	Language    string  `mapstructure:"language"`    // Language code (e.g., "en")
	MinDuration float64 `mapstructure:"min_duration"` // Minimum call duration in seconds to transcribe

	OpenAI       OpenAITranscriptionConfig       `mapstructure:"openai"`
	HTTP         HTTPTranscriptionConfig         `mapstructure:"http"`
	Embedded     EmbeddedTranscriptionConfig     `mapstructure:"embedded"`
	Preprocess   AudioPreprocessConfig           `mapstructure:"preprocess"`
}

// AudioPreprocessConfig holds audio preprocessing configuration
type AudioPreprocessConfig struct {
	Enabled      bool   `mapstructure:"enabled"`       // Enable preprocessing (default: true)
	SampleRate   int    `mapstructure:"sample_rate"`   // Target sample rate (default: 16000)
	HighpassHz   int    `mapstructure:"highpass_hz"`   // High-pass filter cutoff (default: 300)
	LowpassHz    int    `mapstructure:"lowpass_hz"`    // Low-pass filter cutoff (default: 3000)
	Normalize    bool   `mapstructure:"normalize"`     // Normalize audio levels (default: true)
	CustomFilter string `mapstructure:"custom_filter"` // Custom sox filter chain (overrides above)
}

// OpenAITranscriptionConfig holds OpenAI Whisper API configuration
type OpenAITranscriptionConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"` // Empty = OpenAI, or set for compatible providers (Groq, etc.)
	Model   string `mapstructure:"model"`    // "whisper-1", "whisper-large-v3", etc.
	Prompt  string `mapstructure:"prompt"`   // Optional prompt to guide transcription (terminology, context)
}

// HTTPTranscriptionConfig holds self-hosted HTTP Whisper server configuration
type HTTPTranscriptionConfig struct {
	URL     string `mapstructure:"url"`     // e.g., "http://localhost:9000/asr"
	APIKey  string `mapstructure:"api_key"` // Optional authentication
	Timeout int    `mapstructure:"timeout"` // Request timeout in seconds
}

// EmbeddedTranscriptionConfig holds embedded whisper.cpp configuration
type EmbeddedTranscriptionConfig struct {
	ModelPath string `mapstructure:"model_path"` // Directory for model files
	ModelSize string `mapstructure:"model_size"` // "tiny", "base", "small"
	Threads   int    `mapstructure:"threads"`    // CPU threads for inference
	AutoLoad  bool   `mapstructure:"auto_load"`  // Download model if missing
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
	cfg.Transcription.OpenAI.APIKey = expandEnv(cfg.Transcription.OpenAI.APIKey)
	cfg.Transcription.HTTP.APIKey = expandEnv(cfg.Transcription.HTTP.APIKey)

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
	v.SetDefault("storage.mode", "copy") // "copy" or "external"

	// Deduplication defaults
	v.SetDefault("deduplication.enabled", true)
	v.SetDefault("deduplication.time_window_seconds", 3)
	v.SetDefault("deduplication.threshold", 0.7)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Auth defaults
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.dashboard.username", "admin")
	v.SetDefault("auth.dashboard.password", "admin")

	// Transcription defaults
	v.SetDefault("transcription.enabled", false)
	v.SetDefault("transcription.provider", "openai")
	v.SetDefault("transcription.concurrency", 2)
	v.SetDefault("transcription.retry_count", 3)
	v.SetDefault("transcription.language", "en")
	v.SetDefault("transcription.min_duration", 2.0)

	// OpenAI provider defaults
	v.SetDefault("transcription.openai.model", "whisper-1")

	// HTTP provider defaults
	v.SetDefault("transcription.http.url", "http://localhost:9000/asr")

	// Audio preprocessing defaults (voice bandpass filter)
	v.SetDefault("transcription.preprocess.enabled", true)
	v.SetDefault("transcription.preprocess.sample_rate", 16000)
	v.SetDefault("transcription.preprocess.highpass_hz", 300)
	v.SetDefault("transcription.preprocess.lowpass_hz", 3000)
	v.SetDefault("transcription.preprocess.normalize", true)
	v.SetDefault("transcription.http.timeout", 60)

	// Embedded provider defaults
	v.SetDefault("transcription.embedded.model_path", "./data/models")
	v.SetDefault("transcription.embedded.model_size", "base")
	v.SetDefault("transcription.embedded.threads", 4)
	v.SetDefault("transcription.embedded.auto_load", true)
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

	// Transcription
	v.BindEnv("transcription.enabled", "TRANSCRIPTION_ENABLED")
	v.BindEnv("transcription.provider", "TRANSCRIPTION_PROVIDER")
	v.BindEnv("transcription.openai.api_key", "OPENAI_API_KEY")
	v.BindEnv("transcription.openai.base_url", "OPENAI_BASE_URL")
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
