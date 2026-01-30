package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "tr-engine-test-config")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an empty config file
	configPath := filepath.Join(tmpDir, "empty.yaml")
	err = os.WriteFile(configPath, []byte(""), 0644)
	require.NoError(t, err)

	// Load with empty config file to trigger defaults
	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Verify defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "tr_engine", cfg.Database.Name)
	assert.Equal(t, "disable", cfg.Database.SSLMode)
	assert.Equal(t, "tcp://localhost:1883", cfg.MQTT.Broker)
	assert.Equal(t, "feeds/#", cfg.MQTT.Topics.Status)
	assert.Equal(t, "copy", cfg.Storage.Mode)
	assert.Equal(t, "json", cfg.Logging.Format)
}

func TestLoad_File(t *testing.T) {
	// Create a temporary directory and config file
	tmpDir, err := os.MkdirTemp("", "tr-engine-test-config-file")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configContent := `
server:
  host: "127.0.0.1"
  port: 9090

database:
  host: "db-server"
  port: 5433
  name: "custom_db"

mqtt:
  broker: "tcp://mqtt:1883"
  client_id: "test-client"

logging:
  level: "debug"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "db-server", cfg.Database.Host)
	assert.Equal(t, 5433, cfg.Database.Port)
	assert.Equal(t, "custom_db", cfg.Database.Name)
	assert.Equal(t, "tcp://mqtt:1883", cfg.MQTT.Broker)
	assert.Equal(t, "test-client", cfg.MQTT.ClientID)
	assert.Equal(t, "debug", cfg.Logging.Level)
}

func TestLoad_EnvOverrides(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "tr-engine-test-config-env")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an empty config file
	configPath := filepath.Join(tmpDir, "empty.yaml")
	err = os.WriteFile(configPath, []byte(""), 0644)
	require.NoError(t, err)

	// Set environment variables
	os.Setenv("TR_ENGINE_SERVER_PORT", "9091")
	os.Setenv("DB_HOST", "env-db-host")
	os.Setenv("DB_PORT", "5434")
	os.Setenv("MQTT_BROKER", "tcp://env-mqtt:1883")
	defer func() {
		os.Unsetenv("TR_ENGINE_SERVER_PORT")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("MQTT_BROKER")
	}()

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, 9091, cfg.Server.Port)
	assert.Equal(t, "env-db-host", cfg.Database.Host)
	assert.Equal(t, 5434, cfg.Database.Port)
	assert.Equal(t, "tcp://env-mqtt:1883", cfg.MQTT.Broker)
}

func TestLoad_EnvExpansion(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "tr-engine-test-config-expansion")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set environment variable for password
	os.Setenv("MY_SECRET_PASS", "supersecret")
	defer os.Unsetenv("MY_SECRET_PASS")

	configContent := `
database:
  password: "${MY_SECRET_PASS}"
mqtt:
  password: "$MY_SECRET_PASS"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "supersecret", cfg.Database.Password)
	assert.Equal(t, "supersecret", cfg.MQTT.Password)
}

func TestConnectionString(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "password",
		Name:     "db",
		SSLMode:  "disable",
	}

	expected := "postgres://user:password@localhost:5432/db?sslmode=disable"
	assert.Equal(t, expected, cfg.ConnectionString())
}
