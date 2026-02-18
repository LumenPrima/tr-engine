package trconfig

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// TRConfig represents trunk-recorder's config.json (fields we care about).
type TRConfig struct {
	CaptureDir string     `json:"captureDir"`
	Systems    []TRSystem `json:"systems"`
}

// TRSystem is a system entry in trunk-recorder's config.
type TRSystem struct {
	ShortName      string `json:"shortName"`
	Type           string `json:"type"`
	TalkgroupsFile string `json:"talkgroupsFile"`
}

// LoadConfig reads and parses a trunk-recorder config.json file.
func LoadConfig(path string) (*TRConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg TRConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.CaptureDir == "" {
		return nil, fmt.Errorf("%s: captureDir is empty or missing", path)
	}

	return &cfg, nil
}

// VolumeMap translates container paths to host paths using Docker volume mappings.
type VolumeMap struct {
	mappings []volumeMapping // sorted longest container path first
	baseDir  string          // for resolving relative host paths
}

type volumeMapping struct {
	hostPath      string
	containerPath string
}

// LoadVolumeMap parses a docker-compose.yaml file and extracts volume mappings.
// It uses simple line parsing — no YAML library needed since we only need the
// volumes entries which are simple "- host:container" lines.
func LoadVolumeMap(composePath, baseDir string) (*VolumeMap, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}

	vm := &VolumeMap{baseDir: baseDir}
	lines := strings.Split(string(data), "\n")

	inVolumes := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect volumes: section (any indentation level)
		if trimmed == "volumes:" {
			inVolumes = true
			continue
		}

		// If we're in volumes and hit a non-list item, we're done
		if inVolumes {
			if !strings.HasPrefix(trimmed, "- ") {
				inVolumes = false
				continue
			}

			// Parse "- host:container" or "- host:container:options"
			entry := strings.TrimPrefix(trimmed, "- ")
			// Handle entries with colons carefully:
			// ./path:/container:ro or /host/path:/container/path
			// Windows paths shouldn't appear in docker-compose (Linux containers)
			parts := strings.SplitN(entry, ":", 3)
			if len(parts) >= 2 {
				hostPath := parts[0]
				containerPath := parts[1]
				// Clean trailing slashes for consistent matching
				containerPath = strings.TrimRight(containerPath, "/")
				vm.mappings = append(vm.mappings, volumeMapping{
					hostPath:      hostPath,
					containerPath: containerPath,
				})
			}
		}
	}

	// Sort longest container path first for best prefix matching
	sort.Slice(vm.mappings, func(i, j int) bool {
		return len(vm.mappings[i].containerPath) > len(vm.mappings[j].containerPath)
	})

	return vm, nil
}

// Translate converts a container path to a host path using the volume mappings.
// If no mapping matches, the path is returned unchanged.
func (vm *VolumeMap) Translate(containerPath string) string {
	if vm == nil {
		return containerPath
	}

	cleanPath := strings.TrimRight(containerPath, "/")
	for _, m := range vm.mappings {
		if cleanPath == m.containerPath || strings.HasPrefix(cleanPath, m.containerPath+"/") {
			hostPath := m.hostPath + cleanPath[len(m.containerPath):]
			// Resolve relative paths against baseDir
			if !filepath.IsAbs(hostPath) {
				hostPath = filepath.Join(vm.baseDir, hostPath)
			}
			return hostPath
		}
	}
	return containerPath
}

// TalkgroupEntry is a parsed row from trunk-recorder's talkgroup CSV file.
type TalkgroupEntry struct {
	Tgid        int
	AlphaTag    string
	Mode        string
	Description string
	Tag         string
	Category    string
	Priority    int
}

// LoadTalkgroupCSV reads a trunk-recorder talkgroup CSV file.
// It uses header-aware parsing so column order and optional columns don't matter.
func LoadTalkgroupCSV(path string) ([]TalkgroupEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return ParseTalkgroupCSV(f)
}

// ParseTalkgroupCSV parses trunk-recorder talkgroup CSV data from a reader.
// Header-aware: matches columns by name, not position.
func ParseTalkgroupCSV(reader io.Reader) ([]TalkgroupEntry, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	// Read header row
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	// Build column index map (case-insensitive, trimmed)
	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Require at minimum the Decimal column
	decIdx, ok := colIdx["decimal"]
	if !ok {
		return nil, fmt.Errorf("missing required 'Decimal' column in header")
	}

	var entries []TalkgroupEntry
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		// Parse tgid
		if decIdx >= len(record) {
			continue
		}
		tgid, err := strconv.Atoi(strings.TrimSpace(record[decIdx]))
		if err != nil || tgid <= 0 {
			continue
		}

		entry := TalkgroupEntry{Tgid: tgid}

		if idx, ok := colIdx["alpha tag"]; ok && idx < len(record) {
			entry.AlphaTag = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["mode"]; ok && idx < len(record) {
			entry.Mode = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["description"]; ok && idx < len(record) {
			entry.Description = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["tag"]; ok && idx < len(record) {
			entry.Tag = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["category"]; ok && idx < len(record) {
			entry.Category = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["priority"]; ok && idx < len(record) {
			entry.Priority, _ = strconv.Atoi(strings.TrimSpace(record[idx]))
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
