package ingest

import (
	"context"
	"encoding/json"
	"time"
)

func (p *Pipeline) handleStatus(payload []byte) error {
	var msg StatusMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	if err := p.db.InsertPluginStatus(ctx, msg.ClientID, msg.InstanceID, msg.Status, ts); err != nil {
		return err
	}

	// Ensure instance is tracked (don't resolve system — status has no sys_name)
	if msg.InstanceID != "" {
		if _, err := p.db.UpsertInstance(ctx, msg.InstanceID); err != nil {
			p.log.Warn().Err(err).Str("instance_id", msg.InstanceID).Msg("failed to upsert instance from status")
		}
	}

	p.log.Debug().
		Str("instance_id", msg.InstanceID).
		Str("status", msg.Status).
		Msg("plugin status recorded")

	p.PublishEvent(EventData{
		Type: "console",
		Payload: map[string]any{
			"instance_id": msg.InstanceID,
			"status":      msg.Status,
			"time":        ts,
		},
	})

	return nil
}
