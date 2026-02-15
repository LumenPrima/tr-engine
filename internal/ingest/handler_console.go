package ingest

import (
	"context"
	"encoding/json"
	"time"
)

func (p *Pipeline) handleConsoleLog(payload []byte) error {
	var msg ConsoleLogMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	mqttTS := time.Unix(msg.Timestamp, 0)
	data := msg.Console

	// Parse the ISO 8601 log time from TR
	logTime, err := time.Parse("2006-01-02T15:04:05.999999", data.Time)
	if err != nil {
		logTime = mqttTS // fallback to MQTT timestamp
	}

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	if err := p.db.InsertConsoleMessage(ctx, msg.InstanceID, logTime, data.Severity, data.LogMsg, mqttTS); err != nil {
		return err
	}

	p.PublishEvent(EventData{
		Type: "console",
		Payload: map[string]any{
			"instance_id": msg.InstanceID,
			"severity":    data.Severity,
			"log_msg":     data.LogMsg,
			"time":        logTime,
		},
	})

	return nil
}
