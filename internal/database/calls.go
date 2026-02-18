package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type CallRow struct {
	SystemID       int
	SiteID         *int
	Tgid           int
	TrCallID       string
	CallNum        *int
	StartTime      time.Time
	StopTime       *time.Time
	Duration       *float32
	Freq           *int64
	FreqError      *int
	SignalDB       *float32
	NoiseDB        *float32
	ErrorCount     *int
	SpikeCount     *int
	AudioType      string
	Phase2TDMA     bool
	TDMASlot       *int16
	Analog         bool
	Conventional   bool
	Encrypted      bool
	Emergency      bool
	CallState      *int16
	CallStateType  string
	MonState       *int16
	MonStateType   string
	RecState       *int16
	RecStateType   string
	RecNum         *int16
	SrcNum         *int16
	PatchedTgids   []int32
	SrcList        json.RawMessage
	FreqList       json.RawMessage
	UnitIDs        []int32
	SystemName     string
	SiteShortName  string
	TgAlphaTag     string
	TgDescription  string
	TgTag          string
	TgGroup        string
	InstanceID     string
}

// InsertCall inserts a new call and returns its call_id.
func (db *DB) InsertCall(ctx context.Context, c *CallRow) (int64, error) {
	var callID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO calls (
			system_id, site_id, tgid, tr_call_id, call_num,
			start_time, stop_time, duration, freq, freq_error,
			signal_db, noise_db, error_count, spike_count,
			audio_type, phase2_tdma, tdma_slot, analog, conventional,
			encrypted, emergency,
			call_state, call_state_type, mon_state, mon_state_type,
			rec_state, rec_state_type, rec_num, src_num,
			patched_tgids,
			src_list, freq_list, unit_ids,
			system_name, site_short_name,
			tg_alpha_tag, tg_description, tg_tag, tg_group,
			instance_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18, $19,
			$20, $21,
			$22, $23, $24, $25,
			$26, $27, $28, $29,
			$30,
			$31, $32, $33,
			$34, $35,
			$36, $37, $38, $39,
			$40
		) RETURNING call_id
	`,
		c.SystemID, c.SiteID, c.Tgid, c.TrCallID, c.CallNum,
		c.StartTime, c.StopTime, c.Duration, c.Freq, c.FreqError,
		c.SignalDB, c.NoiseDB, c.ErrorCount, c.SpikeCount,
		c.AudioType, c.Phase2TDMA, c.TDMASlot, c.Analog, c.Conventional,
		c.Encrypted, c.Emergency,
		c.CallState, c.CallStateType, c.MonState, c.MonStateType,
		c.RecState, c.RecStateType, c.RecNum, c.SrcNum,
		c.PatchedTgids,
		c.SrcList, c.FreqList, c.UnitIDs,
		c.SystemName, c.SiteShortName,
		c.TgAlphaTag, c.TgDescription, c.TgTag, c.TgGroup,
		c.InstanceID,
	).Scan(&callID)
	return callID, err
}

// UpdateCallEnd updates a call with end-of-call data.
func (db *DB) UpdateCallEnd(ctx context.Context, callID int64, startTime time.Time,
	stopTime time.Time, duration float32, freq int64, freqError int,
	signalDB, noiseDB float32, errorCount, spikeCount int,
	recState int16, recStateType string, callState int16, callStateType string,
	callFilename string, retryAttempt int16, processCallTime float32,
) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET
			stop_time = $3,
			duration = $4,
			freq = $5,
			freq_error = $6,
			signal_db = $7,
			noise_db = $8,
			error_count = $9,
			spike_count = $10,
			rec_state = $11,
			rec_state_type = $12,
			call_state = $13,
			call_state_type = $14,
			call_filename = $15,
			retry_attempt = $16,
			process_call_time = $17
		WHERE call_id = $1 AND start_time = $2
	`,
		callID, startTime,
		stopTime, duration, freq, freqError,
		signalDB, noiseDB, errorCount, spikeCount,
		recState, recStateType, callState, callStateType,
		callFilename, retryAttempt, processCallTime,
	)
	return err
}

// UpdateCallElapsed updates a call's running duration from calls_active elapsed data.
func (db *DB) UpdateCallElapsed(ctx context.Context, callID int64, startTime time.Time, stopTime *time.Time, duration *float32) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET
			stop_time = COALESCE($3, stop_time),
			duration = COALESCE($4, duration),
			updated_at = now()
		WHERE call_id = $1 AND start_time = $2
			AND (duration IS NULL OR duration = 0)
	`,
		callID, startTime, stopTime, duration,
	)
	return err
}

// UpdateCallStartFields enriches an audio-created call with fields from call_start.
// This prevents duplicate calls when audio MQTT messages arrive before call_start.
func (db *DB) UpdateCallStartFields(ctx context.Context, callID int64, startTime time.Time,
	trCallID string, callNum int, instanceID string,
	callState int16, callStateType string,
	monState int16, monStateType string,
	recState int16, recStateType string,
) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET
			tr_call_id = $3,
			call_num = $4,
			instance_id = $5,
			call_state = $6,
			call_state_type = $7,
			mon_state = $8,
			mon_state_type = $9,
			rec_state = $10,
			rec_state_type = $11
		WHERE call_id = $1 AND start_time = $2
	`,
		callID, startTime,
		trCallID, callNum, instanceID,
		callState, callStateType,
		monState, monStateType,
		recState, recStateType,
	)
	return err
}

// UpdateCallAudio updates a call with audio file path and size.
func (db *DB) UpdateCallAudio(ctx context.Context, callID int64, startTime time.Time, audioPath string, audioSize int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET
			audio_file_path = $3,
			audio_file_size = $4
		WHERE call_id = $1 AND start_time = $2
	`, callID, startTime, audioPath, audioSize)
	return err
}

// UpdateCallFilename sets the call_filename field (TR's original audio file path).
func (db *DB) UpdateCallFilename(ctx context.Context, callID int64, startTime time.Time, callFilename string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET call_filename = $3
		WHERE call_id = $1 AND start_time = $2
	`, callID, startTime, callFilename)
	return err
}

// UpsertCallGroup creates or finds a call group and returns its id.
func (db *DB) UpsertCallGroup(ctx context.Context, systemID, tgid int, startTime time.Time,
	tgAlphaTag, tgDescription, tgTag, tgGroup string,
) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO call_groups (system_id, tgid, start_time, tg_alpha_tag, tg_description, tg_tag, tg_group)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (system_id, tgid, start_time) DO UPDATE SET
			tg_alpha_tag   = COALESCE(NULLIF($4, ''), call_groups.tg_alpha_tag),
			tg_description = COALESCE(NULLIF($5, ''), call_groups.tg_description)
		RETURNING id
	`, systemID, tgid, startTime, tgAlphaTag, tgDescription, tgTag, tgGroup).Scan(&id)
	return id, err
}

// SetCallGroupID sets the call_group_id on a call.
func (db *DB) SetCallGroupID(ctx context.Context, callID int64, startTime time.Time, callGroupID int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET call_group_id = $3
		WHERE call_id = $1 AND start_time = $2
	`, callID, startTime, callGroupID)
	return err
}

// SetCallGroupPrimary sets the primary_call_id on a call group.
func (db *DB) SetCallGroupPrimary(ctx context.Context, callGroupID int, callID int64) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE call_groups SET primary_call_id = $2
		WHERE id = $1 AND primary_call_id IS NULL
	`, callGroupID, callID)
	return err
}

// UpdateCallSrcFreq updates a call with srcList, freqList, and unit_ids JSONB columns.
func (db *DB) UpdateCallSrcFreq(ctx context.Context, callID int64, startTime time.Time,
	srcList json.RawMessage, freqList json.RawMessage, unitIDs []int32) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE calls SET
			src_list = $3,
			freq_list = $4,
			unit_ids = $5
		WHERE call_id = $1 AND start_time = $2
	`, callID, startTime, srcList, freqList, unitIDs)
	return err
}

type CallFrequencyRow struct {
	CallID        int64
	CallStartTime time.Time
	Freq          int64
	Time          *time.Time
	Pos           *float32
	Len           *float32
	ErrorCount    *int
	SpikeCount    *int
}

// InsertCallFrequencies batch-inserts call frequency records.
func (db *DB) InsertCallFrequencies(ctx context.Context, rows []CallFrequencyRow) (int64, error) {
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{
			r.CallID, r.CallStartTime, r.Freq,
			r.Time, r.Pos, r.Len,
			r.ErrorCount, r.SpikeCount,
		}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"call_frequencies"},
		[]string{
			"call_id", "call_start_time", "freq",
			"time", "pos", "len",
			"error_count", "spike_count",
		},
		pgx.CopyFromRows(copyRows),
	)
}

type CallTransmissionRow struct {
	CallID        int64
	CallStartTime time.Time
	Src           int
	Time          *time.Time
	Pos           *float32
	Duration      *float32
	Emergency     int16
	SignalSystem  string
	Tag           string
}

// InsertCallTransmissions batch-inserts call transmission records.
func (db *DB) InsertCallTransmissions(ctx context.Context, rows []CallTransmissionRow) (int64, error) {
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{
			r.CallID, r.CallStartTime, r.Src,
			r.Time, r.Pos, r.Duration, r.Emergency,
			r.SignalSystem, r.Tag,
		}
	}

	return db.Pool.CopyFrom(ctx,
		pgx.Identifier{"call_transmissions"},
		[]string{
			"call_id", "call_start_time", "src",
			"time", "pos", "duration", "emergency",
			"signal_system", "tag",
		},
		pgx.CopyFromRows(copyRows),
	)
}

// InsertActiveCallCheckpoint stores a snapshot of active calls for crash recovery.
func (db *DB) InsertActiveCallCheckpoint(ctx context.Context, instanceID string, activeCalls []byte, callCount int) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO call_active_checkpoints (instance_id, active_calls, call_count)
		VALUES ($1, $2, $3)
	`, instanceID, activeCalls, callCount)
	return err
}

// PurgeStaleCalls deletes RECORDING calls older than maxAge that never received
// audio or a call_end. These are orphaned call_start records. Returns the number deleted.
func (db *DB) PurgeStaleCalls(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM calls
		WHERE rec_state_type = 'RECORDING'
			AND audio_file_path IS NULL
			AND (stop_time IS NULL OR duration IS NULL OR duration = 0)
			AND start_time < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PurgeOrphanCallGroups deletes call_groups with no remaining calls. Returns count deleted.
func (db *DB) PurgeOrphanCallGroups(ctx context.Context) (int64, error) {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM call_groups cg
		WHERE NOT EXISTS (SELECT 1 FROM calls c WHERE c.call_group_id = cg.id)
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// FindCallByTrCallID finds a call by its trunk-recorder call ID.
func (db *DB) FindCallByTrCallID(ctx context.Context, trCallID string) (int64, time.Time, error) {
	var callID int64
	var startTime time.Time
	err := db.Pool.QueryRow(ctx, `
		SELECT call_id, start_time FROM calls
		WHERE tr_call_id = $1
		ORDER BY start_time DESC
		LIMIT 1
	`, trCallID).Scan(&callID, &startTime)
	return callID, startTime, err
}

// FindCallForAudio finds a call matching the audio metadata.
// Uses fuzzy start_time matching (±5s) to handle trunk-recorder shifting
// start_time by 1-2s between call_start/call_end and audio messages.
func (db *DB) FindCallForAudio(ctx context.Context, systemID, tgid int, startTime time.Time) (int64, time.Time, error) {
	var callID int64
	var st time.Time
	err := db.Pool.QueryRow(ctx, `
		SELECT call_id, start_time FROM calls
		WHERE system_id = $1 AND tgid = $2
			AND start_time BETWEEN $3::timestamptz - interval '5 seconds' AND $3::timestamptz + interval '5 seconds'
		ORDER BY ABS(EXTRACT(EPOCH FROM (start_time - $3::timestamptz)))
		LIMIT 1
	`, systemID, tgid, startTime).Scan(&callID, &st)
	return callID, st, err
}
