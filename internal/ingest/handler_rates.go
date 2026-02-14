package ingest

import (
	"context"
	"encoding/json"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleRates(payload []byte) error {
	var msg RatesMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)

	rows := make([]database.DecodeRateRow, 0, len(msg.Rates))
	for _, rate := range msg.Rates {
		var systemID *int
		if sid := p.identity.GetSystemIDForSysName(rate.SysName); sid != 0 {
			systemID = &sid
		}

		rows = append(rows, database.DecodeRateRow{
			SystemID:           systemID,
			DecodeRate:         float32(rate.DecodeRate),
			DecodeRateInterval: float32(rate.DecodeRateInterval),
			ControlChannel:     int64(rate.ControlChannel),
			SysNum:             int16(rate.SysNum),
			SysName:            rate.SysName,
			Time:               ts,
			InstanceID:         msg.InstanceID,
		})
	}

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	_, err := p.db.InsertDecodeRates(ctx, rows)
	return err
}
