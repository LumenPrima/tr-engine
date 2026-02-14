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
		p.recorderBatcher.Add(database.RecorderSnapshotRow{
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

	p.recorderBatcher.Add(database.RecorderSnapshotRow{
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
	})

	return nil
}
