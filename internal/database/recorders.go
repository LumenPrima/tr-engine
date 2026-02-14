package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{
			r.InstanceID, r.RecorderID, r.SrcNum, r.RecNum, r.Type,
			r.RecState, r.RecStateType, r.Freq, r.Duration, r.Count,
			r.Squelched, r.Time,
		}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"recorder_snapshots"},
		[]string{
			"instance_id", "recorder_id", "src_num", "rec_num", "type",
			"rec_state", "rec_state_type", "freq", "duration", "count",
			"squelched", "time",
		},
		pgx.CopyFromRows(copyRows),
	)
}

type DecodeRateRow struct {
	SystemID            *int
	DecodeRate          float32
	DecodeRateInterval  float32
	ControlChannel      int64
	SysNum              int16
	SysName             string
	Time                time.Time
	InstanceID          string
}

// InsertDecodeRates batch-inserts decode rates.
func (db *DB) InsertDecodeRates(ctx context.Context, rows []DecodeRateRow) (int64, error) {
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{
			r.SystemID, r.DecodeRate, r.DecodeRateInterval,
			r.ControlChannel, r.SysNum, r.SysName, r.Time, r.InstanceID,
		}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"decode_rates"},
		[]string{
			"system_id", "decode_rate", "decode_rate_interval",
			"control_channel", "sys_num", "sys_name", "time", "instance_id",
		},
		pgx.CopyFromRows(copyRows),
	)
}
