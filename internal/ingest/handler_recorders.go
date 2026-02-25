package ingest

import (
	"encoding/json"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleRecorders(payload []byte) error {
	var msg RecordersMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)

	for _, rec := range msg.Recorders {
		p.processRecorder(msg.InstanceID, rec, ts)
	}

	return nil
}

func (p *Pipeline) handleRecorder(payload []byte) error {
	var msg RecorderMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)
	p.processRecorder(msg.InstanceID, msg.Recorder, ts)
	return nil
}

func (p *Pipeline) processRecorder(instanceID string, rec RecorderData, ts time.Time) {
	row := database.RecorderSnapshotRow{
		InstanceID:   instanceID,
		RecorderID:   rec.ID,
		SrcNum:       int16(rec.SrcNum),
		RecNum:       int16(rec.RecNum),
		Type:         rec.Type,
		RecState:     int16(rec.RecState),
		RecStateType: rec.RecStateType,
		Freq:         int64(rec.Freq),
		Duration:     float32(rec.Duration),
		Count:        rec.Count,
		Squelched:    rec.Squelched,
		Time:         ts,
	}
	p.recorderBatcher.Add(row)
	p.UpdateRecorderCache(instanceID, row)

	payload := map[string]any{
		"id":          rec.ID,
		"instance_id": instanceID,
		"src_num":     rec.SrcNum,
		"rec_num":     rec.RecNum,
		"type":        rec.Type,
		"rec_state":   rec.RecStateType,
		"freq":        int64(rec.Freq),
		"duration":    rec.Duration,
		"count":       rec.Count,
		"squelched":   rec.Squelched,
	}

	// Enrich with active call data by matching frequency
	freq := int64(rec.Freq)
	if freq > 0 {
		if call, ok := p.activeCalls.FindByFreq(freq); ok {
			payload["tgid"] = call.Tgid
			payload["tg_alpha_tag"] = call.TgAlphaTag
			payload["unit_id"] = call.Unit
			payload["unit_alpha_tag"] = call.UnitAlphaTag
		}
	}

	p.PublishEvent(EventData{
		Type:    "recorder_update",
		Payload: payload,
	})
}
