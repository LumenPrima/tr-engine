package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleCallStart(payload []byte) error {
	var msg CallStartMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	call := &msg.Call
	startTime := time.Unix(call.StartTime, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	identity, err := p.identity.Resolve(ctx, msg.InstanceID, call.SysName)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Upsert talkgroup
	if call.Talkgroup > 0 {
		if err := p.db.UpsertTalkgroup(ctx, identity.SystemID, call.Talkgroup,
			call.TalkgroupAlphaTag, call.TalkgroupTag, call.TalkgroupGroup, call.TalkgroupDescription,
		); err != nil {
			p.log.Warn().Err(err).Int("tgid", call.Talkgroup).Msg("failed to upsert talkgroup")
		}
	}

	// Upsert unit
	if call.Unit > 0 {
		if err := p.db.UpsertUnit(ctx, identity.SystemID, call.Unit,
			call.UnitAlphaTag, "call_start", startTime, call.Talkgroup,
		); err != nil {
			p.log.Warn().Err(err).Int("unit", call.Unit).Msg("failed to upsert unit")
		}
	}

	// Check if already tracked
	if _, ok := p.activeCalls.Get(call.ID); ok {
		return nil // duplicate call_start
	}

	freq := int64(call.Freq)
	duration := float32(call.Length)
	callNum := call.CallNum
	callState := int16(call.CallState)
	monState := int16(call.MonState)
	recState := int16(call.RecState)
	recNum := int16(call.RecNum)
	srcNum := int16(call.SrcNum)
	tdmaSlot := int16(call.TDMASlot)
	siteID := identity.SiteID

	row := &database.CallRow{
		SystemID:      identity.SystemID,
		SiteID:        &siteID,
		Tgid:          call.Talkgroup,
		TrCallID:      call.ID,
		CallNum:       &callNum,
		StartTime:     startTime,
		Duration:      &duration,
		Freq:          &freq,
		AudioType:     call.AudioType,
		Phase2TDMA:    call.Phase2TDMA,
		TDMASlot:      &tdmaSlot,
		Analog:        call.Analog,
		Conventional:  call.Conventional,
		Encrypted:     call.Encrypted,
		Emergency:     call.Emergency,
		CallState:     &callState,
		CallStateType: call.CallStateType,
		MonState:      &monState,
		MonStateType:  call.MonStateType,
		RecState:      &recState,
		RecStateType:  call.RecStateType,
		RecNum:        &recNum,
		SrcNum:        &srcNum,
		SystemName:    call.SysName,
		SiteShortName: call.SysName,
		TgAlphaTag:    call.TalkgroupAlphaTag,
		TgDescription: call.TalkgroupDescription,
		TgTag:         call.TalkgroupTag,
		TgGroup:       call.TalkgroupGroup,
		InstanceID:    msg.InstanceID,
	}

	if call.StopTime > 0 {
		st := time.Unix(call.StopTime, 0)
		row.StopTime = &st
	}

	callID, err := p.db.InsertCall(ctx, row)
	if err != nil {
		return fmt.Errorf("insert call: %w", err)
	}

	p.activeCalls.Set(call.ID, callID, startTime)

	// Create call group
	cgID, err := p.db.UpsertCallGroup(ctx, identity.SystemID, call.Talkgroup, startTime,
		call.TalkgroupAlphaTag, call.TalkgroupDescription, call.TalkgroupTag, call.TalkgroupGroup,
	)
	if err != nil {
		p.log.Warn().Err(err).Msg("failed to upsert call group")
	} else {
		_ = p.db.SetCallGroupID(ctx, callID, startTime, cgID)
		_ = p.db.SetCallGroupPrimary(ctx, cgID, callID)
	}

	p.log.Debug().
		Str("tr_call_id", call.ID).
		Int64("call_id", callID).
		Int("tgid", call.Talkgroup).
		Str("sys_name", call.SysName).
		Msg("call started")

	return nil
}

func (p *Pipeline) handleCallEnd(payload []byte) error {
	var msg CallEndMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	call := &msg.Call
	startTime := time.Unix(call.StartTime, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	// Find active call
	entry, ok := p.activeCalls.Get(call.ID)
	if !ok {
		// Call started before we were running, or duplicate. Try DB lookup.
		var err error
		entry.CallID, entry.StartTime, err = p.db.FindCallByTrCallID(ctx, call.ID)
		if err != nil {
			// Can't find the call — insert it fresh
			return p.handleCallStartFromEnd(ctx, &msg)
		}
	}

	stopTime := time.Unix(call.StopTime, 0)

	err := p.db.UpdateCallEnd(ctx,
		entry.CallID, entry.StartTime,
		stopTime,
		float32(call.Length),
		int64(call.Freq),
		call.FreqError,
		float32(call.Signal),
		float32(call.Noise),
		call.ErrorCount,
		call.SpikeCount,
		int16(call.RecState), call.RecStateType,
		int16(call.CallState), call.CallStateType,
		call.CallFilename,
		int16(call.RetryAttempt),
		float32(call.ProcessCallTime),
	)
	if err != nil {
		return fmt.Errorf("update call end: %w", err)
	}

	p.activeCalls.Delete(call.ID)

	// Upsert talkgroup with latest data
	if call.Talkgroup > 0 {
		identity, err := p.identity.Resolve(ctx, msg.InstanceID, call.SysName)
		if err == nil {
			_ = p.db.UpsertTalkgroup(ctx, identity.SystemID, call.Talkgroup,
				call.TalkgroupAlphaTag, call.TalkgroupTag, call.TalkgroupGroup, call.TalkgroupDescription,
			)
		}
	}

	p.log.Debug().
		Str("tr_call_id", call.ID).
		Int64("call_id", entry.CallID).
		Float64("duration", call.Length).
		Msg("call ended")

	_ = startTime // used for logging context if needed
	return nil
}

// handleCallStartFromEnd creates a call record from a call_end message when we missed the call_start.
func (p *Pipeline) handleCallStartFromEnd(ctx context.Context, msg *CallEndMsg) error {
	call := &msg.Call
	startTime := time.Unix(call.StartTime, 0)

	identity, err := p.identity.Resolve(ctx, msg.InstanceID, call.SysName)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	freq := int64(call.Freq)
	duration := float32(call.Length)
	callNum := call.CallNum
	callState := int16(call.CallState)
	monState := int16(call.MonState)
	recState := int16(call.RecState)
	recNum := int16(call.RecNum)
	srcNum := int16(call.SrcNum)
	tdmaSlot := int16(call.TDMASlot)
	siteID := identity.SiteID
	freqError := call.FreqError
	signal := float32(call.Signal)
	noise := float32(call.Noise)
	stopTime := time.Unix(call.StopTime, 0)
	retryAttempt := int16(call.RetryAttempt)

	row := &database.CallRow{
		SystemID:      identity.SystemID,
		SiteID:        &siteID,
		Tgid:          call.Talkgroup,
		TrCallID:      call.ID,
		CallNum:       &callNum,
		StartTime:     startTime,
		StopTime:      &stopTime,
		Duration:      &duration,
		Freq:          &freq,
		FreqError:     &freqError,
		SignalDB:      &signal,
		NoiseDB:       &noise,
		ErrorCount:    &call.ErrorCount,
		SpikeCount:    &call.SpikeCount,
		AudioType:     call.AudioType,
		Phase2TDMA:    call.Phase2TDMA,
		TDMASlot:      &tdmaSlot,
		Analog:        call.Analog,
		Conventional:  call.Conventional,
		Encrypted:     call.Encrypted,
		Emergency:     call.Emergency,
		CallState:     &callState,
		CallStateType: call.CallStateType,
		MonState:      &monState,
		MonStateType:  call.MonStateType,
		RecState:      &recState,
		RecStateType:  call.RecStateType,
		RecNum:        &recNum,
		SrcNum:        &srcNum,
		SystemName:    call.SysName,
		SiteShortName: call.SysName,
		TgAlphaTag:    call.TalkgroupAlphaTag,
		TgDescription: call.TalkgroupDescription,
		TgTag:         call.TalkgroupTag,
		TgGroup:       call.TalkgroupGroup,
		InstanceID:    msg.InstanceID,
	}

	// Upsert talkgroup
	if call.Talkgroup > 0 {
		_ = p.db.UpsertTalkgroup(ctx, identity.SystemID, call.Talkgroup,
			call.TalkgroupAlphaTag, call.TalkgroupTag, call.TalkgroupGroup, call.TalkgroupDescription,
		)
	}

	// Upsert unit
	if call.Unit > 0 {
		_ = p.db.UpsertUnit(ctx, identity.SystemID, call.Unit,
			call.UnitAlphaTag, "call_end", startTime, call.Talkgroup,
		)
	}

	callID, err := p.db.InsertCall(ctx, row)
	if err != nil {
		return fmt.Errorf("insert call from end: %w", err)
	}

	// Create call group (same as handleCallStart)
	cgID, cgErr := p.db.UpsertCallGroup(ctx, identity.SystemID, call.Talkgroup, startTime,
		call.TalkgroupAlphaTag, call.TalkgroupDescription, call.TalkgroupTag, call.TalkgroupGroup,
	)
	if cgErr != nil {
		p.log.Warn().Err(cgErr).Msg("failed to upsert call group from call_end backfill")
	} else {
		_ = p.db.SetCallGroupID(ctx, callID, startTime, cgID)
		_ = p.db.SetCallGroupPrimary(ctx, cgID, callID)
	}

	_ = retryAttempt

	p.log.Debug().
		Str("tr_call_id", call.ID).
		Int64("call_id", callID).
		Msg("call inserted from call_end (missed call_start)")

	return nil
}

func (p *Pipeline) handleCallsActive(payload []byte) error {
	var msg CallsActiveMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	// Store as checkpoint for crash recovery
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	if err := p.db.InsertActiveCallCheckpoint(ctx, msg.InstanceID, payload, len(msg.Calls)); err != nil {
		p.log.Warn().Err(err).Msg("failed to insert active call checkpoint")
	}

	p.log.Debug().
		Int("active_calls", len(msg.Calls)).
		Str("instance_id", msg.InstanceID).
		Msg("calls_active checkpoint stored")

	return nil
}
