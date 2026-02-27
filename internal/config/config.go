package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string `env:"DATABASE_URL,required"`
	MQTTBrokerURL string `env:"MQTT_BROKER_URL"`
	MQTTTopics    string `env:"MQTT_TOPICS" envDefault:"#"`
	MQTTClientID  string `env:"MQTT_CLIENT_ID" envDefault:"tr-engine"`
	MQTTUsername  string `env:"MQTT_USERNAME"`
	MQTTPassword  string `env:"MQTT_PASSWORD"`

	AudioDir   string `env:"AUDIO_DIR" envDefault:"./audio"`
	TRAudioDir string `env:"TR_AUDIO_DIR"`

	// File-watch ingest mode (alternative to MQTT)
	WatchDir          string `env:"WATCH_DIR"`
	WatchInstanceID   string `env:"WATCH_INSTANCE_ID" envDefault:"file-watch"`
	WatchBackfillDays int    `env:"WATCH_BACKFILL_DAYS" envDefault:"7"`

	// HTTP upload ingest mode (rdio-scanner / OpenMHz compatible)
	UploadInstanceID string `env:"UPLOAD_INSTANCE_ID" envDefault:"http-upload"`

	// TR auto-discovery (reads trunk-recorder's config.json + docker-compose.yaml)
	TRDir        string `env:"TR_DIR"`
	CSVWriteback bool   `env:"CSV_WRITEBACK" envDefault:"false"` // write edits back to TR's CSV files on disk

	// P25 system merging: when true (default), systems with the same sysid/wacn
	// are auto-merged into one system with multiple sites. Set to false to keep
	// each TR instance's systems separate even if they share sysid/wacn.
	MergeP25Systems bool `env:"MERGE_P25_SYSTEMS" envDefault:"true"`

	HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`

	AuthEnabled        bool   `env:"AUTH_ENABLED" envDefault:"true"` // set to false to disable all API auth
	AuthToken          string `env:"AUTH_TOKEN"`
	AuthTokenGenerated bool   // true when auto-generated (not from env/config)
	WriteToken         string `env:"WRITE_TOKEN"` // separate token for write operations; if not set, writes use AuthToken
	RateLimitRPS   float64 `env:"RATE_LIMIT_RPS" envDefault:"20"`
	RateLimitBurst int     `env:"RATE_LIMIT_BURST" envDefault:"40"`
	CORSOrigins string `env:"CORS_ORIGINS"` // comma-separated allowed origins; empty = allow all (*)
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

	RawStore         bool   `env:"RAW_STORE" envDefault:"true"`
	RawIncludeTopics string `env:"RAW_INCLUDE_TOPICS"`
	RawExcludeTopics string `env:"RAW_EXCLUDE_TOPICS"`

	// Transcription (optional — disabled when no STT provider is configured)
	STTProvider        string `env:"STT_PROVIDER" envDefault:"whisper"`
	WhisperURL         string        `env:"WHISPER_URL"`
	WhisperAPIKey      string        `env:"WHISPER_API_KEY"`
	WhisperModel       string        `env:"WHISPER_MODEL"`
	WhisperTimeout     time.Duration `env:"WHISPER_TIMEOUT" envDefault:"30s"`
	WhisperTemperature float64       `env:"WHISPER_TEMPERATURE" envDefault:"0.1"`
	WhisperLanguage    string        `env:"WHISPER_LANGUAGE" envDefault:"en"`
	WhisperPrompt      string        `env:"WHISPER_PROMPT"`
	WhisperHotwords    string        `env:"WHISPER_HOTWORDS"`
	WhisperBeamSize    int           `env:"WHISPER_BEAM_SIZE" envDefault:"0"`

	// Anti-hallucination parameters (require custom whisper-server or compatible endpoint)
	WhisperRepetitionPenalty          float64 `env:"WHISPER_REPETITION_PENALTY" envDefault:"0"`
	WhisperNoRepeatNgram              int     `env:"WHISPER_NO_REPEAT_NGRAM" envDefault:"0"`
	WhisperConditionOnPrev            *bool   `env:"WHISPER_CONDITION_ON_PREV"`
	WhisperNoSpeechThreshold          float64 `env:"WHISPER_NO_SPEECH_THRESHOLD" envDefault:"0"`
	WhisperHallucinationThreshold     float64 `env:"WHISPER_HALLUCINATION_THRESHOLD" envDefault:"0"`
	WhisperMaxTokens                  int     `env:"WHISPER_MAX_TOKENS" envDefault:"0"`
	WhisperVadFilter                  bool    `env:"WHISPER_VAD_FILTER" envDefault:"false"`

	// ElevenLabs STT (alternative to Whisper; used when STT_PROVIDER=elevenlabs)
	ElevenLabsAPIKey   string `env:"ELEVENLABS_API_KEY"`
	ElevenLabsModel    string `env:"ELEVENLABS_MODEL" envDefault:"scribe_v2"`
	ElevenLabsKeyterms string `env:"ELEVENLABS_KEYTERMS"`

	// LLM post-processing (optional — disabled when LLM_URL is empty; not yet implemented)
	LLMUrl     string        `env:"LLM_URL"`
	LLMModel   string        `env:"LLM_MODEL"`
	LLMTimeout time.Duration `env:"LLM_TIMEOUT" envDefault:"30s"`

	// Update checker (enabled by default — set UPDATE_CHECK=false to disable)
	UpdateCheck    bool   `env:"UPDATE_CHECK" envDefault:"true"`
	UpdateCheckURL string `env:"UPDATE_CHECK_URL" envDefault:"https://updates.luxprimatech.com/check"`

	// Audio preprocessing (requires sox in PATH)
	PreprocessAudio bool `env:"PREPROCESS_AUDIO" envDefault:"false"`

	// Transcription worker pool
	TranscribeWorkers     int     `env:"TRANSCRIBE_WORKERS" envDefault:"2"`
	TranscribeQueueSize   int     `env:"TRANSCRIBE_QUEUE_SIZE" envDefault:"500"`
	TranscribeMinDuration float64 `env:"TRANSCRIBE_MIN_DURATION" envDefault:"1.0"`
	TranscribeMaxDuration float64 `env:"TRANSCRIBE_MAX_DURATION" envDefault:"300"`
}

// Validate checks that at least one ingest source (MQTT, watch directory, or TR auto-discovery) is configured.
func (c *Config) Validate() error {
	if c.MQTTBrokerURL == "" && c.WatchDir == "" && c.TRDir == "" {
		return fmt.Errorf("at least one of MQTT_BROKER_URL, WATCH_DIR, or TR_DIR must be set")
	}
	return nil
}

// Overrides holds CLI flag values that take priority over env vars.
type Overrides struct {
	EnvFile       string
	HTTPAddr      string
	LogLevel      string
	DatabaseURL   string
	MQTTBrokerURL string
	AudioDir      string
	WatchDir      string
	TRDir         string
	WhisperURL    string
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
	if overrides.WatchDir != "" {
		cfg.WatchDir = overrides.WatchDir
	}
	if overrides.TRDir != "" {
		cfg.TRDir = overrides.TRDir
	}
	if overrides.WhisperURL != "" {
		cfg.WhisperURL = overrides.WhisperURL
	}

	// When auth is explicitly disabled, clear any tokens so middleware passes everything through.
	if !cfg.AuthEnabled {
		cfg.AuthToken = ""
		cfg.WriteToken = ""
	} else if cfg.AuthToken == "" {
		// Auto-generate AUTH_TOKEN if not configured. This ensures the API is always
		// protected from automated scanners. Web pages get the token injected via auth.js.
		// The token changes on each restart; set AUTH_TOKEN in .env for a persistent one.
		b := make([]byte, 32)
		if _, err := rand.Read(b); err == nil {
			cfg.AuthToken = base64.URLEncoding.EncodeToString(b)
			cfg.AuthTokenGenerated = true
		}
	}

	return cfg, nil
}
