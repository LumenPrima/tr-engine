package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set required env vars for all subtests
	cleanup := setEnvs(t, map[string]string{
		"DATABASE_URL":   "postgres://localhost/test",
		"MQTT_BROKER_URL": "tcp://localhost:1883",
	})
	defer cleanup()

	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load(Overrides{EnvFile: "nonexistent.env"})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.HTTPAddr != ":8080" {
			t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
		}
		if cfg.AudioDir != "./audio" {
			t.Errorf("AudioDir = %q, want ./audio", cfg.AudioDir)
		}
		if cfg.MQTTTopics != "#" {
			t.Errorf("MQTTTopics = %q, want #", cfg.MQTTTopics)
		}
		if cfg.MQTTClientID != "tr-engine" {
			t.Errorf("MQTTClientID = %q, want tr-engine", cfg.MQTTClientID)
		}
		if !cfg.RawStore {
			t.Error("RawStore = false, want true")
		}
	})

	t.Run("cli_overrides_take_priority", func(t *testing.T) {
		cfg, err := Load(Overrides{
			EnvFile:       "nonexistent.env",
			HTTPAddr:      ":9090",
			LogLevel:      "debug",
			DatabaseURL:   "postgres://override/db",
			MQTTBrokerURL: "tcp://override:1883",
			AudioDir:      "/tmp/audio",
		})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.HTTPAddr != ":9090" {
			t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
		}
		if cfg.DatabaseURL != "postgres://override/db" {
			t.Errorf("DatabaseURL = %q, want override", cfg.DatabaseURL)
		}
		if cfg.MQTTBrokerURL != "tcp://override:1883" {
			t.Errorf("MQTTBrokerURL = %q, want override", cfg.MQTTBrokerURL)
		}
		if cfg.AudioDir != "/tmp/audio" {
			t.Errorf("AudioDir = %q, want /tmp/audio", cfg.AudioDir)
		}
	})

	t.Run("env_vars_read", func(t *testing.T) {
		cfg, err := Load(Overrides{EnvFile: "nonexistent.env"})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DatabaseURL != "postgres://localhost/test" {
			t.Errorf("DatabaseURL = %q, want postgres://localhost/test", cfg.DatabaseURL)
		}
		if cfg.MQTTBrokerURL != "tcp://localhost:1883" {
			t.Errorf("MQTTBrokerURL = %q, want tcp://localhost:1883", cfg.MQTTBrokerURL)
		}
	})

	t.Run("empty_overrides_use_env", func(t *testing.T) {
		cfg, err := Load(Overrides{EnvFile: "nonexistent.env"})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		// Empty override fields should not overwrite env values
		if cfg.DatabaseURL != "postgres://localhost/test" {
			t.Errorf("DatabaseURL = %q, want env value", cfg.DatabaseURL)
		}
	})
}

func TestLoadMissingRequired(t *testing.T) {
	// Clear any existing values
	cleanup := setEnvs(t, map[string]string{
		"DATABASE_URL":    "",
		"MQTT_BROKER_URL": "",
	})
	defer cleanup()
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("MQTT_BROKER_URL")

	_, err := Load(Overrides{EnvFile: "nonexistent.env"})
	if err == nil {
		t.Error("expected error when required env vars are missing")
	}
}

func TestStreamConfig(t *testing.T) {
	cleanup := setEnvs(t, map[string]string{
		"DATABASE_URL":       "postgres://localhost/test",
		"MQTT_BROKER_URL":    "tcp://localhost:1883",
		"STREAM_LISTEN":      ":9123",
		"STREAM_OPUS_BITRATE": "24000",
		"STREAM_MAX_CLIENTS": "25",
		"STREAM_IDLE_TIMEOUT": "45s",
	})
	defer cleanup()

	cfg, err := Load(Overrides{EnvFile: "nonexistent.env"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StreamListen != ":9123" {
		t.Errorf("StreamListen = %q, want :9123", cfg.StreamListen)
	}
	if cfg.StreamOpusBitrate != 24000 {
		t.Errorf("StreamOpusBitrate = %d, want 24000", cfg.StreamOpusBitrate)
	}
	if cfg.StreamMaxClients != 25 {
		t.Errorf("StreamMaxClients = %d, want 25", cfg.StreamMaxClients)
	}
	if cfg.StreamIdleTimeout != 45*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 45s", cfg.StreamIdleTimeout)
	}
}

func TestStreamConfigDefaults(t *testing.T) {
	cleanup := setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"MQTT_BROKER_URL": "tcp://localhost:1883",
	})
	defer cleanup()

	cfg, err := Load(Overrides{EnvFile: "nonexistent.env"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StreamListen != "" {
		t.Errorf("StreamListen = %q, want empty", cfg.StreamListen)
	}
	if cfg.StreamSampleRate != 8000 {
		t.Errorf("StreamSampleRate = %d, want 8000", cfg.StreamSampleRate)
	}
	if cfg.StreamOpusBitrate != 16000 {
		t.Errorf("StreamOpusBitrate = %d, want 16000", cfg.StreamOpusBitrate)
	}
	if cfg.StreamMaxClients != 50 {
		t.Errorf("StreamMaxClients = %d, want 50", cfg.StreamMaxClients)
	}
	if cfg.StreamIdleTimeout != 30*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 30s", cfg.StreamIdleTimeout)
	}
}

// setEnvs sets environment variables and returns a cleanup function.
func setEnvs(t *testing.T, envs map[string]string) func() {
	t.Helper()
	originals := make(map[string]string)
	unset := make([]string, 0)

	for k, v := range envs {
		if orig, ok := os.LookupEnv(k); ok {
			originals[k] = orig
		} else {
			unset = append(unset, k)
		}
		os.Setenv(k, v)
	}

	return func() {
		for k, v := range originals {
			os.Setenv(k, v)
		}
		for _, k := range unset {
			os.Unsetenv(k)
		}
	}
}
