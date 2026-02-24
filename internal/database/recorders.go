package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

type RecorderSnapshotRow struct {
	InstanceID   string
	RecorderID   string
	SrcNum       int16
	RecNum       int16
	Type         string
	RecState     int16
	RecStateType string
	Freq         int64
	Duration     float32
	Count        int
	Squelched    bool
	Time         time.Time
}

// InsertRecorderSnapshots batch-inserts recorder snapshots using CopyFrom.
func (db *DB) InsertRecorderSnapshots(ctx context.Context, rows []RecorderSnapshotRow) (int64, error) {
	params := make([]sqlcdb.InsertRecorderSnapshotsParams, len(rows))
	for i, r := range rows {
		count := int32(r.Count)
		params[i] = sqlcdb.InsertRecorderSnapshotsParams{
			InstanceID:   &r.InstanceID,
			RecorderID:   &r.RecorderID,
			SrcNum:       &r.SrcNum,
			RecNum:       &r.RecNum,
			Type:         &r.Type,
			RecState:     &r.RecState,
			RecStateType: &r.RecStateType,
			Freq:         &r.Freq,
			Duration:     &r.Duration,
			Count:        &count,
			Squelched:    &r.Squelched,
			Time:         pgtype.Timestamptz{Time: r.Time, Valid: true},
		}
	}
	return db.Q.InsertRecorderSnapshots(ctx, params)
}

type DecodeRateRow struct {
	SystemID           *int
	DecodeRate         float32
	DecodeRateInterval float32
	ControlChannel     int64
	SysNum             int16
	SysName            string
	Time               time.Time
	InstanceID         string
}

// InsertDecodeRates batch-inserts decode rates.
func (db *DB) InsertDecodeRates(ctx context.Context, rows []DecodeRateRow) (int64, error) {
	params := make([]sqlcdb.InsertDecodeRatesParams, len(rows))
	for i, r := range rows {
		var sysID *int32
		if r.SystemID != nil {
			v := int32(*r.SystemID)
			sysID = &v
		}
		params[i] = sqlcdb.InsertDecodeRatesParams{
			SystemID:           sysID,
			DecodeRate:         &r.DecodeRate,
			DecodeRateInterval: &r.DecodeRateInterval,
			ControlChannel:     &r.ControlChannel,
			SysNum:             &r.SysNum,
			SysName:            &r.SysName,
			Time:               pgtype.Timestamptz{Time: r.Time, Valid: true},
			InstanceID:         &r.InstanceID,
		}
	}
	return db.Q.InsertDecodeRates(ctx, params)
}
