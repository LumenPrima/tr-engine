# Call Data Export/Import Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add call data (calls, transcriptions, audio files) to the existing metadata export/import CLI, enabling full instance migration and backup/restore.

**Architecture:** Extends the Phase 1 `internal/export/` package with new JSONL record types (`CallRecord`, `TranscriptionRecord`), bulk export queries, and staged import with fuzzy dedup. Audio files are streamed directly into/from the tar.gz archive. CLI gets `--mode metadata|full`, `--include-audio`, `--start`/`--end` flags.

**Tech Stack:** Same as Phase 1 — Go standard library `archive/tar`, `compress/gzip`, `encoding/json`. No new dependencies.

---

### Task 1: Add call/transcription record types and update manifest

**Files:**
- Modify: `internal/export/types.go`

**Step 1: Add new types and update existing ones**

Add `TimeRange`, `CallRecord`, `TranscriptionRecord` types. Update `ManifestFilters` with `TimeRange` and update `ManifestCounts` with call/transcription/audio counts.

```go
// Add to ManifestFilters:
type TimeRange struct {
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// Update ManifestFilters to add TimeRange field:
// TimeRange  *TimeRange `json:"time_range,omitempty"`

// Update ManifestCounts to add:
// Calls              int `json:"calls"`
// Transcriptions     int `json:"transcriptions"`
// AudioFiles         int `json:"audio_files"`
```

Add after `UnitRecord`:

```go
// CallRecord is a JSONL record for a call.
type CallRecord struct {
	V              int              `json:"_v"`
	SystemRef      SystemRef        `json:"system_ref"`
	SiteRef        *SiteRef         `json:"site_ref,omitempty"`
	Tgid           int              `json:"tgid"`
	StartTime      time.Time        `json:"start_time"`
	StopTime       *time.Time       `json:"stop_time,omitempty"`
	Duration       *float32         `json:"duration,omitempty"`
	Freq           *int64           `json:"freq,omitempty"`
	FreqError      *int             `json:"freq_error,omitempty"`
	SignalDB       *float32         `json:"signal_db,omitempty"`
	NoiseDB        *float32         `json:"noise_db,omitempty"`
	ErrorCount     *int             `json:"error_count,omitempty"`
	SpikeCount     *int             `json:"spike_count,omitempty"`
	AudioType      string           `json:"audio_type,omitempty"`
	AudioFilePath  string           `json:"audio_file_path,omitempty"`
	AudioFileSize  *int             `json:"audio_file_size,omitempty"`
	Phase2TDMA     bool             `json:"phase2_tdma,omitempty"`
	TDMASlot       *int16           `json:"tdma_slot,omitempty"`
	Analog         bool             `json:"analog,omitempty"`
	Conventional   bool             `json:"conventional,omitempty"`
	Encrypted      bool             `json:"encrypted,omitempty"`
	Emergency      bool             `json:"emergency,omitempty"`
	PatchedTgids   []int            `json:"patched_tgids,omitempty"`
	SrcList        json.RawMessage  `json:"src_list,omitempty"`
	FreqList       json.RawMessage  `json:"freq_list,omitempty"`
	UnitIDs        []int            `json:"unit_ids,omitempty"`
	MetadataJSON   json.RawMessage  `json:"metadata_json,omitempty"`
	IncidentData   json.RawMessage  `json:"incident_data,omitempty"`
	InstanceID     string           `json:"instance_id,omitempty"`
}

// TranscriptionRecord is a JSONL record for a transcription.
type TranscriptionRecord struct {
	V              int              `json:"_v"`
	SystemRef      SystemRef        `json:"system_ref"`
	Tgid           int              `json:"tgid"`
	CallStartTime  time.Time        `json:"call_start_time"`
	Text           string           `json:"text,omitempty"`
	Source         string           `json:"source"`
	IsPrimary      bool             `json:"is_primary,omitempty"`
	Confidence     *float32         `json:"confidence,omitempty"`
	Language       string           `json:"language,omitempty"`
	Model          string           `json:"model,omitempty"`
	Provider       string           `json:"provider,omitempty"`
	WordCount      int              `json:"word_count,omitempty"`
	DurationMs     int              `json:"duration_ms,omitempty"`
	ProviderMs     *int             `json:"provider_ms,omitempty"`
	Words          json.RawMessage  `json:"words,omitempty"`
}
```

Note: `CallRecord` needs `import "encoding/json"` added to the types.go import list.

**Step 2: Run build**

Run: `cd /c/Users/drewm/tr-engine && go build ./internal/export/`
Expected: PASS

**Step 3: Commit**

```
feat(export): add call and transcription JSONL record types
```

---

### Task 2: Add type round-trip tests

**Files:**
- Modify: `internal/export/types_test.go`

**Step 1: Add tests**

```go
func TestCallRecord_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dur := float32(45.2)
	freq := int64(851250000)
	rec := CallRecord{
		V:         1,
		SystemRef: SystemRef{Sysid: "348", Wacn: "BEE00"},
		SiteRef:   &SiteRef{InstanceID: "tr-1", ShortName: "butco"},
		Tgid:      24513,
		StartTime: now,
		Duration:  &dur,
		Freq:      &freq,
		Emergency: true,
		SrcList:   json.RawMessage(`[{"src":12345}]`),
		FreqList:  json.RawMessage(`[{"freq":851250000}]`),
		UnitIDs:   []int{12345, 67890},
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CallRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Tgid != 24513 || !decoded.Emergency || decoded.SiteRef == nil {
		t.Errorf("call round-trip failed: %+v", decoded)
	}
	if decoded.SiteRef.ShortName != "butco" {
		t.Errorf("site_ref not preserved")
	}
	if len(decoded.UnitIDs) != 2 {
		t.Errorf("unit_ids not preserved")
	}
}

func TestTranscriptionRecord_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	conf := float32(0.92)
	rec := TranscriptionRecord{
		V:             1,
		SystemRef:     SystemRef{Sysid: "348", Wacn: "BEE00"},
		Tgid:          24513,
		CallStartTime: now,
		Text:          "Engine 5 responding",
		Source:        "auto",
		IsPrimary:     true,
		Confidence:    &conf,
		Language:      "en",
		Model:         "whisper-large-v3-turbo",
		WordCount:     3,
	}
	data, _ := json.Marshal(rec)
	var decoded TranscriptionRecord
	json.Unmarshal(data, &decoded)
	if decoded.Source != "auto" || decoded.Text != "Engine 5 responding" {
		t.Errorf("transcription round-trip failed")
	}
	if decoded.Confidence == nil || *decoded.Confidence != 0.92 {
		t.Errorf("confidence not preserved")
	}
}
```

**Step 2: Run tests**

Run: `cd /c/Users/drewm/tr-engine && go test ./internal/export/ -v -run "TestCallRecord|TestTranscription" -count=1`
Expected: PASS

**Step 3: Commit**

```
test(export): add call and transcription record round-trip tests
```

---

### Task 3: Add bulk export queries for calls and transcriptions

**Files:**
- Modify: `internal/database/calls.go`
- Modify: `internal/database/transcriptions.go`

**Dependencies:** None (can be done in parallel with Task 1)

**Step 1: Add CallExport type and ExportCalls query**

Add to `internal/database/calls.go`:

```go
// CallExport contains all non-derived call fields for export.
type CallExport struct {
	SystemID       int
	SiteID         *int
	Tgid           int
	StartTime      time.Time
	StopTime       *time.Time
	Duration       *float32
	Freq           *int64
	FreqError      *int
	SignalDB       *float32
	NoiseDB        *float32
	ErrorCount     *int
	SpikeCount     *int
	AudioType      string
	AudioFilePath  string
	AudioFileSize  *int
	Phase2TDMA     bool
	TDMASlot       *int16
	Analog         bool
	Conventional   bool
	Encrypted      bool
	Emergency      bool
	PatchedTgids   []int32
	SrcList        json.RawMessage
	FreqList       json.RawMessage
	UnitIDs        []int32
	MetadataJSON   json.RawMessage
	IncidentData   json.RawMessage
	InstanceID     string
}

// ExportCalls returns all calls for the given systems and optional time range.
func (db *DB) ExportCalls(ctx context.Context, systemIDs []int, start, end *time.Time) ([]CallExport, error) {
	query := `
		SELECT system_id, site_id, tgid, start_time, stop_time, duration,
			freq, freq_error, signal_db, noise_db, error_count, spike_count,
			COALESCE(audio_type, ''), COALESCE(audio_file_path, ''),
			audio_file_size, phase2_tdma, tdma_slot,
			analog, conventional, encrypted, emergency,
			patched_tgids, src_list, freq_list, unit_ids,
			metadata_json, incidentdata, COALESCE(instance_id, '')
		FROM calls
		WHERE ($1::int[] IS NULL OR system_id = ANY($1))
		  AND ($2::timestamptz IS NULL OR start_time >= $2)
		  AND ($3::timestamptz IS NULL OR start_time < $3)
		ORDER BY start_time ASC
	`
	var startArg, endArg any
	if start != nil {
		startArg = *start
	}
	if end != nil {
		endArg = *end
	}

	rows, err := db.Pool.Query(ctx, query, pqIntArray(systemIDs), startArg, endArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CallExport
	for rows.Next() {
		var c CallExport
		if err := rows.Scan(
			&c.SystemID, &c.SiteID, &c.Tgid, &c.StartTime, &c.StopTime, &c.Duration,
			&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
			&c.AudioType, &c.AudioFilePath,
			&c.AudioFileSize, &c.Phase2TDMA, &c.TDMASlot,
			&c.Analog, &c.Conventional, &c.Encrypted, &c.Emergency,
			&c.PatchedTgids, &c.SrcList, &c.FreqList, &c.UnitIDs,
			&c.MetadataJSON, &c.IncidentData, &c.InstanceID,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
```

**Step 2: Add TranscriptionExport type and ExportTranscriptions query**

Add to `internal/database/transcriptions.go`:

```go
// TranscriptionExport contains fields needed for export.
type TranscriptionExport struct {
	SystemID      int
	Tgid          int
	CallStartTime time.Time
	Text          string
	Source        string
	IsPrimary     bool
	Confidence    *float32
	Language      string
	Model         string
	Provider      string
	WordCount     int
	DurationMs    int
	ProviderMs    *int
	Words         json.RawMessage
}

// ExportTranscriptions returns all transcriptions for the given systems and optional time range.
func (db *DB) ExportTranscriptions(ctx context.Context, systemIDs []int, start, end *time.Time) ([]TranscriptionExport, error) {
	query := `
		SELECT c.system_id, c.tgid, t.call_start_time,
			COALESCE(t.text, ''), t.source, t.is_primary, t.confidence,
			COALESCE(t.language, ''), COALESCE(t.model, ''),
			COALESCE(t.provider, ''), COALESCE(t.word_count, 0),
			COALESCE(t.duration_ms, 0), t.provider_ms, t.words
		FROM transcriptions t
		JOIN calls c ON c.call_id = t.call_id AND c.start_time = t.call_start_time
		WHERE ($1::int[] IS NULL OR c.system_id = ANY($1))
		  AND ($2::timestamptz IS NULL OR t.call_start_time >= $2)
		  AND ($3::timestamptz IS NULL OR t.call_start_time < $3)
		ORDER BY t.call_start_time ASC, t.id ASC
	`
	var startArg, endArg any
	if start != nil {
		startArg = *start
	}
	if end != nil {
		endArg = *end
	}

	rows, err := db.Pool.Query(ctx, query, pqIntArray(systemIDs), startArg, endArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TranscriptionExport
	for rows.Next() {
		var t TranscriptionExport
		if err := rows.Scan(
			&t.SystemID, &t.Tgid, &t.CallStartTime,
			&t.Text, &t.Source, &t.IsPrimary, &t.Confidence,
			&t.Language, &t.Model, &t.Provider, &t.WordCount,
			&t.DurationMs, &t.ProviderMs, &t.Words,
		); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
```

**Step 3: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./internal/database/`
Expected: PASS

**Step 4: Commit**

```
feat(export): add bulk export queries for calls and transcriptions
```

---

### Task 4: Add site ID lookup map builder to database package

The export needs to map `site_id` → `SiteRef` for each call. The import needs `SiteRef` → `site_id`. We need a helper.

**Files:**
- Modify: `internal/database/sites.go`

**Step 1: Add SiteIDMap helper**

```go
// SiteIDMap returns a map of site_id → Site for the given systems.
func (db *DB) SiteIDMap(ctx context.Context, systemIDs []int) (map[int]Site, error) {
	sites, err := db.LoadAllSites(ctx)
	if err != nil {
		return nil, err
	}
	idSet := make(map[int]bool, len(systemIDs))
	for _, id := range systemIDs {
		idSet[id] = true
	}
	m := make(map[int]Site)
	for _, s := range sites {
		if len(systemIDs) == 0 || idSet[s.SystemID] {
			m[s.SiteID] = s
		}
	}
	return m, nil
}
```

**Step 2: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./internal/database/`
Expected: PASS

**Step 3: Commit**

```
feat(export): add SiteIDMap helper for call export/import
```

---

### Task 5: Implement full export (calls + transcriptions + audio)

**Files:**
- Modify: `internal/export/export.go`

**Dependencies:** Tasks 1, 3, 4

**Step 1: Update ExportOptions**

Add fields to `ExportOptions`:

```go
type ExportOptions struct {
	SystemIDs    []int      // filter to specific systems (empty = all)
	Version      string     // tr-engine version string
	Mode         string     // "metadata" or "full"
	IncludeAudio bool       // include audio files in archive
	Start        *time.Time // time range start for calls (optional)
	End          *time.Time // time range end for calls (optional)
	AudioDir     string     // path to audio directory on disk
}
```

**Step 2: Rename ExportMetadata to Export and add call logic**

Rename `ExportMetadata` to `Export`. When `opts.Mode == "full"`, after writing metadata JSONL files, also write `calls.jsonl`, `transcriptions.jsonl`, and optionally audio files.

The key additions after the existing units JSONL write:

```go
	if opts.Mode == "full" {
		// Load calls
		calls, err := db.ExportCalls(ctx, systemIDs, opts.Start, opts.End)
		if err != nil {
			return fmt.Errorf("load calls: %w", err)
		}

		// Build site_id → SiteRef map for call export
		siteIDMap, err := db.SiteIDMap(ctx, systemIDs)
		if err != nil {
			return fmt.Errorf("load site map: %w", err)
		}

		// Load transcriptions
		transcriptions, err := db.ExportTranscriptions(ctx, systemIDs, opts.Start, opts.End)
		if err != nil {
			return fmt.Errorf("load transcriptions: %w", err)
		}

		// Update manifest counts
		manifest.Counts.Calls = len(calls)
		manifest.Counts.Transcriptions = len(transcriptions)

		// Write calls.jsonl
		if err := writeJSONL(tw, "calls.jsonl", func(enc *json.Encoder) error {
			for _, c := range calls {
				ref, ok := sysRefMap[c.SystemID]
				if !ok {
					continue
				}
				rec := buildCallRecord(c, ref, siteIDMap)
				if err := enc.Encode(rec); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}

		// Write transcriptions.jsonl
		if err := writeJSONL(tw, "transcriptions.jsonl", func(enc *json.Encoder) error {
			for _, t := range transcriptions {
				ref, ok := sysRefMap[t.SystemID]
				if !ok {
					continue
				}
				rec := TranscriptionRecord{
					V:             1,
					SystemRef:     ref,
					Tgid:          t.Tgid,
					CallStartTime: t.CallStartTime,
					Text:          t.Text,
					Source:        t.Source,
					IsPrimary:     t.IsPrimary,
					Confidence:    t.Confidence,
					Language:      t.Language,
					Model:         t.Model,
					Provider:      t.Provider,
					WordCount:     t.WordCount,
					DurationMs:    t.DurationMs,
					ProviderMs:    t.ProviderMs,
					Words:         t.Words,
				}
				if err := enc.Encode(rec); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}

		// Write audio files (if included)
		if opts.IncludeAudio && opts.AudioDir != "" {
			audioCount := 0
			for _, c := range calls {
				if c.AudioFilePath == "" {
					continue
				}
				// Normalize path separators (Windows backslash → forward slash)
				relPath := filepath.ToSlash(c.AudioFilePath)
				diskPath := filepath.Join(opts.AudioDir, filepath.FromSlash(relPath))
				info, err := os.Stat(diskPath)
				if err != nil {
					continue // skip missing audio files
				}
				f, err := os.Open(diskPath)
				if err != nil {
					continue
				}
				hdr := &tar.Header{
					Name:    "audio/" + relPath,
					Size:    info.Size(),
					Mode:    0644,
					ModTime: info.ModTime(),
				}
				if err := tw.WriteHeader(hdr); err != nil {
					f.Close()
					return fmt.Errorf("write audio header %s: %w", relPath, err)
				}
				if _, err := io.Copy(tw, f); err != nil {
					f.Close()
					return fmt.Errorf("write audio %s: %w", relPath, err)
				}
				f.Close()
				audioCount++
			}
			manifest.Counts.AudioFiles = audioCount
		}
	}
```

Note: The manifest is written first, but we don't know call/audio counts yet. Two approaches: (A) write manifest last, or (B) write manifest first with counts computed upfront. Since we already load all calls into memory before writing any JSONL, we can compute counts before writing the manifest. Restructure the function to load all data first, compute manifest, then write everything.

Also add a `buildCallRecord` helper:

```go
func buildCallRecord(c database.CallExport, sysRef SystemRef, siteIDMap map[int]database.Site) CallRecord {
	rec := CallRecord{
		V:            1,
		SystemRef:    sysRef,
		Tgid:         c.Tgid,
		StartTime:    c.StartTime,
		StopTime:     c.StopTime,
		Duration:     c.Duration,
		Freq:         c.Freq,
		FreqError:    c.FreqError,
		SignalDB:     c.SignalDB,
		NoiseDB:      c.NoiseDB,
		ErrorCount:   c.ErrorCount,
		SpikeCount:   c.SpikeCount,
		AudioType:    c.AudioType,
		AudioFilePath: filepath.ToSlash(c.AudioFilePath),
		AudioFileSize: c.AudioFileSize,
		Phase2TDMA:   c.Phase2TDMA,
		TDMASlot:     c.TDMASlot,
		Analog:       c.Analog,
		Conventional: c.Conventional,
		Encrypted:    c.Encrypted,
		Emergency:    c.Emergency,
		SrcList:      c.SrcList,
		FreqList:     c.FreqList,
		MetadataJSON: c.MetadataJSON,
		IncidentData: c.IncidentData,
		InstanceID:   c.InstanceID,
	}
	// Convert int32 slices to int slices
	if len(c.PatchedTgids) > 0 {
		rec.PatchedTgids = make([]int, len(c.PatchedTgids))
		for i, v := range c.PatchedTgids {
			rec.PatchedTgids[i] = int(v)
		}
	}
	if len(c.UnitIDs) > 0 {
		rec.UnitIDs = make([]int, len(c.UnitIDs))
		for i, v := range c.UnitIDs {
			rec.UnitIDs[i] = int(v)
		}
	}
	// Resolve site_id → SiteRef
	if c.SiteID != nil {
		if site, ok := siteIDMap[*c.SiteID]; ok {
			rec.SiteRef = &SiteRef{
				InstanceID: site.InstanceID,
				ShortName:  site.ShortName,
			}
		}
	}
	return rec
}
```

Add `"os"`, `"io"`, `"path/filepath"` to imports.

**Step 3: Update callers**

Update `cmd/tr-engine/export.go` to call `export.Export()` instead of `export.ExportMetadata()` and pass the new options. Update `cmd/tr-engine/import.go`'s call to match the renamed function if needed (import still calls `ImportMetadata` which stays as-is for now).

**Step 4: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./...`
Expected: PASS

**Step 5: Run existing tests**

Run: `cd /c/Users/drewm/tr-engine && go test ./internal/export/ -count=1`
Expected: PASS (existing tests use the function directly, need to update any references to `ExportMetadata`)

**Step 6: Commit**

```
feat(export): implement full export with calls, transcriptions, and audio
```

---

### Task 6: Update CLI export command with new flags

**Files:**
- Modify: `cmd/tr-engine/export.go`

**Dependencies:** Task 5

**Step 1: Add new flags and wire to ExportOptions**

```go
func runExport(args []string, overrides config.Overrides) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	output := fs.String("output", "", "Output file path (required)")
	systems := fs.String("systems", "", "Comma-separated system IDs to export (default: all)")
	mode := fs.String("mode", "metadata", "Export mode: metadata, full")
	includeAudio := fs.Bool("include-audio", false, "Include audio files in archive (only with --mode full)")
	startStr := fs.String("start", "", "Start time for calls (ISO 8601, e.g. 2026-02-01)")
	endStr := fs.String("end", "", "End time for calls (ISO 8601, e.g. 2026-03-01)")
	fs.StringVar(&overrides.EnvFile, "env-file", overrides.EnvFile, "Path to .env file")
	fs.StringVar(&overrides.DatabaseURL, "database-url", overrides.DatabaseURL, "PostgreSQL connection URL")
	fs.StringVar(&overrides.AudioDir, "audio-dir", overrides.AudioDir, "Audio file directory")
	fs.Parse(args)
	// ... validation, parse start/end times, pass to ExportOptions
}
```

Parse `--start`/`--end` with `time.Parse` supporting both `2006-01-02` and RFC3339 formats. Validate that `--include-audio` is only used with `--mode full`. Increase context timeout from 10 min to 30 min for full exports.

**Step 2: Build and test CLI help**

Run: `cd /c/Users/drewm/tr-engine && go build ./cmd/tr-engine/ && ./tr-engine.exe export --help`
Expected: Shows all new flags

**Step 3: Commit**

```
feat(export): add --mode, --include-audio, --start, --end flags to export CLI
```

---

### Task 7: Implement call import

**Files:**
- Modify: `internal/export/import.go`
- Modify: `internal/database/calls.go` (add `FindCallFuzzy`)

**Dependencies:** Tasks 1, 5

**Step 1: Add FindCallFuzzy to database package**

Add to `internal/database/calls.go` — reuses the `FindCallForAudio` ±5s pattern:

```go
// FindCallFuzzy checks if a call exists matching (system_id, tgid, start_time ± 5s).
// Returns call_id and start_time if found, or 0 if not found.
func (db *DB) FindCallFuzzy(ctx context.Context, systemID, tgid int, startTime time.Time) (int64, time.Time, error) {
	return db.FindCallForAudio(ctx, systemID, tgid, startTime)
}
```

This is a thin alias so the import code reads clearly. Uses the exact same SQL as `FindCallForAudio`.

**Step 2: Add importCalls to import.go**

Add after `importUnits`:

```go
// importCalls processes calls.jsonl — dedup, create call_groups, insert calls, rebuild relational tables.
func importCalls(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, siteMap map[string]int,
	result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase scanner buffer for large call records with src_list/freq_list
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec CallRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt call record")
			result.Calls.Skip++
			continue
		}
		if rec.V != 1 {
			result.Calls.Skip++
			continue
		}

		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			result.Calls.Skip++
			continue
		}

		// Resolve site
		var siteID *int
		if rec.SiteRef != nil {
			siteKey := rec.SiteRef.InstanceID + ":" + rec.SiteRef.ShortName
			if sid, ok := siteMap[siteKey]; ok {
				siteID = &sid
			}
		}

		// Fuzzy dedup check
		if !dryRun {
			existingID, _, err := db.FindCallFuzzy(ctx, systemID, rec.Tgid, rec.StartTime)
			if err == nil && existingID > 0 {
				result.Calls.Skip++
				continue
			}
		}

		if !dryRun {
			// Look up current talkgroup info for denormalized fields
			tgAlphaTag, tgDescription, tgTag, tgGroup := "", "", "", ""
			// (populate from local talkgroup data if available)

			// Upsert call group
			cgID, err := db.UpsertCallGroup(ctx, systemID, rec.Tgid, rec.StartTime,
				tgAlphaTag, tgDescription, tgTag, tgGroup)
			if err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Time("start", rec.StartTime).Msg("failed to upsert call group")
				result.Calls.Skip++
				continue
			}

			// Build CallRow
			callRow := buildCallRowFromRecord(rec, systemID, siteID)

			// Insert call
			callID, err := db.InsertCall(ctx, callRow)
			if err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Time("start", rec.StartTime).Msg("failed to insert call")
				result.Calls.Skip++
				continue
			}

			// Set call group
			if err := db.SetCallGroupID(ctx, callID, rec.StartTime, cgID); err != nil {
				log.Warn().Err(err).Msg("failed to set call group id")
			}

			// Rebuild call_frequencies from freq_list
			if len(rec.FreqList) > 0 {
				rebuildCallFrequencies(ctx, db, callID, rec.StartTime, rec.FreqList, log)
			}
			// Rebuild call_transmissions from src_list
			if len(rec.SrcList) > 0 {
				rebuildCallTransmissions(ctx, db, callID, rec.StartTime, rec.SrcList, log)
			}
		}
		result.Calls.Update++
	}

	return scanner.Err()
}
```

Add helper functions `buildCallRowFromRecord`, `rebuildCallFrequencies`, `rebuildCallTransmissions`. The rebuild helpers parse the JSONL arrays and call the existing `InsertCallFrequencies`/`InsertCallTransmissions` batch functions.

`buildCallRowFromRecord` maps `CallRecord` fields → `database.CallRow` fields, converting `[]int` → `[]int32`, etc.

**Step 3: Wire into ImportMetadata**

Add `Calls ImportCounts` and `Transcriptions ImportCounts` to `ImportResult`. After Stage 4 (units), add:

```go
	// Stage 5: Calls (when mode is "full" or "calls")
	if opts.Mode == "full" || opts.Mode == "calls" {
		if err := importCalls(ctx, db, files["calls.jsonl"], sysMap, siteMap, result, opts.DryRun, log); err != nil {
			return result, fmt.Errorf("import calls: %w", err)
		}
	}
```

**Step 4: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./...`
Expected: PASS

**Step 5: Commit**

```
feat(export): implement call import with fuzzy dedup and call_group creation
```

---

### Task 8: Implement transcription import

**Files:**
- Modify: `internal/export/import.go`

**Dependencies:** Task 7

**Step 1: Add importTranscriptions function**

```go
// importTranscriptions processes transcriptions.jsonl.
func importTranscriptions(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int,
	result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec TranscriptionRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt transcription record")
			result.Transcriptions.Skip++
			continue
		}
		if rec.V != 1 {
			result.Transcriptions.Skip++
			continue
		}

		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			result.Transcriptions.Skip++
			continue
		}

		if !dryRun {
			// Find parent call by fuzzy match
			callID, callStartTime, err := db.FindCallFuzzy(ctx, systemID, rec.Tgid, rec.CallStartTime)
			if err != nil || callID == 0 {
				log.Debug().Int("tgid", rec.Tgid).Time("start", rec.CallStartTime).Msg("skipping transcription: parent call not found")
				result.Transcriptions.Skip++
				continue
			}

			// Insert transcription
			row := &database.TranscriptionRow{
				CallID:        callID,
				CallStartTime: callStartTime,
				Text:          rec.Text,
				Source:        rec.Source,
				IsPrimary:     rec.IsPrimary,
				Confidence:    rec.Confidence,
				Language:      rec.Language,
				Model:         rec.Model,
				Provider:      rec.Provider,
				WordCount:     rec.WordCount,
				DurationMs:    rec.DurationMs,
				ProviderMs:    rec.ProviderMs,
				Words:         rec.Words,
			}
			if _, err := db.InsertTranscription(ctx, row); err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Msg("failed to import transcription")
				result.Transcriptions.Skip++
				continue
			}
		}
		result.Transcriptions.Update++
	}

	return scanner.Err()
}
```

**Step 2: Wire into ImportMetadata after calls stage**

```go
	// Stage 6: Transcriptions
	if opts.Mode == "full" || opts.Mode == "calls" {
		if err := importTranscriptions(ctx, db, files["transcriptions.jsonl"], sysMap, result, opts.DryRun, log); err != nil {
			return result, fmt.Errorf("import transcriptions: %w", err)
		}
	}
```

**Step 3: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./...`
Expected: PASS

**Step 4: Commit**

```
feat(export): implement transcription import with fuzzy call resolution
```

---

### Task 9: Implement audio file import

**Files:**
- Modify: `internal/export/import.go`

**Dependencies:** Task 7

**Step 1: Add audio extraction to ImportMetadata**

After processing all JSONL files, if the archive contains `audio/` entries, extract them to the local audio directory. The import function needs an `AudioDir` field on `ImportOptions`.

Update `ImportOptions`:
```go
type ImportOptions struct {
	Mode     string // "full", "metadata", "calls"
	DryRun   bool
	AudioDir string // path to local audio directory
}
```

The audio extraction happens during the initial tar read pass — instead of only reading JSONL files into memory, also detect `audio/` prefixed entries and extract them to disk:

```go
	// During tar reading loop, handle audio files:
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if strings.HasPrefix(hdr.Name, "audio/") && hdr.Typeflag == tar.TypeReg {
			if opts.AudioDir != "" && !opts.DryRun {
				relPath := strings.TrimPrefix(hdr.Name, "audio/")
				destPath := filepath.Join(opts.AudioDir, filepath.FromSlash(relPath))
				// Create parent directories
				os.MkdirAll(filepath.Dir(destPath), 0755)
				// Skip if file already exists (idempotent)
				if _, err := os.Stat(destPath); err == nil {
					audioSkipped++
					continue
				}
				outFile, err := os.Create(destPath)
				if err != nil {
					log.Warn().Err(err).Str("path", relPath).Msg("failed to extract audio file")
					continue
				}
				io.Copy(outFile, tr)
				outFile.Close()
				audioExtracted++
			}
			continue
		}
		// Regular JSONL/manifest files: read into memory
		data, err := io.ReadAll(tr)
		// ...
	}
```

**Step 2: Pass AudioDir from CLI**

Update `cmd/tr-engine/import.go` to add `--audio-dir` flag and pass `cfg.AudioDir` to `ImportOptions`.

**Step 3: Build**

Run: `cd /c/Users/drewm/tr-engine && go build ./...`
Expected: PASS

**Step 4: Commit**

```
feat(export): implement audio file extraction on import
```

---

### Task 10: Update export tests

**Files:**
- Modify: `internal/export/export_test.go`

**Dependencies:** Tasks 1-5

**Step 1: Add test for buildCallRecord**

Test that `buildCallRecord` correctly maps fields, converts int32 slices, and resolves site_id to SiteRef.

**Step 2: Add test for full archive structure**

Create a test that verifies a full-mode archive (mocked — no DB) contains the expected files: manifest.json, systems.jsonl, sites.jsonl, talkgroups.jsonl, talkgroup_directory.jsonl, units.jsonl, calls.jsonl, transcriptions.jsonl.

**Step 3: Run tests**

Run: `cd /c/Users/drewm/tr-engine && go test ./internal/export/ -v -count=1`
Expected: PASS

**Step 4: Commit**

```
test(export): add call export and archive structure tests
```

---

### Task 11: Integration test with live database

**Dependencies:** All previous tasks

**Step 1: Run full export**

Run: `cd /c/Users/drewm/tr-engine && go run ./cmd/tr-engine/ export --output /tmp/test-full-export.tar.gz --mode full`
Expected: Creates archive with metadata + calls + transcriptions. Log shows counts.

**Step 2: Inspect archive**

Run: `tar tzf /tmp/test-full-export.tar.gz | head -20`
Expected: manifest.json, systems.jsonl, sites.jsonl, talkgroups.jsonl, talkgroup_directory.jsonl, units.jsonl, calls.jsonl, transcriptions.jsonl

Run: `tar xzf /tmp/test-full-export.tar.gz --to-stdout calls.jsonl | head -3`
Expected: JSONL records with `_v`, `system_ref`, `tgid`, `start_time`, `src_list`, etc.

Run: `tar xzf /tmp/test-full-export.tar.gz --to-stdout transcriptions.jsonl | head -3`
Expected: JSONL records with `_v`, `system_ref`, `tgid`, `call_start_time`, `text`, `source`

**Step 3: Dry-run import**

Run: `cd /c/Users/drewm/tr-engine && go run ./cmd/tr-engine/ import --file /tmp/test-full-export.tar.gz --mode full --dry-run`
Expected: JSON summary showing calls and transcriptions counts (all updates/skips since data exists)

**Step 4: Real import (idempotent)**

Run: `cd /c/Users/drewm/tr-engine && go run ./cmd/tr-engine/ import --file /tmp/test-full-export.tar.gz --mode full`
Expected: All calls skipped (fuzzy dedup), transcriptions may show some updates/skips

**Step 5: Time-filtered export**

Run: `cd /c/Users/drewm/tr-engine && go run ./cmd/tr-engine/ export --output /tmp/test-filtered.tar.gz --mode full --start 2026-02-20 --end 2026-02-21`
Expected: Smaller archive with only calls in that 1-day window

**Step 6: Run full test suite**

Run: `cd /c/Users/drewm/tr-engine && go test ./... -count=1`
Expected: All tests pass

**Step 7: Commit**

```
feat(export): call data export/import - phase 2 complete
```

---

## Verification

1. **Build:** `go build ./...` — all packages compile
2. **Unit tests:** `go test ./internal/export/ -v` — all tests pass
3. **Full test suite:** `go test ./...` — all packages pass
4. **Metadata export:** `tr-engine export --output test.tar.gz` — produces metadata-only archive (backward compatible)
5. **Full export:** `tr-engine export --output test.tar.gz --mode full` — includes calls + transcriptions
6. **Time-filtered export:** `tr-engine export --output test.tar.gz --mode full --start 2026-02-20 --end 2026-02-21` — subset
7. **Dry-run import:** `tr-engine import --file test.tar.gz --mode full --dry-run` — shows counts
8. **Idempotent import:** `tr-engine import --file test.tar.gz --mode full` — all calls deduped

## Critical Files

| File | Action | Purpose |
|------|--------|---------|
| `internal/export/types.go` | Modify | Add `CallRecord`, `TranscriptionRecord`, update manifest types |
| `internal/export/export.go` | Modify | Add full export (calls, transcriptions, audio) |
| `internal/export/import.go` | Modify | Add call/transcription/audio import stages |
| `internal/database/calls.go` | Modify | Add `ExportCalls`, `FindCallFuzzy` |
| `internal/database/transcriptions.go` | Modify | Add `ExportTranscriptions` |
| `internal/database/sites.go` | Modify | Add `SiteIDMap` |
| `cmd/tr-engine/export.go` | Modify | Add `--mode`, `--include-audio`, `--start`, `--end` flags |
| `cmd/tr-engine/import.go` | Modify | Add `--audio-dir` flag, pass to ImportOptions |
| `internal/export/types_test.go` | Modify | Add call/transcription round-trip tests |
| `internal/export/export_test.go` | Modify | Add call export and archive tests |

## Reusable Existing Functions

- `db.InsertCall()` — `internal/database/calls.go:88` — insert a call row
- `db.UpsertCallGroup()` — `internal/database/calls.go:219` — find/create call group
- `db.SetCallGroupID()` — `internal/database/calls.go:234` — link call to group
- `db.FindCallForAudio()` — `internal/database/calls.go:361` — fuzzy ±5s call match (reused as `FindCallFuzzy`)
- `db.InsertCallFrequencies()` — `internal/database/calls.go:275` — batch insert freq records
- `db.InsertCallTransmissions()` — `internal/database/calls.go:305` — batch insert tx records
- `db.InsertTranscription()` — `internal/database/transcriptions.go:169` — insert + update denorm fields
- `db.LoadAllSites()` — `internal/database/sites.go:195` — load all sites for ID map
- `config.Load()` — `internal/config/config.go` — reads `AUDIO_DIR` from env
