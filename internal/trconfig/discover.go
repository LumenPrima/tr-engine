package trconfig

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// DiscoveryResult contains paths and metadata discovered from trunk-recorder's config.
type DiscoveryResult struct {
	CaptureDir string             // host path to TR's audio output directory
	Systems    []DiscoveredSystem // systems found in config.json
}

// DiscoveredSystem is a system from TR's config with its parsed talkgroup CSV.
type DiscoveredSystem struct {
	ShortName  string
	Type       string
	Talkgroups []TalkgroupEntry
	CSVPath    string // host path to the talkgroup CSV file (empty if none)
	Units      []UnitEntry
	UnitCSVPath string // host path to the unit tags CSV file (empty if none)
}

// Discover reads trunk-recorder's config.json and optionally docker-compose.yaml
// from the given directory, translates container paths to host paths, and loads
// talkgroup CSV files.
func Discover(trDir string, log zerolog.Logger) (*DiscoveryResult, error) {
	// Load config.json
	configPath := filepath.Join(trDir, "config.json")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("capture_dir", cfg.CaptureDir).
		Int("systems", len(cfg.Systems)).
		Msg("loaded trunk-recorder config")

	// Try to load docker-compose volume mappings for path translation
	var vm *VolumeMap
	for _, name := range []string{"docker-compose.yaml", "docker-compose.yml"} {
		composePath := filepath.Join(trDir, name)
		if _, statErr := os.Stat(composePath); statErr == nil {
			vm, err = LoadVolumeMap(composePath, trDir)
			if err != nil {
				log.Warn().Err(err).Str("path", composePath).Msg("failed to parse docker-compose volumes")
			} else {
				log.Info().
					Str("path", composePath).
					Int("mappings", len(vm.mappings)).
					Msg("loaded docker-compose volume mappings")
			}
			break
		}
	}

	// Translate captureDir
	captureDir := vm.Translate(cfg.CaptureDir)
	log.Info().
		Str("container_path", cfg.CaptureDir).
		Str("host_path", captureDir).
		Msg("resolved capture directory")

	// Process each system
	result := &DiscoveryResult{
		CaptureDir: captureDir,
	}

	for _, sys := range cfg.Systems {
		ds := DiscoveredSystem{
			ShortName: sys.ShortName,
			Type:      sys.Type,
		}

		if sys.TalkgroupsFile != "" {
			tgPath := vm.Translate(sys.TalkgroupsFile)
			tgs, skipped, tgErr := LoadTalkgroupCSV(tgPath)
			if tgErr != nil {
				log.Warn().Err(tgErr).
					Str("system", sys.ShortName).
					Str("path", tgPath).
					Msg("failed to load talkgroup CSV")
			} else {
				ds.Talkgroups = tgs
				ds.CSVPath = tgPath
				ev := log.Info().
					Str("system", sys.ShortName).
					Int("talkgroups", len(tgs)).
					Str("path", tgPath)
				if skipped > 0 {
					ev = ev.Int("skipped", skipped)
				}
				ev.Msg("loaded talkgroup CSV")
				if skipped > 0 {
					log.Warn().
						Str("system", sys.ShortName).
						Int("skipped", skipped).
						Str("path", tgPath).
						Msg("CSV rows skipped (malformed, missing/invalid Decimal, or tgid <= 0)")
				}
			}
		}

		if sys.UnitTagsFile != "" {
			unitPath := vm.Translate(sys.UnitTagsFile)
			units, unitErr := LoadUnitCSV(unitPath)
			if unitErr != nil {
				log.Warn().Err(unitErr).
					Str("system", sys.ShortName).
					Str("path", unitPath).
					Msg("failed to load unit tags CSV")
			} else {
				ds.Units = units
				ds.UnitCSVPath = unitPath
				log.Info().
					Str("system", sys.ShortName).
					Int("units", len(units)).
					Str("path", unitPath).
					Msg("loaded unit tags CSV")
			}
		}

		result.Systems = append(result.Systems, ds)
	}

	return result, nil
}
