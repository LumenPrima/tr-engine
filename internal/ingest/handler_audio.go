package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		}
	}

	// Decode and save audio file
	audioData := msg.Call.AudioM4ABase64
	if audioData == "" {
		audioData = msg.Call.AudioWavBase64
	}

	var audioPath string
	var audioSize int

	if audioData != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(audioData)
		if decErr != nil {
			p.log.Warn().Err(decErr).Msg("failed to decode audio base64")
		} else {
			audioSize = len(decoded)
			audioPath, err = p.saveAudioFile(meta.ShortName, startTime, meta.Filename, decoded)
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

	// Insert call frequencies
	if callID > 0 && len(meta.FreqList) > 0 {
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

	// Insert call transmissions
	if callID > 0 && len(meta.SrcList) > 0 {
		txRows := make([]database.CallTransmissionRow, 0, len(meta.SrcList))
		for _, s := range meta.SrcList {
			st := time.Unix(s.Time, 0)
			pos := float32(s.Pos)
			txRows = append(txRows, database.CallTransmissionRow{
				CallID:        callID,
				CallStartTime: callStartTime,
				Src:           s.Src,
				Time:          &st,
				Pos:           &pos,
				Emergency:     int16(s.Emergency),
				SignalSystem:  s.SignalSystem,
				Tag:           s.Tag,
			})
		}
		if _, err := p.db.InsertCallTransmissions(ctx, txRows); err != nil {
			p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to insert call transmissions")
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
			meta.TalkgroupTag, meta.TalkgroupGroupTag, meta.TalkgroupGroup, meta.TalkgroupDesc,
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

// saveAudioFile writes decoded audio to the filesystem.
// Path: {audioDir}/{sysName}/{YYYY-MM-DD}/{filename}
func (p *Pipeline) saveAudioFile(sysName string, startTime time.Time, filename string, data []byte) (string, error) {
	dateDir := startTime.Format("2006-01-02")
	dir := filepath.Join(p.audioDir, sysName, dateDir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if filename == "" {
		filename = fmt.Sprintf("%d.m4a", startTime.Unix())
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	// Return relative path from audioDir
	relPath := filepath.Join(sysName, dateDir, filename)
	return relPath, nil
}
