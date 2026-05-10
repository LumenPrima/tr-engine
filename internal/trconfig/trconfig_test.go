package trconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	t.Run("parses valid config.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfgFile := filepath.Join(dir, "config.json")

		cfg := `{
			"captureDir": "/var/TR/audio",
			"systems": [
				{
					"shortName": "butco",
					"type": "P25",
					"talkgroupsFile": "talkgroups.tsv",
					"unitTagsFile": "unitTags.tsv"
				}
			]
		}`
		os.WriteFile(cfgFile, []byte(cfg), 0644)

		cfgParsed, err := LoadConfig(cfgFile)
		if err != nil {
			t.Fatalf("LoadConfig() = %v", err)
		}
		if cfgParsed.CaptureDir != "/var/TR/audio" {
			t.Errorf("captureDir = %q, want %q", cfgParsed.CaptureDir, "/var/TR/audio")
		}
		if len(cfgParsed.Systems) != 1 {
			t.Fatalf("systems count = %d, want 1", len(cfgParsed.Systems))
		}
		if cfgParsed.Systems[0].ShortName != "butco" {
			t.Errorf("system shortName = %q, want %q", cfgParsed.Systems[0].ShortName, "butco")
		}
	})

	t.Run("empty captureDir returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfgFile := filepath.Join(dir, "config.json")

		cfg := `{"captureDir": "", "systems": []}`
		os.WriteFile(cfgFile, []byte(cfg), 0644)

		_, err := LoadConfig(cfgFile)
		if err == nil {
			t.Error("LoadConfig() should error for empty captureDir")
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		t.Parallel()
		_, err := LoadConfig("/nonexistent/config.json")
		if err == nil {
			t.Error("LoadConfig() should error for missing file")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfgFile := filepath.Join(dir, "config.json")

		os.WriteFile(cfgFile, []byte("{not valid json"), 0644)

		_, err := LoadConfig(cfgFile)
		if err == nil {
			t.Error("LoadConfig() should error for invalid JSON")
		}
	})

	t.Run("multiple systems parsed", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfgFile := filepath.Join(dir, "config.json")

		cfg := `{
			"captureDir": "/var/audio",
			"systems": [
				{"shortName": "butco", "type": "P25", "talkgroupsFile": "a.tsv"},
				{"shortName": "warco", "type": "P25", "talkgroupsFile": "b.tsv"},
				{"shortName": "conv", "type": "Conventional", "talkgroupsFile": ""}
			]
		}`
		os.WriteFile(cfgFile, []byte(cfg), 0644)

		cfgParsed, err := LoadConfig(cfgFile)
		if err != nil {
			t.Fatalf("LoadConfig() = %v", err)
		}
		if len(cfgParsed.Systems) != 3 {
			t.Fatalf("systems count = %d, want 3", len(cfgParsed.Systems))
		}
		if cfgParsed.Systems[2].Type != "Conventional" {
			t.Errorf("system 2 type = %q, want %q", cfgParsed.Systems[2].Type, "Conventional")
		}
	})
}

func TestLoadVolumeMap(t *testing.T) {
	t.Parallel()

	t.Run("parses simple docker-compose volumes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		composeFile := filepath.Join(dir, "docker-compose.yml")

		compose := `services:
  tr:
    image: robotastic/trunk-recorder
    volumes:
      - ./data:/var/TR
      - ./audio:/var/TR/audio
`
		os.WriteFile(composeFile, []byte(compose), 0644)

		vm, err := LoadVolumeMap(composeFile, dir)
		if err != nil {
			t.Fatalf("LoadVolumeMap() = %v", err)
		}
		result := vm.Translate("/var/TR/audio/something.wav")
		if result != filepath.Join(dir, "audio", "something.wav") {
			t.Errorf("Translate() = %q, want %q", result, filepath.Join(dir, "audio", "something.wav"))
		}
	})

	t.Run("nonexistent compose file returns error", func(t *testing.T) {
		t.Parallel()
		_, err := LoadVolumeMap("/nonexistent/docker-compose.yml", "/tmp")
		if err == nil {
			t.Error("LoadVolumeMap() should error for missing file")
		}
	})

	t.Run("empty volumes list is valid", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		composeFile := filepath.Join(dir, "docker-compose.yml")

		compose := `services:
  tr:
    image: robotastic/trunk-recorder
`
		os.WriteFile(composeFile, []byte(compose), 0644)

		_, err := LoadVolumeMap(composeFile, dir)
		if err != nil {
			t.Errorf("LoadVolumeMap() = %v for valid empty compose", err)
		}
	})
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	t.Run("discovers systems from config", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cfgFile := filepath.Join(dir, "config.json")
		cfg := `{
			"captureDir": "/data/audio",
			"systems": [
				{
					"shortName": "butco",
					"type": "P25",
					"talkgroupsFile": "talkgroups.tsv",
					"unitTagsFile": "unitTags.tsv"
				}
			]
		}`
		os.WriteFile(cfgFile, []byte(cfg), 0644)

		result, err := Discover(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("Discover() = %v", err)
		}
		if len(result.Systems) != 1 {
			t.Fatalf("systems count = %d, want 1", len(result.Systems))
		}
		if result.Systems[0].ShortName != "butco" {
			t.Errorf("system 0 shortName = %q, want %q", result.Systems[0].ShortName, "butco")
		}
		if result.CaptureDir != "/data/audio" {
			t.Errorf("captureDir = %q, want %q", result.CaptureDir, "/data/audio")
		}
	})
}
