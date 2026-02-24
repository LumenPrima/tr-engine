package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleAudio(payload []byte) error {
	var msg AudioMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	meta := &msg.Call.Metadata
	startTime := time.Unix(meta.StartTime, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	identity, err := p.identity.Resolve(ctx, msg.InstanceID, meta.ShortName)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Find the matching call, or create one from audio metadata
	callID, callStartTime, err := p.db.FindCallForAudio(ctx, identity.SystemID, meta.Talkgroup, startTime)
	if err != nil {
		// No call record yet — create one from audio metadata.
		// call_end will find this record later via FindCallForAudio and update it.
		callID, callStartTime, err = p.createCallFromAudio(ctx, identity, meta, startTime)
		if err != nil {
			p.log.Error().Err(err).
				Int("tgid", meta.Talkgroup).
				Str("sys_name", meta.ShortName).
				Msg("failed to create call from audio")
			return nil
		}
	}

	// Decode and save audio file (skip when TR_AUDIO_DIR is set — files served from TR's filesystem)
	var audioPath string
	var audioSize int

	if p.trAudioDir == "" {
		audioData := msg.Call.AudioM4ABase64
		inferredType := "m4a"
		if audioData == "" {
			audioData = msg.Call.AudioWavBase64
			inferredType = "wav"
		}

		// Prefer audio_type from metadata, fall back to whichever base64 field was populated
		audioType := meta.AudioType
		if audioType == "" {
			audioType = inferredType
		}

		if audioData != "" {
			decoded, decErr := base64.StdEncoding.DecodeString(audioData)
			if decErr != nil {
				p.log.Warn().Err(decErr).Msg("failed to decode audio base64")
			} else {
				audioSize = len(decoded)
				audioPath, err = p.saveAudioFile(meta.ShortName, startTime, meta.Filename, audioType, decoded)
				if err != nil {
					p.log.Error().Err(err).Msg("failed to save audio file")
				}
			}
		}

		// Update call with audio path if we found the call
		if callID > 0 && audioPath != "" {
			if err := p.db.UpdateCallAudio(ctx, callID, callStartTime, audioPath, audioSize); err != nil {
				p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to update call audio")
			}
		}
	}

	// Build srcList/freqList JSON and update call
	if callID > 0 {
		p.processSrcFreqData(ctx, callID, callStartTime, meta)
	}

	// Enqueue for transcription if audio was saved and call is not encrypted
	if callID > 0 && meta.Encrypted == 0 {
		if meta.Transcript != "" {
			p.insertSourceTranscription(callID, callStartTime, identity.SystemID, meta.Talkgroup, meta)
		} else {
			p.enqueueTranscription(callID, callStartTime, identity.SystemID, audioPath, meta)
		}
	}

	p.log.Debug().
		Str("sys_name", meta.ShortName).
		Int("tgid", meta.Talkgroup).
		Int("audio_size", audioSize).
		Int("freqs", len(meta.FreqList)).
		Int("srcs", len(meta.SrcList)).
		Msg("audio processed")

	return nil
}

// createCallFromAudio creates a call record from audio metadata when no call_start was received.
// The call_end handler will later find this record via FindCallForAudio and enrich it.
func (p *Pipeline) createCallFromAudio(ctx context.Context, identity *ResolvedIdentity, meta *AudioMetadata, startTime time.Time) (int64, time.Time, error) {
	// Final dedup check right before INSERT — narrows the TOCTOU race window
	// between concurrent MQTT (handleAudio) and file-watch (processWatchedFile)
	// paths from seconds to sub-millisecond.
	if existingID, existingST, err := p.db.FindCallForAudio(ctx, identity.SystemID, meta.Talkgroup, startTime); err == nil {
		return existingID, existingST, nil
	}

	freq := int64(meta.Freq)
	duration := float32(meta.CallLength)
	signal := float32(meta.Signal)
	noise := float32(meta.Noise)
	tdmaSlot := int16(meta.TDMASlot)
	recNum := int16(meta.RecorderNum)
	srcNum := int16(meta.SourceNum)
	siteID := identity.SiteID
	freqError := meta.FreqError
	encrypted := meta.Encrypted != 0
	emergency := meta.Emergency != 0

	row := &database.CallRow{
		SystemID:      identity.SystemID,
		SiteID:        &siteID,
		Tgid:          meta.Talkgroup,
		StartTime:     startTime,
		Duration:      &duration,
		Freq:          &freq,
		FreqError:     &freqError,
		SignalDB:      &signal,
		NoiseDB:       &noise,
		AudioType:     meta.AudioType,
		Phase2TDMA:    meta.Phase2TDMA != 0,
		TDMASlot:      &tdmaSlot,
		Encrypted:     encrypted,
		Emergency:     emergency,
		RecNum:        &recNum,
		SrcNum:        &srcNum,
		SystemName:    meta.ShortName,
		SiteShortName: meta.ShortName,
		TgAlphaTag:    meta.TalkgroupTag,
		TgDescription: meta.TalkgroupDesc,
		TgTag:         meta.TalkgroupGroupTag,
		TgGroup:       meta.TalkgroupGroup,
		IncidentData:  meta.IncidentData,
	}

	if meta.StopTime > 0 {
		st := time.Unix(meta.StopTime, 0)
		row.StopTime = &st
	}

	callID, err := p.db.InsertCall(ctx, row)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("insert call from audio: %w", err)
	}

	// Upsert talkgroup
	if meta.Talkgroup > 0 {
		_ = p.db.UpsertTalkgroup(ctx, identity.SystemID, meta.Talkgroup,
			meta.TalkgroupTag, meta.TalkgroupGroupTag, meta.TalkgroupGroup, meta.TalkgroupDesc, startTime,
		)
	}

	// Create call group
	cgID, cgErr := p.db.UpsertCallGroup(ctx, identity.SystemID, meta.Talkgroup, startTime,
		meta.TalkgroupTag, meta.TalkgroupDesc, meta.TalkgroupGroupTag, meta.TalkgroupGroup,
	)
	if cgErr == nil {
		_ = p.db.SetCallGroupID(ctx, callID, startTime, cgID)
		_ = p.db.SetCallGroupPrimary(ctx, cgID, callID)
	}

	p.log.Debug().
		Int64("call_id", callID).
		Int("tgid", meta.Talkgroup).
		Str("sys_name", meta.ShortName).
		Msg("call created from audio metadata")

	return callID, startTime, nil
}

// processSrcFreqData builds srcList/freqList JSON, updates the call's denormalized
// JSONB columns, and inserts into the relational call_frequencies/call_transmissions tables.
func (p *Pipeline) processSrcFreqData(ctx context.Context, callID int64, callStartTime time.Time, meta *AudioMetadata) {
	if len(meta.SrcList) == 0 && len(meta.FreqList) == 0 {
		return
	}

	// Build freqList JSON
	var freqListJSON json.RawMessage
	if len(meta.FreqList) > 0 {
		type freqEntry struct {
			Freq       int64   `json:"freq"`
			Time       int64   `json:"time"`
			Pos        float64 `json:"pos"`
			Len        float64 `json:"len"`
			ErrorCount int     `json:"error_count"`
			SpikeCount int     `json:"spike_count"`
		}
		entries := make([]freqEntry, len(meta.FreqList))
		for i, f := range meta.FreqList {
			entries[i] = freqEntry{
				Freq:       int64(f.Freq),
				Time:       f.Time,
				Pos:        f.Pos,
				Len:        f.Len,
				ErrorCount: f.ErrorCount,
				SpikeCount: f.SpikeCount,
			}
		}
		freqListJSON, _ = json.Marshal(entries)
	}

	// Build srcList JSON with computed duration
	var srcListJSON json.RawMessage
	unitSet := make(map[int32]struct{})
	if len(meta.SrcList) > 0 {
		type srcEntry struct {
			Src          int     `json:"src"`
			Tag          string  `json:"tag,omitempty"`
			Time         int64   `json:"time"`
			Pos          float64 `json:"pos"`
			Duration     float64 `json:"duration,omitempty"`
			Emergency    int     `json:"emergency"`
			SignalSystem string  `json:"signal_system,omitempty"`
		}
		entries := make([]srcEntry, len(meta.SrcList))
		for i, s := range meta.SrcList {
			var dur float64
			if i+1 < len(meta.SrcList) {
				dur = meta.SrcList[i+1].Pos - s.Pos
			} else if meta.CallLength > 0 {
				dur = float64(meta.CallLength) - s.Pos
			}
			entries[i] = srcEntry{
				Src:          s.Src,
				Tag:          s.Tag,
				Time:         s.Time,
				Pos:          s.Pos,
				Duration:     dur,
				Emergency:    s.Emergency,
				SignalSystem: s.SignalSystem,
			}
			unitSet[int32(s.Src)] = struct{}{}
		}
		srcListJSON, _ = json.Marshal(entries)
	}

	unitIDs := make([]int32, 0, len(unitSet))
	for uid := range unitSet {
		unitIDs = append(unitIDs, uid)
	}

	if err := p.db.UpdateCallSrcFreq(ctx, callID, callStartTime, srcListJSON, freqListJSON, unitIDs); err != nil {
		p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to update call src/freq data")
	}

	// Insert into relational tables for ad-hoc queries
	if len(meta.FreqList) > 0 {
		freqRows := make([]database.CallFrequencyRow, 0, len(meta.FreqList))
		for _, f := range meta.FreqList {
			ft := time.Unix(f.Time, 0)
			pos := float32(f.Pos)
			length := float32(f.Len)
			ec := f.ErrorCount
			sc := f.SpikeCount
			freqRows = append(freqRows, database.CallFrequencyRow{
				CallID:        callID,
				CallStartTime: callStartTime,
				Freq:          int64(f.Freq),
				Time:          &ft,
				Pos:           &pos,
				Len:           &length,
				ErrorCount:    &ec,
				SpikeCount:    &sc,
			})
		}
		if _, err := p.db.InsertCallFrequencies(ctx, freqRows); err != nil {
			p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to insert call frequencies")
		}
	}
	if len(meta.SrcList) > 0 {
		txRows := make([]database.CallTransmissionRow, 0, len(meta.SrcList))
		for i, s := range meta.SrcList {
			st := time.Unix(s.Time, 0)
			pos := float32(s.Pos)
			var dur *float32
			if i+1 < len(meta.SrcList) {
				d := float32(meta.SrcList[i+1].Pos - s.Pos)
				dur = &d
			} else if meta.CallLength > 0 {
				d := float32(float64(meta.CallLength) - s.Pos)
				dur = &d
			}
			txRows = append(txRows, database.CallTransmissionRow{
				CallID:        callID,
				CallStartTime: callStartTime,
				Src:           s.Src,
				Time:          &st,
				Pos:           &pos,
				Duration:      dur,
				Emergency:     int16(s.Emergency),
				SignalSystem:  s.SignalSystem,
				Tag:           s.Tag,
			})
		}
		if _, err := p.db.InsertCallTransmissions(ctx, txRows); err != nil {
			p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to insert call transmissions")
		}
	}
}

// processWatchedFile handles a JSON metadata file from the file watcher.
// It creates a call record, processes srcList/freqList, sets the audio path,
// and publishes a call_end SSE event.
func (p *Pipeline) processWatchedFile(instanceID string, meta *AudioMetadata, jsonPath string) error {
	startTime := time.Unix(meta.StartTime, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
	defer cancel()

	// Resolve identity (auto-creates system/site if needed)
	identity, err := p.identity.Resolve(ctx, instanceID, meta.ShortName)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Check for existing call (dedup against MQTT ingest or prior backfill)
	if existingID, _, findErr := p.db.FindCallForAudio(ctx, identity.SystemID, meta.Talkgroup, startTime); findErr == nil {
		p.log.Debug().
			Int64("call_id", existingID).
			Str("path", jsonPath).
			Msg("watched file already in DB, skipping")
		return nil
	}

	// Create call from audio metadata
	callID, callStartTime, err := p.createCallFromAudio(ctx, identity, meta, startTime)
	if err != nil && strings.Contains(err.Error(), "no partition") {
		// Auto-create missing partition and retry once
		p.ensurePartitionsFor(startTime)
		callID, callStartTime, err = p.createCallFromAudio(ctx, identity, meta, startTime)
	}
	if err != nil {
		return fmt.Errorf("create call from watched file: %w", err)
	}

	// Set call_filename to the companion audio file next to the .json.
	// Try common extensions in preference order.
	base := strings.TrimSuffix(jsonPath, ".json")
	var audioPath string
	for _, ext := range []string{".m4a", ".wav", ".mp3"} {
		if _, statErr := os.Stat(base + ext); statErr == nil {
			audioPath = base + ext
			break
		}
	}
	if audioPath != "" {
		if err := p.db.UpdateCallFilename(ctx, callID, callStartTime, audioPath); err != nil {
			p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to set call_filename from watched file")
		}
		meta.Filename = audioPath // pass to transcription job
	}

	// Process srcList/freqList
	p.processSrcFreqData(ctx, callID, callStartTime, meta)

	// Upsert units from srcList
	for _, s := range meta.SrcList {
		if s.Src > 0 {
			_ = p.db.UpsertUnit(ctx, identity.SystemID, s.Src,
				s.Tag, "file_watch", startTime, meta.Talkgroup,
			)
		}
	}

	// Publish call_end SSE event (file appears after call is complete)
	stopTime := startTime
	if meta.StopTime > 0 {
		stopTime = time.Unix(meta.StopTime, 0)
	}
	p.PublishEvent(EventData{
		Type:      "call_end",
		SystemID:  identity.SystemID,
		SiteID:    identity.SiteID,
		Tgid:      meta.Talkgroup,
		Emergency: meta.Emergency != 0,
		Payload: map[string]any{
			"call_id":       callID,
			"system_id":     identity.SystemID,
			"tgid":          meta.Talkgroup,
			"tg_alpha_tag":  meta.TalkgroupTag,
			"freq":          int64(meta.Freq),
			"start_time":    startTime,
			"stop_time":     stopTime,
			"duration":      float64(meta.CallLength),
			"emergency":     meta.Emergency != 0,
			"encrypted":     meta.Encrypted != 0,
			"call_filename": audioPath,
			"source":        "file_watch",
		},
	})

	// Enqueue for transcription if not encrypted
	if meta.Encrypted == 0 {
		if meta.Transcript != "" {
			p.insertSourceTranscription(callID, callStartTime, identity.SystemID, meta.Talkgroup, meta)
		} else {
			p.enqueueTranscription(callID, callStartTime, identity.SystemID, audioPath, meta)
		}
	}

	p.log.Debug().
		Int64("call_id", callID).
		Int("tgid", meta.Talkgroup).
		Str("sys_name", meta.ShortName).
		Str("path", jsonPath).
		Msg("call created from watched file")

	return nil
}

// saveAudioFile writes decoded audio to the filesystem.
// Path: {audioDir}/{sysName}/{YYYY-MM-DD}/{filename}
func (p *Pipeline) saveAudioFile(sysName string, startTime time.Time, filename string, audioType string, data []byte) (string, error) {
	dateDir := startTime.Format("2006-01-02")
	dir := filepath.Join(p.audioDir, sysName, dateDir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if filename == "" {
		// Use audio_type from metadata if available, default to .wav
		ext := ".wav"
		if audioType != "" {
			// audio_type from TR is typically "m4a", "wav", "mp3" (no dot)
			if audioType[0] != '.' {
				ext = "." + audioType
			} else {
				ext = audioType
			}
		}
		filename = fmt.Sprintf("%d%s", startTime.Unix(), ext)
	}

	path := filepath.Join(dir, filename)

	// Write to a temp file then rename for atomicity — prevents concurrent
	// HTTP audio serve from reading a partial/truncated file.
	tmp, err := os.CreateTemp(dir, ".audio-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}

	// Return relative path from audioDir
	relPath := filepath.Join(sysName, dateDir, filename)
	return relPath, nil
}
