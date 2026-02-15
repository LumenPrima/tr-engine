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
		row := database.RecorderSnapshotRow{
			InstanceID:   msg.InstanceID,
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
		p.UpdateRecorderCache(msg.InstanceID, row)

		p.PublishEvent(EventData{
			Type: "recorder_update",
			Payload: map[string]any{
				"id":          rec.ID,
				"instance_id": msg.InstanceID,
				"src_num":     rec.SrcNum,
				"rec_num":     rec.RecNum,
				"type":        rec.Type,
				"rec_state":   rec.RecStateType,
				"freq":        int64(rec.Freq),
				"duration":    rec.Duration,
				"count":       rec.Count,
				"squelched":   rec.Squelched,
			},
		})
	}

	return nil
}

func (p *Pipeline) handleRecorder(payload []byte) error {
	var msg RecorderMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)
	rec := msg.Recorder

	row := database.RecorderSnapshotRow{
		InstanceID:   msg.InstanceID,
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
	p.UpdateRecorderCache(msg.InstanceID, row)

	p.PublishEvent(EventData{
		Type: "recorder_update",
		Payload: map[string]any{
			"id":          rec.ID,
			"instance_id": msg.InstanceID,
			"src_num":     rec.SrcNum,
			"rec_num":     rec.RecNum,
			"type":        rec.Type,
			"rec_state":   rec.RecStateType,
			"freq":        int64(rec.Freq),
			"duration":    rec.Duration,
			"count":       rec.Count,
			"squelched":   rec.Squelched,
		},
	})

	return nil
}
