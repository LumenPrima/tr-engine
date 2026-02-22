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
	// Extract event type from topic: {prefix}/units/{sys_name}/{event_type}
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
			data.TalkgroupAlphaTag, data.TalkgroupTag, data.TalkgroupGroup, data.TalkgroupDescription, ts,
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

	// Dedup check: skip DB insert + SSE publish if an equivalent event was
	// already processed within the same 5-second window (multi-site dedup).
	isDup := false
	{
		dk := unitDedupKey{
			SystemID:   identity.SystemID,
			UnitID:     data.Unit,
			EventType:  eventType,
			Tgid:       data.Talkgroup,
			TimeBucket: ts.Unix() / 5,
		}
		if _, loaded := p.unitEventDedup.LoadOrStore(dk, time.Now()); loaded {
			isDup = true
		}
	}

	if !isDup {
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

		p.PublishEvent(EventData{
			Type:     "unit_event",
			SubType:  eventType,
			SystemID: identity.SystemID,
			SiteID:   identity.SiteID,
			Tgid:     data.Talkgroup,
			UnitID:   data.Unit,
			Payload: map[string]any{
				"event_type":     eventType,
				"system_id":      identity.SystemID,
				"unit_id":        data.Unit,
				"unit_alpha_tag": data.UnitAlphaTag,
				"tgid":           data.Talkgroup,
				"tg_alpha_tag":   data.TalkgroupAlphaTag,
				"time":           ts,
			},
		})
	}

	// Update affiliation map
	affKey := affiliationKey{SystemID: identity.SystemID, UnitID: data.Unit}
	switch eventType {
	case "join":
		if data.Talkgroup > 0 {
			var prevTgid *int
			if existing, ok := p.affiliations.Get(affKey); ok && existing.Tgid != data.Talkgroup {
				prevTgid = &existing.Tgid
			}
			p.affiliations.Update(affKey, &affiliationEntry{
				SystemID:        identity.SystemID,
				SystemName:      identity.SystemName,
				Sysid:           identity.Sysid,
				UnitID:          data.Unit,
				UnitAlphaTag:    data.UnitAlphaTag,
				Tgid:            data.Talkgroup,
				TgAlphaTag:      data.TalkgroupAlphaTag,
				TgDescription:   data.TalkgroupDescription,
				TgTag:           data.TalkgroupTag,
				TgGroup:         data.TalkgroupGroup,
				PreviousTgid:    prevTgid,
				AffiliatedSince: ts,
				LastEventTime:   ts,
				Status:          "affiliated",
			})
		}
	case "off":
		p.affiliations.MarkOff(affKey, ts)
	case "call", "end", "location":
		// These events carry the tgid the unit is currently on. If it differs
		// from the current affiliation, treat it as an implicit re-affiliation
		// (the join may have happened on a site we don't monitor).
		if data.Talkgroup > 0 {
			if existing, ok := p.affiliations.Get(affKey); ok && existing.Tgid != data.Talkgroup {
				prevTgid := existing.Tgid
				p.affiliations.Update(affKey, &affiliationEntry{
					SystemID:        identity.SystemID,
					SystemName:      identity.SystemName,
					Sysid:           identity.Sysid,
					UnitID:          data.Unit,
					UnitAlphaTag:    data.UnitAlphaTag,
					Tgid:            data.Talkgroup,
					TgAlphaTag:      data.TalkgroupAlphaTag,
					TgDescription:   data.TalkgroupDescription,
					TgTag:           data.TalkgroupTag,
					TgGroup:         data.TalkgroupGroup,
					PreviousTgid:    &prevTgid,
					AffiliatedSince: ts,
					LastEventTime:   ts,
					Status:          "affiliated",
				})
			} else {
				p.affiliations.UpdateActivity(affKey, ts)
			}
		} else {
			p.affiliations.UpdateActivity(affKey, ts)
		}
	default:
		p.affiliations.UpdateActivity(affKey, ts)
	}

	return nil
}
