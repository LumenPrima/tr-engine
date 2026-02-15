package config

import (
	"os"
	"testing"
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
