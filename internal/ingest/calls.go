package ingest

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/metrics"
	"go.uber.org/zap"
)

// ProcessCallStart handles a call_start message
func (p *Processor) ProcessCallStart(ctx context.Context, data *CallEventData) error {
	instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
	if err != nil {
		return err
	}

	sys, err := p.getSystem(ctx, data.ShortName)
	if err != nil {
		return err
	}
	if sys == nil {
		p.logger.Warn("System not found for call start",
			zap.String("short_name", data.ShortName),
		)
		return nil
	}
	sysid := database.EffectiveSYSID(sys)

	// Upsert talkgroup
	tg, err := p.db.UpsertTalkgroup(ctx, sysid, data.TGID, data.TGAlphaTag, data.TGDesc, data.TGGroup, data.TGTag, 0, "")
	if err != nil {
		p.logger.Error("Failed to upsert talkgroup", zap.Error(err))
	}

	var tgSysid *string
	if tg != nil {
		tgSysid = &tg.SYSID
		// Record site association
		if err := p.db.UpsertTalkgroupSite(ctx, tg.SYSID, tg.TGID, sys.ID); err != nil {
			p.logger.Error("Failed to upsert talkgroup site", zap.Error(err))
		}
	}

	// Upsert initiating unit if present
	if data.Unit > 0 {
		unit, err := p.db.UpsertUnit(ctx, sysid, data.Unit, data.UnitAlphaTag, "ota")
		if err != nil {
			p.logger.Error("Failed to upsert unit from call_start", zap.Error(err))
		} else if unit != nil {
			// Record site association
			if err := p.db.UpsertUnitSite(ctx, unit.SYSID, unit.UnitID, sys.ID); err != nil {
				p.logger.Error("Failed to upsert unit site", zap.Error(err))
			}
		}
	}

	// Create call record
	call := &models.Call{
		InstanceID:   instID,
		SystemID:     sys.ID,
		TgSysid:      tgSysid,
		TRCallID:     data.CallID,
		CallNum:      data.CallNum,
		StartTime:    data.StartTime,
		CallState:    int16(data.CallState),
		MonState:     int16(data.MonState),
		Encrypted:    data.Encrypted,
		Emergency:    data.Emergency,
		Phase2TDMA:   data.Phase2TDMA,
		TDMASlot:     int16(data.TDMASlot),
		Conventional: data.Conventional,
		Analog:       data.Analog,
		AudioType:    data.AudioType,
		Freq:         data.Freq,
		FreqError:    data.FreqError,
		MetadataJSON: data.RawJSON,
	}

	if err := p.db.InsertCall(ctx, call, data.TGID); err != nil {
		return err
	}

	// Track in-memory active calls
	p.trackActiveCall(data)

	p.logger.Debug("Call started",
		zap.String("call_id", data.CallID),
		zap.Int("tgid", data.TGID),
		zap.String("system", data.ShortName),
	)

	// Track metrics
	metrics.CallsProcessed.WithLabelValues("start", data.ShortName).Inc()

	// Broadcast call start
	p.broadcast("call_start", map[string]interface{}{
		"call_id":             call.ID,
		"tr_call_id":          data.CallID,
		"talkgroup":           data.TGID,
		"talkgroup_alpha_tag": data.TGAlphaTag,
		"system":              data.ShortName,
		"sysid":               sysid,
		"freq":                data.Freq,
		"unit":                data.Unit,
		"unit_alpha_tag":      data.UnitAlphaTag,
		"encrypted":           data.Encrypted,
		"emergency":           data.Emergency,
	})

	return nil
}

// SnapshotDiff describes what changed between two calls_active snapshots
type SnapshotDiff struct {
	NewCalls     []*CallEventData // Calls in snapshot not previously tracked
	EndedCalls   []*ActiveCallInfo // Calls previously tracked but no longer in snapshot
	UpdatedCalls []*CallEventData // Calls still active (for mid-call state updates)
}

// DiffActiveCalls compares a new calls_active snapshot against the in-memory tracker
// and returns what's new, what ended, and what's still going. Also updates the in-memory map.
func (p *Processor) DiffActiveCalls(calls []*ActiveCallInfo, callData []*CallEventData) *SnapshotDiff {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	diff := &SnapshotDiff{}

	// Build a set of keys from the new snapshot
	newKeys := make(map[string]bool, len(calls))
	for i, call := range calls {
		key := makeCallKey(call.System, call.CallNum)
		newKeys[key] = true

		if existing, exists := p.activeCalls[key]; exists {
			// Call exists — update in-memory state, merge units
			existing.TGAlphaTag = call.TGAlphaTag
			existing.Freq = call.Freq
			existing.Encrypted = call.Encrypted
			existing.Emergency = call.Emergency
			existing.MissedCount = 0 // Reset missed count since call is present
			for _, newUnit := range call.Units {
				p.addUnitToCallLocked(existing, newUnit.UnitID, newUnit.UnitTag)
			}
			// Backfill CallID if we didn't have it
			if call.CallID != "" && existing.CallID == "" {
				existing.CallID = call.CallID
			}
			diff.UpdatedCalls = append(diff.UpdatedCalls, callData[i])
		} else {
			// New call — not previously tracked
			p.activeCalls[key] = call
			diff.NewCalls = append(diff.NewCalls, callData[i])
		}
	}

	// Find calls that are no longer in the snapshot
	// Use a grace period (missedSnapshotThreshold) to allow call_end messages to arrive
	// before marking a call as ended via snapshot diff
	for key, call := range p.activeCalls {
		if !newKeys[key] {
			call.MissedCount++
			if call.MissedCount >= missedSnapshotThreshold {
				// Call has been missing for multiple snapshots, consider it ended
				call.EndTime = time.Now()
				p.recentCalls = append([]*ActiveCallInfo{call}, p.recentCalls...)
				delete(p.activeCalls, key)
				diff.EndedCalls = append(diff.EndedCalls, call)
			}
		}
	}

	// Trim recent calls
	if len(p.recentCalls) > maxRecentCalls {
		p.recentCalls = p.recentCalls[:maxRecentCalls]
	}

	p.logger.Debug("Diffed active calls snapshot",
		zap.Int("new", len(diff.NewCalls)),
		zap.Int("ended", len(diff.EndedCalls)),
		zap.Int("updated", len(diff.UpdatedCalls)),
		zap.Int("active", len(p.activeCalls)),
	)

	return diff
}

// ProcessNewCallFromSnapshot handles a call that appeared in calls_active
// but was never seen via call_start. Creates the DB record.
func (p *Processor) ProcessNewCallFromSnapshot(ctx context.Context, data *CallEventData) error {
	p.logger.Info("Discovered new call from calls_active snapshot",
		zap.String("call_id", data.CallID),
		zap.Int("tgid", data.TGID),
		zap.String("system", data.ShortName),
	)
	return p.ProcessCallStart(ctx, data)
}

// ProcessMissedCallEnd handles a call that disappeared from calls_active
// without a call_end message. Marks it as ended in the DB.
func (p *Processor) ProcessMissedCallEnd(ctx context.Context, ended *ActiveCallInfo) {
	if ended.CallID == "" && ended.CallNum == 0 {
		return
	}

	// Try to find the call in the DB
	var call *models.Call
	var err error

	if ended.CallID != "" {
		call, err = p.db.GetCallByTRID(ctx, ended.CallID, ended.StartTime)
		if err != nil {
			p.logger.Error("Failed to find ended call by tr_call_id", zap.Error(err))
		}
	}

	// Get system for sysid lookup
	sys, _ := p.getSystem(ctx, ended.System)
	var sysid string
	if sys != nil {
		sysid = database.EffectiveSYSID(sys)
		if call == nil {
			call, err = p.db.GetCallBySystemTGIDAndTime(ctx, sys.ID, ended.TGID, ended.StartTime)
			if err != nil {
				p.logger.Debug("Error finding ended call by tgid/time", zap.Error(err))
			}
		}
	}

	if call == nil {
		p.logger.Debug("Ended call not found in DB, skipping",
			zap.String("call_id", ended.CallID),
			zap.Int("tgid", ended.TGID),
		)
		return
	}

	// Only update if the call doesn't already have a stop_time (call_end may have handled it)
	if call.StopTime != nil {
		return
	}

	now := ended.EndTime
	call.StopTime = &now
	if ended.StartTime.Before(now) {
		call.Duration = float32(now.Sub(ended.StartTime).Seconds())
	}

	if err := p.db.UpdateCall(ctx, call); err != nil {
		p.logger.Error("Failed to update missed call end", zap.Error(err))
		return
	}

	p.logger.Debug("Marked missed call as ended from snapshot diff",
		zap.Int64("call_id", call.ID),
		zap.String("system", ended.System),
		zap.Int("tgid", ended.TGID),
	)

	metrics.CallsProcessed.WithLabelValues("end_missed", ended.System).Inc()

	p.broadcast("call_end", map[string]interface{}{
		"call_id":    call.ID,
		"talkgroup":  ended.TGID,
		"system":     ended.System,
		"sysid":      sysid,
		"duration":   call.Duration,
		"encrypted":  ended.Encrypted,
		"emergency":  ended.Emergency,
	})
}

// ProcessCallActiveUpdate handles a mid-call state update from calls_active.
// Only updates the DB if meaningful fields changed (not every snapshot).
func (p *Processor) ProcessCallActiveUpdate(ctx context.Context, data *CallEventData) {
	// Track metrics
	metrics.CallsProcessed.WithLabelValues("active", data.ShortName).Inc()
}

// ProcessCallEnd handles a call_end message
func (p *Processor) ProcessCallEnd(ctx context.Context, data *CallEventData) error {
	instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
	if err != nil {
		return err
	}

	sys, err := p.getSystem(ctx, data.ShortName)
	if err != nil {
		return err
	}
	if sys == nil {
		p.logger.Warn("System not found for call end",
			zap.String("short_name", data.ShortName),
		)
		return nil
	}
	sysid := database.EffectiveSYSID(sys)

	// Upsert talkgroup
	tg, err := p.db.UpsertTalkgroup(ctx, sysid, data.TGID, data.TGAlphaTag, data.TGDesc, data.TGGroup, data.TGTag, 0, "")
	if err != nil {
		p.logger.Error("Failed to upsert talkgroup", zap.Error(err))
	}

	var tgSysid *string
	if tg != nil {
		tgSysid = &tg.SYSID
		if err := p.db.UpsertTalkgroupSite(ctx, tg.SYSID, tg.TGID, sys.ID); err != nil {
			p.logger.Error("Failed to upsert talkgroup site", zap.Error(err))
		}
	}

	// Upsert unit if present (may have missed call_start)
	if data.Unit > 0 {
		unit, err := p.db.UpsertUnit(ctx, sysid, data.Unit, data.UnitAlphaTag, "ota")
		if err != nil {
			p.logger.Error("Failed to upsert unit from call_end", zap.Error(err))
		} else if unit != nil {
			if err := p.db.UpsertUnitSite(ctx, unit.SYSID, unit.UnitID, sys.ID); err != nil {
				p.logger.Error("Failed to upsert unit site", zap.Error(err))
			}
		}
	}

	// Find or create call - first try by tr_call_id
	call, err := p.db.GetCallByTRID(ctx, data.CallID, data.StartTime)
	if err != nil {
		return err
	}

	// If not found by tr_call_id, try by tgid + start_time (for calls created from audio)
	if call == nil {
		call, err = p.db.GetCallBySystemTGIDAndTime(ctx, sys.ID, data.TGID, data.StartTime)
		if err != nil {
			p.logger.Debug("Error finding call by tgid/time", zap.Error(err))
		}
		if call != nil {
			// Update tr_call_id to link this call to the trunk-recorder call ID
			call.TRCallID = data.CallID
			p.logger.Debug("Found existing call by tgid/time, linking tr_call_id",
				zap.Int64("call_id", call.ID),
				zap.String("tr_call_id", data.CallID),
			)
		}
	}

	if call == nil {
		// Create new call record (call_start and audio might have been missed)
		call = &models.Call{
			InstanceID:   instID,
			SystemID:     sys.ID,
			TgSysid:      tgSysid,
			TRCallID:     data.CallID,
			CallNum:      data.CallNum,
			StartTime:    data.StartTime,
			Freq:         data.Freq,
			FreqError:    data.FreqError,
			Encrypted:    data.Encrypted,
			Emergency:    data.Emergency,
			Phase2TDMA:   data.Phase2TDMA,
			TDMASlot:     int16(data.TDMASlot),
			Conventional: data.Conventional,
			Analog:       data.Analog,
			AudioType:    data.AudioType,
			MetadataJSON: data.RawJSON,
		}
		if err := p.db.InsertCall(ctx, call, data.TGID); err != nil {
			return err
		}
	}

	// Update call with final data
	stopTime := data.StopTime
	call.StopTime = &stopTime
	call.Duration = data.Duration
	call.CallState = int16(data.CallState)
	call.MonState = int16(data.MonState)
	call.Encrypted = data.Encrypted
	call.Emergency = data.Emergency
	call.ErrorCount = data.ErrorCount
	call.SpikeCount = data.SpikeCount
	call.SignalDB = filterSentinelDB(data.SignalDB)
	call.NoiseDB = filterSentinelDB(data.NoiseDB)
	call.MetadataJSON = data.RawJSON

	if err := p.db.UpdateCall(ctx, call); err != nil {
		return err
	}

	// Handle external audio mode: reference TR's audio files instead of copying
	if p.storage != nil && p.storage.IsExternalMode() && data.CallFilename != "" {
		relativePath := p.storage.GetRelativePathFromExternal(data.CallFilename, data.ShortName)
		fullPath := p.storage.MapExternalPath(data.CallFilename, data.ShortName)

		// Check if the audio file exists
		if p.storage.AudioExists(relativePath) {
			// Get file size
			if fi, err := os.Stat(fullPath); err == nil {
				call.AudioPath = relativePath
				call.AudioSize = int(fi.Size())

				// Read JSON sidecar for transmission/frequency details
				if sidecar, err := p.storage.ReadAudioSidecar(fullPath); err == nil {
					// Convert sidecar srcList to our SourceUnitData format and process
					if len(sidecar.SrcList) > 0 {
						srcList := make([]SourceUnitData, len(sidecar.SrcList))
						for i, src := range sidecar.SrcList {
							srcList[i] = SourceUnitData{
								Src:       src.Src,
								Time:      time.Unix(src.Time, 0),
								Pos:       src.Pos,
								Emergency: src.Emergency != 0,
								Tag:       src.Tag,
							}
						}
						if err := p.processTransmissions(ctx, call, sysid, sys.ID, srcList); err != nil {
							p.logger.Error("Failed to process external audio transmissions", zap.Error(err))
						}
					}

					// Convert sidecar freqList to our FreqEntryData format and process
					if len(sidecar.FreqList) > 0 {
						freqList := make([]FreqEntryData, len(sidecar.FreqList))
						for i, f := range sidecar.FreqList {
							freqList[i] = FreqEntryData{
								Freq:       f.Freq,
								Time:       time.Unix(f.Time, 0),
								Pos:        f.Pos,
								Len:        f.Len,
								ErrorCount: f.ErrorCount,
								SpikeCount: f.SpikeCount,
							}
						}
						if err := p.processFrequencies(ctx, call.ID, freqList); err != nil {
							p.logger.Error("Failed to process external audio frequencies", zap.Error(err))
						}
					}

					p.logger.Debug("Processed external audio sidecar",
						zap.String("path", relativePath),
						zap.Int("transmissions", len(sidecar.SrcList)),
						zap.Int("frequencies", len(sidecar.FreqList)),
					)
				} else {
					p.logger.Debug("No sidecar file for external audio",
						zap.String("path", relativePath),
						zap.Error(err),
					)
				}

				// Update call with audio path
				if err := p.db.UpdateCall(ctx, call); err != nil {
					p.logger.Error("Failed to update call with external audio path", zap.Error(err))
				}

				// Broadcast audio availability
				call.PopulateCallID()
				p.broadcast("audio_available", map[string]interface{}{
					"call_id":             call.CallID, // Deterministic composite ID (sysid:tgid:start_unix)
					"tr_call_id":          data.CallID,
					"talkgroup":           data.TGID,
					"talkgroup_alpha_tag": data.TGAlphaTag,
					"system":              data.ShortName,
					"sysid":               sysid,
					"audio_size":          call.AudioSize,
					"duration":            call.Duration,
				})
			}
		} else {
			p.logger.Debug("External audio file not found",
				zap.String("call_filename", data.CallFilename),
				zap.String("mapped_path", fullPath),
			)
		}
	}

	// Run deduplication if enabled
	if p.dedup != nil && p.dedup.IsEnabled() {
		callGroup, err := p.dedup.ProcessCall(ctx, call, data.TGID, data.ShortName)
		if err != nil {
			p.logger.Error("Deduplication failed", zap.Error(err))
		} else if callGroup != nil {
			call.CallGroupID = &callGroup.ID
			p.db.UpdateCall(ctx, call)
		}
	}

	p.logger.Debug("Call ended",
		zap.String("call_id", data.CallID),
		zap.Int("tgid", data.TGID),
		zap.Float32("duration", data.Duration),
	)

	// Track metrics
	metrics.CallsProcessed.WithLabelValues("end", data.ShortName).Inc()
	if data.Duration > 0 {
		metrics.CallDuration.Observe(float64(data.Duration))
	}

	// Broadcast call end
	p.broadcast("call_end", map[string]interface{}{
		"call_id":             call.ID,
		"tr_call_id":          data.CallID,
		"talkgroup":           data.TGID,
		"talkgroup_alpha_tag": data.TGAlphaTag,
		"system":              data.ShortName,
		"sysid":               sysid,
		"unit":                data.Unit,
		"unit_alpha_tag":      data.UnitAlphaTag,
		"duration":            data.Duration,
		"encrypted":           data.Encrypted,
		"emergency":           data.Emergency,
		"error_count":         data.ErrorCount,
		"spike_count":         data.SpikeCount,
	})

	// Remove from in-memory active calls tracker
	// Try by system+call_num first (direct map lookup), fall back to call_id scan
	p.untrackActiveCallEnd(data.ShortName, data.CallNum, data.CallID)

	return nil
}

// processTransmissions processes unit transmissions from srcList
func (p *Processor) processTransmissions(ctx context.Context, call *models.Call, sysid string, systemID int, srcList []SourceUnitData) error {
	for i, src := range srcList {
		// Upsert unit
		unit, err := p.db.UpsertUnit(ctx, sysid, src.Src, src.Tag, "ota")
		if err != nil {
			p.logger.Error("Failed to upsert unit", zap.Error(err))
			continue
		}

		var unitSysid *string
		if unit != nil {
			unitSysid = &unit.SYSID
			// Record site association
			if err := p.db.UpsertUnitSite(ctx, unit.SYSID, unit.UnitID, systemID); err != nil {
				p.logger.Error("Failed to upsert unit site", zap.Error(err))
			}
		}

		// Calculate duration from position gaps in srcList.
		// Each entry's `pos` is its offset (in seconds) within the audio file.
		// Duration = next entry's pos - this entry's pos (or call end for last entry).
		var duration float32
		var stopTime *time.Time
		if i+1 < len(srcList) {
			// Duration until next transmission starts
			duration = srcList[i+1].Pos - src.Pos
		} else if call.Duration > 0 {
			// Last transmission: duration until end of call
			duration = call.Duration - src.Pos
		}
		if duration > 0 {
			st := src.Time.Add(time.Duration(duration*1000) * time.Millisecond)
			stopTime = &st
		}

		tx := &models.Transmission{
			CallID:     call.ID,
			UnitSysid:  unitSysid,
			UnitRID:    src.Src,
			StartTime:  src.Time,
			StopTime:   stopTime,
			Duration:   duration,
			Position:   src.Pos,
			Emergency:  src.Emergency,
			ErrorCount: src.ErrorCount,
			SpikeCount: src.SpikeCount,
		}

		if err := p.db.InsertTransmission(ctx, tx); err != nil {
			p.logger.Error("Failed to insert transmission",
				zap.Error(err),
				zap.Int64("unit", src.Src),
			)
		}
	}

	return nil
}

// processFrequencies processes frequency entries from freqList
func (p *Processor) processFrequencies(ctx context.Context, callID int64, freqList []FreqEntryData) error {
	for _, f := range freqList {
		cf := &models.CallFrequency{
			CallID:     callID,
			Freq:       f.Freq,
			Time:       f.Time,
			Position:   f.Pos,
			Duration:   f.Len,
			ErrorCount: f.ErrorCount,
			SpikeCount: f.SpikeCount,
		}
		if err := p.db.InsertCallFrequency(ctx, cf); err != nil {
			p.logger.Error("Failed to insert frequency entry",
				zap.Error(err),
				zap.Int64("freq", f.Freq),
			)
		}
	}

	return nil
}

// ProcessAudio handles an audio message
func (p *Processor) ProcessAudio(ctx context.Context, data *AudioData) error {
	// Save audio file
	audioPath, audioSize, err := p.storage.SaveAudio(data.ShortName, data.StartTime, data.AudioData, data.Filename)
	if err != nil {
		return err
	}

	// Track audio metrics
	metrics.AudioFilesProcessed.WithLabelValues(data.ShortName).Inc()
	metrics.AudioFileSize.Observe(float64(audioSize))
	if len(data.SrcList) > 0 {
		metrics.TransmissionsRecorded.WithLabelValues(data.ShortName).Add(float64(len(data.SrcList)))
	}

	// Get system for processing
	sys, _ := p.getSystem(ctx, data.ShortName)
	var sysid string
	if sys != nil {
		sysid = database.EffectiveSYSID(sys)
	}

	// Find the call - audio messages don't have call_id, so search by tgid + start_time
	var call *models.Call
	if data.CallID != "" {
		call, err = p.db.GetCallByTRID(ctx, data.CallID, data.StartTime)
		if err != nil {
			return err
		}
	}

	// If no call_id or not found, try to find by tgid + start_time
	if call == nil && sys != nil {
		call, err = p.db.GetCallByTGIDAndTime(ctx, sys.ID, data.TGID, data.StartTime)
		if err != nil {
			p.logger.Debug("Error looking up call by tgid/time", zap.Error(err))
		}
	}

	if call != nil {
		// Update existing call with audio info
		call.AudioPath = audioPath
		call.AudioSize = audioSize
		if err := p.db.UpdateCall(ctx, call); err != nil {
			return err
		}

		p.logger.Debug("Audio linked to existing call",
			zap.Int64("call_id", call.ID),
			zap.Int("tgid", data.TGID),
			zap.String("path", audioPath),
		)
	} else if sys != nil {
		// Call not found - create it from the audio message
		// This happens when audio arrives before call_start/call_end
		instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
		if err != nil {
			p.logger.Error("Failed to get/create instance for audio call", zap.Error(err))
			return err
		}

		// Upsert talkgroup
		tg, err := p.db.UpsertTalkgroup(ctx, sysid, data.TGID, data.TGAlphaTag, data.TGDesc, data.TGGroup, data.TGTag, 0, "")
		if err != nil {
			p.logger.Error("Failed to upsert talkgroup for audio call", zap.Error(err))
		}

		var tgSysid *string
		if tg != nil {
			tgSysid = &tg.SYSID
			if err := p.db.UpsertTalkgroupSite(ctx, tg.SYSID, tg.TGID, sys.ID); err != nil {
				p.logger.Error("Failed to upsert talkgroup site", zap.Error(err))
			}
		}

		// Calculate duration
		var duration float32
		if !data.StopTime.IsZero() && data.StopTime.After(data.StartTime) {
			duration = float32(data.StopTime.Sub(data.StartTime).Seconds())
		}

		// Create call record from audio metadata
		call = &models.Call{
			InstanceID:   instID,
			SystemID:     sys.ID,
			TgSysid:      tgSysid,
			TRCallID:     data.CallID,
			StartTime:    data.StartTime,
			StopTime:     &data.StopTime,
			Duration:     duration,
			CallState:    3, // Completed
			Encrypted:    data.Encrypted,
			Emergency:    data.Emergency,
			Phase2TDMA:   data.Phase2TDMA,
			TDMASlot:     int16(data.TDMASlot),
			AudioType:    data.AudioType,
			Freq:         int64(data.Freq),
			FreqError:    data.FreqError,
			SignalDB:     filterSentinelDB(data.SignalDB),
			NoiseDB:      filterSentinelDB(data.NoiseDB),
			AudioPath:    audioPath,
			AudioSize:    audioSize,
		}

		if err := p.db.InsertCall(ctx, call, data.TGID); err != nil {
			p.logger.Error("Failed to insert call from audio", zap.Error(err))
			return err
		}

		p.logger.Debug("Created call from audio message",
			zap.Int64("call_id", call.ID),
			zap.Int("tgid", data.TGID),
			zap.String("system", data.ShortName),
			zap.String("path", audioPath),
		)

		// Run deduplication on newly created call
		if p.dedup != nil && p.dedup.IsEnabled() {
			callGroup, err := p.dedup.ProcessCall(ctx, call, data.TGID, data.ShortName)
			if err != nil {
				p.logger.Error("Deduplication failed for audio-created call", zap.Error(err))
			} else if callGroup != nil {
				call.CallGroupID = &callGroup.ID
				p.db.UpdateCall(ctx, call)
			}
		}

		// Track metrics
		metrics.CallsProcessed.WithLabelValues("audio_created", data.ShortName).Inc()
	} else {
		// System not registered - can't create call
		p.logger.Warn("Cannot create call from audio - system not registered",
			zap.Int("tgid", data.TGID),
			zap.String("system", data.ShortName),
			zap.String("path", audioPath),
		)
		return nil
	}

	// Process transmissions from srcList
	if len(data.SrcList) > 0 && sys != nil && call != nil {
		if err := p.processTransmissions(ctx, call, sysid, sys.ID, data.SrcList); err != nil {
			p.logger.Error("Failed to process transmissions", zap.Error(err))
		}
	}

	// Process frequencies from freqList
	if len(data.FreqList) > 0 && call != nil {
		if err := p.processFrequencies(ctx, call.ID, data.FreqList); err != nil {
			p.logger.Error("Failed to process frequencies", zap.Error(err))
		}
	}

	// Broadcast audio availability so frontend knows to fetch/enable playback
	if call != nil {
		call.PopulateCallID()
		p.broadcast("audio_available", map[string]interface{}{
			"call_id":             call.CallID, // Deterministic composite ID (sysid:tgid:start_unix)
			"tr_call_id":          data.CallID,
			"talkgroup":           data.TGID,
			"talkgroup_alpha_tag": data.TGAlphaTag,
			"system":              data.ShortName,
			"sysid":               sysid,
			"audio_size":          audioSize,
			"duration":            call.Duration,
			"transmissions":       len(data.SrcList),
			"frequencies":         len(data.FreqList),
		})

		// Queue for transcription if service is enabled
		if p.transcriber != nil {
			if err := p.transcriber.QueueCall(ctx, call.ID, call.Duration, 0); err != nil {
				p.logger.Error("Failed to queue call for transcription",
					zap.Int64("call_id", call.ID),
					zap.Error(err),
				)
			}
		}
	}

	p.logger.Debug("Audio processing complete",
		zap.Int("tgid", data.TGID),
		zap.String("path", audioPath),
		zap.Int("size", audioSize),
		zap.Int("transmissions", len(data.SrcList)),
		zap.Int("frequencies", len(data.FreqList)),
	)

	return nil
}

// ProcessUnitEvent handles a unit event message
func (p *Processor) ProcessUnitEvent(ctx context.Context, data *UnitEventData) error {
	instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
	if err != nil {
		return err
	}

	sys, err := p.getSystem(ctx, data.ShortName)
	if err != nil {
		return err
	}
	if sys == nil {
		// Try to create system from this message
		sys, err = p.db.UpsertSystem(ctx, instID, data.SysNum, data.ShortName, "", "", "", "", 0, 0, nil)
		if err != nil {
			return err
		}

		p.systemLock.Lock()
		p.systems[data.ShortName] = sys
		p.systemLock.Unlock()
	}
	sysid := database.EffectiveSYSID(sys)

	// Upsert unit
	unit, err := p.db.UpsertUnit(ctx, sysid, data.UnitID, data.UnitTag, "ota")
	if err != nil {
		return err
	}

	var unitSysid *string
	if unit != nil {
		unitSysid = &unit.SYSID
		if err := p.db.UpsertUnitSite(ctx, unit.SYSID, unit.UnitID, sys.ID); err != nil {
			p.logger.Error("Failed to upsert unit site", zap.Error(err))
		}
	}

	// Upsert talkgroup if present
	var tgSysid *string
	if data.TGID > 0 {
		tg, err := p.db.UpsertTalkgroup(ctx, sysid, data.TGID, data.TGAlphaTag, data.TGDesc, data.TGGroup, data.TGTag, 0, "")
		if err != nil {
			p.logger.Error("Failed to upsert talkgroup", zap.Error(err))
		} else if tg != nil {
			tgSysid = &tg.SYSID
			if err := p.db.UpsertTalkgroupSite(ctx, tg.SYSID, tg.TGID, sys.ID); err != nil {
				p.logger.Error("Failed to upsert talkgroup site", zap.Error(err))
			}
		}
	}

	// Insert unit event
	event := &models.UnitEvent{
		InstanceID:   instID,
		SystemID:     sys.ID,
		UnitSysid:    unitSysid,
		UnitRID:      data.UnitID,
		EventType:    data.EventType,
		TgSysid:      tgSysid,
		TGID:         data.TGID,
		Time:         data.Timestamp,
		MetadataJSON: data.RawJSON,
	}

	if err := p.db.InsertUnitEvent(ctx, event); err != nil {
		return err
	}

	// Track metrics
	metrics.UnitEventsProcessed.WithLabelValues(data.EventType, data.ShortName).Inc()

	// Broadcast unit event
	p.broadcast("unit_event", map[string]interface{}{
		"unit":       data.UnitID,
		"unit_tag":   data.UnitTag,
		"event_type": data.EventType,
		"talkgroup":  data.TGID,
		"system":     data.ShortName,
		"sysid":      sysid,
	})

	return nil
}

// ProcessTrunkMessage handles a trunking message
func (p *Processor) ProcessTrunkMessage(ctx context.Context, data *TrunkMessageData) error {
	sysID, err := p.getSystemID(ctx, data.ShortName)
	if err != nil {
		return err
	}
	if sysID == 0 {
		// System not registered, skip
		return nil
	}

	msg := &models.TrunkMessage{
		SystemID:    sysID,
		Time:        data.Timestamp,
		MsgType:     int16(data.MsgType),
		MsgTypeName: data.MsgTypeName,
		Opcode:      data.Opcode,
		OpcodeType:  data.OpcodeType,
		OpcodeDesc:  data.OpcodeDesc,
		Meta:        data.Meta,
	}

	return p.db.InsertTrunkMessage(ctx, msg)
}

// Helper to get JSON bytes
func toJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// filterSentinelDB converts sentinel values (999, -999) to 0 so omitempty excludes them.
// Trunk-recorder uses 999 to indicate unknown signal/noise levels.
func filterSentinelDB(v float32) float32 {
	if v >= 900 || v <= -900 {
		return 0
	}
	return v
}
