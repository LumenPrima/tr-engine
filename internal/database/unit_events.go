package database

import (
	"context"
	"time"
)

type UnitEventRow struct {
	EventType            string
	SystemID             int
	UnitRID              int
	Time                 time.Time
	Tgid                 *int
	UnitAlphaTag         string
	TgAlphaTag           string
	CallNum              *int
	Freq                 *int64
	StartTime            *time.Time
	StopTime             *time.Time
	Encrypted            *bool
	Emergency            *bool
	Position             *float32
	Length               *float32
	ErrorCount           *int
	SpikeCount           *int
	SampleCount          *int
	TransmissionFilename string
	TalkgroupPatches     []int32
	InstanceID           string
	SysNum               *int16
	SysName              string
}

func (db *DB) InsertUnitEvent(ctx context.Context, e *UnitEventRow) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO unit_events (
			event_type, system_id, unit_rid, "time", tgid,
			unit_alpha_tag, tg_alpha_tag, call_num, freq,
			start_time, stop_time, encrypted, emergency,
			"position", length, error_count, spike_count, sample_count,
			transmission_filename, talkgroup_patches,
			instance_id, sys_num, sys_name
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20,
			$21, $22, $23
		)
	`,
		e.EventType, e.SystemID, e.UnitRID, e.Time, e.Tgid,
		e.UnitAlphaTag, e.TgAlphaTag, e.CallNum, e.Freq,
		e.StartTime, e.StopTime, e.Encrypted, e.Emergency,
		e.Position, e.Length, e.ErrorCount, e.SpikeCount, e.SampleCount,
		e.TransmissionFilename, e.TalkgroupPatches,
		e.InstanceID, e.SysNum, e.SysName,
	)
	return err
}
