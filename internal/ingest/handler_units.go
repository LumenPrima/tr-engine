package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleUnitEvent(topic string, payload []byte) error {
	// Extract event type from topic: trdash/units/{sys_name}/{event_type}
	parts := strings.Split(topic, "/")
	if len(parts) != 4 {
		return fmt.Errorf("invalid unit event topic: %s", topic)
	}
	eventType := parts[3]

	// Parse envelope
	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return err
	}

	// Parse the event-type-named field
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}

	eventJSON, ok := raw[eventType]
	if !ok {
		return fmt.Errorf("missing %q key in unit event payload", eventType)
	}

	var data UnitEventData
	if err := json.Unmarshal(eventJSON, &data); err != nil {
		return err
	}

	ts := time.Unix(env.Timestamp, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	// Resolve identity
	identity, err := p.identity.Resolve(ctx, env.InstanceID, data.SysName)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Upsert talkgroup if present
	if data.Talkgroup > 0 {
		if err := p.db.UpsertTalkgroup(ctx, identity.SystemID, data.Talkgroup,
			data.TalkgroupAlphaTag, data.TalkgroupTag, data.TalkgroupGroup, data.TalkgroupDescription,
		); err != nil {
			p.log.Warn().Err(err).Int("tgid", data.Talkgroup).Msg("failed to upsert talkgroup")
		}
	}

	// Upsert unit
	if err := p.db.UpsertUnit(ctx, identity.SystemID, data.Unit,
		data.UnitAlphaTag, eventType, ts, data.Talkgroup,
	); err != nil {
		p.log.Warn().Err(err).Int("unit", data.Unit).Msg("failed to upsert unit")
	}

	// Build unit event row
	row := &database.UnitEventRow{
		EventType:    eventType,
		SystemID:     identity.SystemID,
		UnitRID:      data.Unit,
		Time:         ts,
		UnitAlphaTag: data.UnitAlphaTag,
		TgAlphaTag:   data.TalkgroupAlphaTag,
		InstanceID:   env.InstanceID,
		SysName:      data.SysName,
	}

	sysNum := int16(data.SysNum)
	row.SysNum = &sysNum

	if data.Talkgroup > 0 {
		tgid := data.Talkgroup
		row.Tgid = &tgid
	}

	if data.Freq > 0 {
		freq := int64(data.Freq)
		row.Freq = &freq
	}

	if data.CallNum > 0 {
		callNum := data.CallNum
		row.CallNum = &callNum
	}

	if data.StartTime > 0 {
		st := time.Unix(data.StartTime, 0)
		row.StartTime = &st
	}
	if data.StopTime > 0 {
		st := time.Unix(data.StopTime, 0)
		row.StopTime = &st
	}

	// Fields specific to "end" events
	if eventType == "end" || eventType == "call" {
		if data.Emergency {
			row.Emergency = &data.Emergency
		}
		if data.Encrypted {
			row.Encrypted = &data.Encrypted
		}
	}

	if eventType == "end" || eventType == "call" {
		pos := float32(data.Position)
		row.Position = &pos
		length := float32(data.Length)
		row.Length = &length
		row.ErrorCount = &data.ErrorCount
		row.SpikeCount = &data.SpikeCount
		row.SampleCount = &data.SampleCount
		row.TransmissionFilename = data.TransmissionFilename
	}

	if err := p.db.InsertUnitEvent(ctx, row); err != nil {
		return fmt.Errorf("insert unit event: %w", err)
	}

	return nil
}
