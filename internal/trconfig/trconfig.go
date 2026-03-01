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
	UnitTagsFile   string `json:"unitTagsFile"`
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

// UpdateTalkgroupCSV reads a trunk-recorder talkgroup CSV file, updates the Alpha Tag
// for the row matching tgid in the Decimal column, and writes the file back.
// Returns an error if the file doesn't exist or the tgid is not found.
func UpdateTalkgroupCSV(path string, tgid int, alphaTag string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	r := csv.NewReader(strings.NewReader(string(data)))
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("%s: empty CSV", path)
	}

	// Find column indexes from header
	header := records[0]
	decIdx := -1
	tagIdx := -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "decimal":
			decIdx = i
		case "alpha tag":
			tagIdx = i
		}
	}
	if decIdx == -1 {
		return fmt.Errorf("%s: missing 'Decimal' column", path)
	}
	if tagIdx == -1 {
		return fmt.Errorf("%s: missing 'Alpha Tag' column", path)
	}

	// Find and update the matching row
	found := false
	tgidStr := strconv.Itoa(tgid)
	for i := 1; i < len(records); i++ {
		if decIdx < len(records[i]) && strings.TrimSpace(records[i][decIdx]) == tgidStr {
			if tagIdx < len(records[i]) {
				records[i][tagIdx] = alphaTag
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s: tgid %d not found", path, tgid)
	}

	// Write back
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(records); err != nil {
		return fmt.Errorf("encode CSV: %w", err)
	}

	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
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
func LoadTalkgroupCSV(path string) (*CSVParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return ParseTalkgroupCSVDetailed(f)
}

// UnitEntry is a parsed row from trunk-recorder's unit tags CSV file.
type UnitEntry struct {
	UnitID   int
	AlphaTag string
}

// LoadUnitCSV reads a trunk-recorder unit tags CSV file (two-column, headerless: unit_id,alpha_tag).
// Skips the first row if the first column is non-numeric (header row).
func LoadUnitCSV(path string) ([]UnitEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // allow variable fields

	var entries []UnitEntry
	first := true
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}
		if len(record) < 2 {
			continue
		}

		unitID, parseErr := strconv.Atoi(strings.TrimSpace(record[0]))
		if parseErr != nil {
			if first {
				first = false
				continue // skip header row
			}
			continue
		}
		first = false

		entries = append(entries, UnitEntry{
			UnitID:   unitID,
			AlphaTag: strings.TrimSpace(record[1]),
		})
	}

	return entries, nil
}

// UpdateUnitCSV updates or appends a unit's alpha_tag in a TR unit tags CSV file.
// If the file doesn't exist, it creates it with a single row.
func UpdateUnitCSV(path string, unitID int, alphaTag string) error {
	unitIDStr := strconv.Itoa(unitID)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file with single row
			var buf strings.Builder
			w := csv.NewWriter(&buf)
			w.Write([]string{unitIDStr, alphaTag})
			w.Flush()
			return os.WriteFile(path, []byte(buf.String()), 0644)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	r := csv.NewReader(strings.NewReader(string(data)))
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Find and update the matching row
	found := false
	for i, record := range records {
		if len(record) >= 2 && strings.TrimSpace(record[0]) == unitIDStr {
			records[i][1] = alphaTag
			found = true
			break
		}
	}

	if !found {
		records = append(records, []string{unitIDStr, alphaTag})
	}

	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(records); err != nil {
		return fmt.Errorf("encode CSV: %w", err)
	}

	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// CSVParseResult holds the result of parsing a talkgroup CSV.
type CSVParseResult struct {
	Entries    []TalkgroupEntry
	Skipped    int // malformed/invalid rows
	Duplicates int // rows with a tgid already seen earlier in the file
}

// ParseTalkgroupCSV parses trunk-recorder talkgroup CSV data from a reader.
// Header-aware: matches columns by name, not position.
// Returns (entries, skippedRows, error). Rows are skipped for malformed CSV,
// missing/invalid Decimal column, or tgid <= 0.
func ParseTalkgroupCSV(reader io.Reader) ([]TalkgroupEntry, int, error) {
	result, err := ParseTalkgroupCSVDetailed(reader)
	if err != nil {
		return nil, 0, err
	}
	return result.Entries, result.Skipped, nil
}

// ParseTalkgroupCSVDetailed parses trunk-recorder talkgroup CSV data and returns
// detailed results including duplicate tgid counts.
func ParseTalkgroupCSVDetailed(reader io.Reader) (*CSVParseResult, error) {
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
	seen := make(map[int]bool)
	result := &CSVParseResult{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Skipped++
			continue
		}

		// Parse tgid
		if decIdx >= len(record) {
			result.Skipped++
			continue
		}
		tgid, err := strconv.Atoi(strings.TrimSpace(record[decIdx]))
		if err != nil || tgid <= 0 {
			result.Skipped++
			continue
		}

		if seen[tgid] {
			result.Duplicates++
		}
		seen[tgid] = true

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

	result.Entries = entries
	return result, nil
}
