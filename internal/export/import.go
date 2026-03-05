package export

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/database"
)

// ImportResult tracks counts per entity type.
type ImportResult struct {
	Systems            ImportCounts `json:"systems"`
	Sites              ImportCounts `json:"sites"`
	Talkgroups         ImportCounts `json:"talkgroups"`
	TalkgroupDirectory ImportCounts `json:"talkgroup_directory"`
	Units              ImportCounts `json:"units"`
	Calls              ImportCounts `json:"calls"`
	Transcriptions     ImportCounts `json:"transcriptions"`
}

// ImportCounts tracks create/update/skip per entity type.
type ImportCounts struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Skip   int `json:"skip"`
}

// ImportOptions configures import behavior.
type ImportOptions struct {
	Mode     string // "full", "metadata", "calls"
	DryRun   bool
	AudioDir string // path to local audio directory for extracting audio files
}

// ImportMetadata reads a tar.gz archive and imports metadata entities.
func ImportMetadata(ctx context.Context, db *database.DB, r io.Reader, opts ImportOptions, log zerolog.Logger) (*ImportResult, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// Read files from archive. Audio files are extracted to disk (they can be
	// several GB total); all other entries are small metadata and loaded into memory.
	files := make(map[string][]byte)
	var audioExtracted, audioSkipped int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		// Extract audio files to disk instead of loading into memory
		if strings.HasPrefix(hdr.Name, "audio/") && hdr.Typeflag == tar.TypeReg {
			if opts.AudioDir != "" && !opts.DryRun {
				relPath := strings.TrimPrefix(hdr.Name, "audio/")
				destPath := filepath.Join(opts.AudioDir, filepath.FromSlash(relPath))
				// Create parent directories
				if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
					log.Warn().Err(err).Str("path", relPath).Msg("failed to create audio directory")
					continue
				}
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
				if _, err := io.Copy(outFile, tr); err != nil {
					outFile.Close()
					log.Warn().Err(err).Str("path", relPath).Msg("failed to write audio file")
					continue
				}
				outFile.Close()
				audioExtracted++
			}
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", hdr.Name, err)
		}
		files[hdr.Name] = data
	}

	// Parse and validate manifest
	manifestData, ok := files["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("archive missing manifest.json")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("unsupported archive version %d (expected 1)", manifest.Version)
	}
	if manifest.Format != "tr-engine-export" {
		return nil, fmt.Errorf("unsupported archive format %q", manifest.Format)
	}

	result := &ImportResult{}

	// Stage 1: Systems
	sysMap, err := importSystems(ctx, db, files["systems.jsonl"], result, opts.DryRun, log)
	if err != nil {
		return result, fmt.Errorf("import systems: %w", err)
	}

	// Stage 2: Sites
	siteMap, err := importSites(ctx, db, files["sites.jsonl"], sysMap, result, opts.DryRun, log)
	if err != nil {
		return result, fmt.Errorf("import sites: %w", err)
	}
	// siteMap is used in call import below

	// Stage 3: Talkgroups
	if err := importTalkgroups(ctx, db, files["talkgroups.jsonl"], sysMap, result, opts.DryRun, log); err != nil {
		return result, fmt.Errorf("import talkgroups: %w", err)
	}

	// Stage 3b: Talkgroup directory
	if err := importTalkgroupDirectory(ctx, db, files["talkgroup_directory.jsonl"], sysMap, result, opts.DryRun, log); err != nil {
		return result, fmt.Errorf("import talkgroup directory: %w", err)
	}

	// Stage 4: Units
	if err := importUnits(ctx, db, files["units.jsonl"], sysMap, result, opts.DryRun, log); err != nil {
		return result, fmt.Errorf("import units: %w", err)
	}

	// Stage 5: Calls (when mode is "full" or "calls")
	if opts.Mode == "full" || opts.Mode == "calls" {
		if err := importCalls(ctx, db, files["calls.jsonl"], sysMap, siteMap, result, opts.DryRun, log); err != nil {
			return result, fmt.Errorf("import calls: %w", err)
		}
	}

	// Stage 6: Transcriptions (when mode is "full" or "calls")
	if opts.Mode == "full" || opts.Mode == "calls" {
		if err := importTranscriptions(ctx, db, files["transcriptions.jsonl"], sysMap, result, opts.DryRun, log); err != nil {
			return result, fmt.Errorf("import transcriptions: %w", err)
		}
	}

	// Post-import: refresh stats (skip on dry run)
	if !opts.DryRun {
		if _, err := db.RefreshTalkgroupStatsHot(ctx); err != nil {
			log.Warn().Err(err).Msg("post-import: hot stats refresh failed")
		}
		if _, err := db.RefreshTalkgroupStatsCold(ctx); err != nil {
			log.Warn().Err(err).Msg("post-import: cold stats refresh failed")
		}
	}

	if audioExtracted > 0 || audioSkipped > 0 {
		log.Info().Int("extracted", audioExtracted).Int("skipped", audioSkipped).Msg("audio file import")
	}

	return result, nil
}

// resolveSystemID finds the local system_id for a SystemRef using the sysMap.
func resolveSystemID(ref SystemRef, sysMap map[string]int) (int, bool) {
	if ref.Sysid != "" && ref.Sysid != "0" {
		id, ok := sysMap["p25:"+ref.Sysid+":"+ref.Wacn]
		return id, ok
	}
	// Conventional systems are resolved via site key — check all conv: keys.
	// Caller must try conv: key fallback for their specific site.
	return 0, false
}

// importSystems processes systems.jsonl and returns systemRef -> local system_id map.
func importSystems(ctx context.Context, db *database.DB, data []byte, result *ImportResult, dryRun bool, log zerolog.Logger) (map[string]int, error) {
	sysMap := make(map[string]int)
	if data == nil {
		return sysMap, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec SystemRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt system record")
			result.Systems.Skip++
			continue
		}
		if rec.V != 1 {
			log.Warn().Int("version", rec.V).Msg("skipping system with unsupported version")
			result.Systems.Skip++
			continue
		}

		isP25 := rec.Sysid != "" && rec.Sysid != "0"

		if isP25 {
			existingID, err := db.FindSystemBySysidWacn(ctx, rec.Sysid, rec.Wacn, 0)
			if err != nil {
				return nil, fmt.Errorf("find system %s/%s: %w", rec.Sysid, rec.Wacn, err)
			}
			key := "p25:" + rec.Sysid + ":" + rec.Wacn

			if existingID > 0 {
				sysMap[key] = existingID
				result.Systems.Update++
				log.Info().Str("sysid", rec.Sysid).Str("wacn", rec.Wacn).Int("system_id", existingID).Msg("matched existing P25 system")
			} else if !dryRun {
				systemID, _, err := db.FindOrCreateSystem(ctx, "", rec.Name, rec.Type)
				if err != nil {
					return nil, fmt.Errorf("create system %q: %w", rec.Name, err)
				}
				if err := db.UpdateSystemIdentity(ctx, systemID, rec.Type, rec.Sysid, rec.Wacn, rec.Name); err != nil {
					return nil, fmt.Errorf("set identity for system %q: %w", rec.Name, err)
				}
				sysMap[key] = systemID
				result.Systems.Create++
				log.Info().Str("sysid", rec.Sysid).Int("system_id", systemID).Msg("created new P25 system")
			} else {
				result.Systems.Create++
			}
		} else {
			// Conventional: match by (instance_id, short_name) via site
			for _, siteRef := range rec.Sites {
				key := "conv:" + siteRef.InstanceID + ":" + siteRef.ShortName
				existingID, err := db.FindSystemViaSiteIdentity(ctx, siteRef.InstanceID, siteRef.ShortName)
				if err != nil {
					return nil, fmt.Errorf("find system via site %s/%s: %w", siteRef.InstanceID, siteRef.ShortName, err)
				}
				if existingID > 0 {
					sysMap[key] = existingID
					result.Systems.Update++
					log.Info().Str("instance", siteRef.InstanceID).Str("short_name", siteRef.ShortName).Msg("matched existing conventional system")
				} else if !dryRun {
					systemID, _, err := db.FindOrCreateSystem(ctx, siteRef.InstanceID, siteRef.ShortName, rec.Type)
					if err != nil {
						return nil, fmt.Errorf("create conventional system %q: %w", siteRef.ShortName, err)
					}
					sysMap[key] = systemID
					result.Systems.Create++
					log.Info().Str("short_name", siteRef.ShortName).Int("system_id", systemID).Msg("created new conventional system")
				} else {
					result.Systems.Create++
				}
			}
		}
	}

	return sysMap, scanner.Err()
}

// importSites processes sites.jsonl and returns siteRef -> local site_id map.
func importSites(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, result *ImportResult, dryRun bool, log zerolog.Logger) (map[string]int, error) {
	siteMap := make(map[string]int)
	if data == nil {
		return siteMap, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec SiteRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt site record")
			result.Sites.Skip++
			continue
		}
		if rec.V != 1 {
			result.Sites.Skip++
			continue
		}

		// Resolve parent system — try P25 key first, then conventional key
		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			convKey := "conv:" + rec.InstanceID + ":" + rec.ShortName
			systemID, ok = sysMap[convKey]
		}
		if !ok {
			log.Warn().Str("instance_id", rec.InstanceID).Str("short_name", rec.ShortName).Msg("skipping site: system not resolved")
			result.Sites.Skip++
			continue
		}

		siteKey := rec.InstanceID + ":" + rec.ShortName

		if !dryRun {
			siteID, err := db.FindOrCreateSite(ctx, systemID, rec.InstanceID, rec.ShortName)
			if err != nil {
				return nil, fmt.Errorf("upsert site %s: %w", siteKey, err)
			}
			siteMap[siteKey] = siteID
			result.Sites.Update++
			log.Debug().Str("key", siteKey).Int("site_id", siteID).Msg("imported site")
		} else {
			result.Sites.Update++
		}
	}

	return siteMap, scanner.Err()
}

// importTalkgroups processes talkgroups.jsonl.
func importTalkgroups(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec TalkgroupRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt talkgroup record")
			result.Talkgroups.Skip++
			continue
		}
		if rec.V != 1 {
			result.Talkgroups.Skip++
			continue
		}

		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			result.Talkgroups.Skip++
			continue
		}

		if !dryRun {
			if err := db.ImportUpsertTalkgroup(ctx, systemID, rec.Tgid,
				rec.AlphaTag, rec.AlphaTagSource, rec.Tag, rec.Group,
				rec.Description, rec.Mode, rec.Priority,
				rec.FirstSeen, rec.LastSeen,
			); err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Msg("failed to import talkgroup")
				result.Talkgroups.Skip++
				continue
			}
		}
		result.Talkgroups.Update++
	}

	return scanner.Err()
}

// importTalkgroupDirectory processes talkgroup_directory.jsonl.
func importTalkgroupDirectory(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec TalkgroupDirectoryRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt talkgroup directory record")
			result.TalkgroupDirectory.Skip++
			continue
		}
		if rec.V != 1 {
			result.TalkgroupDirectory.Skip++
			continue
		}

		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			result.TalkgroupDirectory.Skip++
			continue
		}

		prio := 0
		if rec.Priority != nil {
			prio = *rec.Priority
		}

		if !dryRun {
			if err := db.UpsertTalkgroupDirectory(ctx, systemID, rec.Tgid,
				rec.AlphaTag, rec.Mode, rec.Description, rec.Tag, rec.Category, prio,
			); err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Msg("failed to import talkgroup directory entry")
				result.TalkgroupDirectory.Skip++
				continue
			}
		}
		result.TalkgroupDirectory.Update++
	}

	return scanner.Err()
}

// importUnits processes units.jsonl.
func importUnits(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec UnitRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			log.Warn().Err(err).Msg("skipping corrupt unit record")
			result.Units.Skip++
			continue
		}
		if rec.V != 1 {
			result.Units.Skip++
			continue
		}

		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok {
			result.Units.Skip++
			continue
		}

		if !dryRun {
			if err := db.ImportUpsertUnit(ctx, systemID, rec.UnitID,
				rec.AlphaTag, rec.AlphaTagSource, rec.FirstSeen, rec.LastSeen,
			); err != nil {
				log.Warn().Err(err).Int("unit_id", rec.UnitID).Msg("failed to import unit")
				result.Units.Skip++
				continue
			}
		}
		result.Units.Update++
	}

	return scanner.Err()
}

// importCalls processes calls.jsonl — dedup, create call_groups, insert calls, rebuild relational tables.
func importCalls(ctx context.Context, db *database.DB, data []byte, sysMap map[string]int, siteMap map[string]int,
	result *ImportResult, dryRun bool, log zerolog.Logger) error {
	if data == nil {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase buffer for large call records with embedded src_list/freq_list JSONB
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

		// Resolve system — try P25 key first, then conventional via site ref
		systemID, ok := resolveSystemID(rec.SystemRef, sysMap)
		if !ok && rec.SiteRef != nil {
			convKey := "conv:" + rec.SiteRef.InstanceID + ":" + rec.SiteRef.ShortName
			systemID, ok = sysMap[convKey]
		}
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

		if !dryRun {
			// Fuzzy dedup check: skip if call already exists within ±5s
			existingID, _, err := db.FindCallFuzzy(ctx, systemID, rec.Tgid, rec.StartTime)
			if err == nil && existingID > 0 {
				result.Calls.Skip++
				continue
			}

			// Upsert call group (uses exact match on system_id, tgid, start_time)
			cgID, err := db.UpsertCallGroup(ctx, systemID, rec.Tgid, rec.StartTime, "", "", "", "")
			if err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Msg("failed to upsert call group")
				result.Calls.Skip++
				continue
			}

			// Build CallRow from record
			callRow := buildCallRowFromRecord(rec, systemID, siteID)

			// Insert call
			callID, err := db.InsertCall(ctx, callRow)
			if err != nil {
				log.Warn().Err(err).Int("tgid", rec.Tgid).Msg("failed to insert call")
				result.Calls.Skip++
				continue
			}

			// Link call to group
			if err := db.SetCallGroupID(ctx, callID, rec.StartTime, cgID); err != nil {
				log.Warn().Err(err).Msg("failed to set call group id")
			}

			// Set audio file path if present
			if rec.AudioFilePath != "" {
				audioSize := 0
				if rec.AudioFileSize != nil {
					audioSize = *rec.AudioFileSize
				}
				if err := db.UpdateCallAudio(ctx, callID, rec.StartTime, rec.AudioFilePath, audioSize); err != nil {
					log.Warn().Err(err).Msg("failed to set audio path")
				}
			}

			// Rebuild call_frequencies from freq_list JSONB
			if len(rec.FreqList) > 0 {
				rebuildCallFrequencies(ctx, db, callID, rec.StartTime, rec.FreqList, log)
			}

			// Rebuild call_transmissions from src_list JSONB
			if len(rec.SrcList) > 0 {
				rebuildCallTransmissions(ctx, db, callID, rec.StartTime, rec.SrcList, log)
			}
		}
		result.Calls.Update++
	}

	return scanner.Err()
}

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

// buildCallRowFromRecord converts a CallRecord to a database.CallRow.
func buildCallRowFromRecord(rec CallRecord, systemID int, siteID *int) *database.CallRow {
	row := &database.CallRow{
		SystemID:     systemID,
		SiteID:       siteID,
		Tgid:         rec.Tgid,
		StartTime:    rec.StartTime,
		StopTime:     rec.StopTime,
		Duration:     rec.Duration,
		Freq:         rec.Freq,
		FreqError:    rec.FreqError,
		SignalDB:     rec.SignalDB,
		NoiseDB:      rec.NoiseDB,
		ErrorCount:   rec.ErrorCount,
		SpikeCount:   rec.SpikeCount,
		AudioType:    rec.AudioType,
		Phase2TDMA:   rec.Phase2TDMA,
		TDMASlot:     rec.TDMASlot,
		Analog:       rec.Analog,
		Conventional: rec.Conventional,
		Encrypted:    rec.Encrypted,
		Emergency:    rec.Emergency,
		SrcList:      rec.SrcList,
		FreqList:     rec.FreqList,
		IncidentData: rec.IncidentData,
		InstanceID:   rec.InstanceID,
	}

	// Convert []int → []int32
	if len(rec.PatchedTgids) > 0 {
		row.PatchedTgids = make([]int32, len(rec.PatchedTgids))
		for i, v := range rec.PatchedTgids {
			row.PatchedTgids[i] = int32(v)
		}
	}
	if len(rec.UnitIDs) > 0 {
		row.UnitIDs = make([]int32, len(rec.UnitIDs))
		for i, v := range rec.UnitIDs {
			row.UnitIDs[i] = int32(v)
		}
	}

	return row
}

// rebuildCallFrequencies parses freq_list JSONB and inserts call_frequencies rows.
func rebuildCallFrequencies(ctx context.Context, db *database.DB, callID int64, startTime time.Time, freqListJSON json.RawMessage, log zerolog.Logger) {
	var freqList []struct {
		Freq       int64    `json:"freq"`
		Time       *float64 `json:"time"`
		Pos        *float32 `json:"pos"`
		Len        *float32 `json:"len"`
		ErrorCount *int     `json:"error_count"`
		SpikeCount *int     `json:"spike_count"`
	}
	if err := json.Unmarshal(freqListJSON, &freqList); err != nil {
		log.Debug().Err(err).Msg("failed to parse freq_list")
		return
	}
	rows := make([]database.CallFrequencyRow, len(freqList))
	for i, f := range freqList {
		rows[i] = database.CallFrequencyRow{
			CallID:        callID,
			CallStartTime: startTime,
			Freq:          f.Freq,
			Pos:           f.Pos,
			Len:           f.Len,
			ErrorCount:    f.ErrorCount,
			SpikeCount:    f.SpikeCount,
		}
		if f.Time != nil {
			t := time.Unix(int64(*f.Time), 0)
			rows[i].Time = &t
		}
	}
	if len(rows) > 0 {
		if _, err := db.InsertCallFrequencies(ctx, rows); err != nil {
			log.Debug().Err(err).Msg("failed to insert call_frequencies")
		}
	}
}

// rebuildCallTransmissions parses src_list JSONB and inserts call_transmissions rows.
func rebuildCallTransmissions(ctx context.Context, db *database.DB, callID int64, startTime time.Time, srcListJSON json.RawMessage, log zerolog.Logger) {
	var srcList []struct {
		Src          int      `json:"src"`
		Time         *float64 `json:"time"`
		Pos          *float32 `json:"pos"`
		Duration     *float32 `json:"duration"`
		Emergency    *int16   `json:"emergency"`
		SignalSystem *string  `json:"signal_system"`
		Tag          *string  `json:"tag"`
	}
	if err := json.Unmarshal(srcListJSON, &srcList); err != nil {
		log.Debug().Err(err).Msg("failed to parse src_list")
		return
	}
	rows := make([]database.CallTransmissionRow, len(srcList))
	for i, s := range srcList {
		rows[i] = database.CallTransmissionRow{
			CallID:        callID,
			CallStartTime: startTime,
			Src:           s.Src,
			Pos:           s.Pos,
			Duration:      s.Duration,
		}
		if s.Time != nil {
			t := time.Unix(int64(*s.Time), 0)
			rows[i].Time = &t
		}
		if s.Emergency != nil {
			rows[i].Emergency = *s.Emergency
		}
		if s.SignalSystem != nil {
			rows[i].SignalSystem = *s.SignalSystem
		}
		if s.Tag != nil {
			rows[i].Tag = *s.Tag
		}
	}
	if len(rows) > 0 {
		if _, err := db.InsertCallTransmissions(ctx, rows); err != nil {
			log.Debug().Err(err).Msg("failed to insert call_transmissions")
		}
	}
}
