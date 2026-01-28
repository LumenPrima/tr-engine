package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
)

// UpsertInstance creates or updates an instance
func (db *DB) UpsertInstance(ctx context.Context, instanceID, instanceKey string, configJSON json.RawMessage) (*models.Instance, error) {
	var inst models.Instance
	err := db.pool.QueryRow(ctx, `
		INSERT INTO instances (instance_id, instance_key, config_json, last_seen)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (instance_id) DO UPDATE SET
			instance_key = EXCLUDED.instance_key,
			config_json = EXCLUDED.config_json,
			last_seen = NOW()
		RETURNING id, instance_id, instance_key, first_seen, last_seen, config_json
	`, instanceID, instanceKey, configJSON).Scan(
		&inst.ID, &inst.InstanceID, &inst.InstanceKey, &inst.FirstSeen, &inst.LastSeen, &inst.ConfigJSON,
	)
	return &inst, err
}

// GetInstanceByID gets an instance by its string ID
func (db *DB) GetInstanceByID(ctx context.Context, instanceID string) (*models.Instance, error) {
	var inst models.Instance
	err := db.pool.QueryRow(ctx, `
		SELECT id, instance_id, instance_key, first_seen, last_seen, config_json
		FROM instances WHERE instance_id = $1
	`, instanceID).Scan(
		&inst.ID, &inst.InstanceID, &inst.InstanceKey, &inst.FirstSeen, &inst.LastSeen, &inst.ConfigJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &inst, err
}

// UpsertSource creates or updates a source
func (db *DB) UpsertSource(ctx context.Context, instanceID, sourceNum int, centerFreq int64, rate int, driver, device, antenna string, gain int, configJSON json.RawMessage) (*models.Source, error) {
	var src models.Source
	err := db.pool.QueryRow(ctx, `
		INSERT INTO sources (instance_id, source_num, center_freq, rate, driver, device, antenna, gain, config_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (instance_id, source_num) DO UPDATE SET
			center_freq = EXCLUDED.center_freq,
			rate = EXCLUDED.rate,
			driver = EXCLUDED.driver,
			device = EXCLUDED.device,
			antenna = EXCLUDED.antenna,
			gain = EXCLUDED.gain,
			config_json = EXCLUDED.config_json
		RETURNING id, instance_id, source_num, center_freq, rate, driver, device, antenna, gain, config_json
	`, instanceID, sourceNum, centerFreq, rate, driver, device, antenna, gain, configJSON).Scan(
		&src.ID, &src.InstanceID, &src.SourceNum, &src.CenterFreq, &src.Rate, &src.Driver, &src.Device, &src.Antenna, &src.Gain, &src.ConfigJSON,
	)
	return &src, err
}

// UpsertSystem creates or updates a system
func (db *DB) UpsertSystem(ctx context.Context, instanceID, sysNum int, shortName, systemType, sysID, wacn, nac string, rfss, siteID int, configJSON json.RawMessage) (*models.System, error) {
	var sys models.System
	err := db.pool.QueryRow(ctx, `
		INSERT INTO systems (instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (instance_id, sys_num) DO UPDATE SET
			short_name = EXCLUDED.short_name,
			system_type = EXCLUDED.system_type,
			sysid = EXCLUDED.sysid,
			wacn = EXCLUDED.wacn,
			nac = EXCLUDED.nac,
			rfss = EXCLUDED.rfss,
			site_id = EXCLUDED.site_id,
			config_json = EXCLUDED.config_json
		RETURNING id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json
	`, instanceID, sysNum, shortName, systemType, sysID, wacn, nac, rfss, siteID, configJSON).Scan(
		&sys.ID, &sys.InstanceID, &sys.SysNum, &sys.ShortName, &sys.SystemType, &sys.SysID, &sys.WACN, &sys.NAC, &sys.RFSS, &sys.SiteID, &sys.ConfigJSON,
	)
	return &sys, err
}

// GetSystemByShortName gets a system by its short name
func (db *DB) GetSystemByShortName(ctx context.Context, shortName string) (*models.System, error) {
	var sys models.System
	err := db.pool.QueryRow(ctx, `
		SELECT id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json
		FROM systems WHERE short_name = $1
	`, shortName).Scan(
		&sys.ID, &sys.InstanceID, &sys.SysNum, &sys.ShortName, &sys.SystemType, &sys.SysID, &sys.WACN, &sys.NAC, &sys.RFSS, &sys.SiteID, &sys.ConfigJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &sys, err
}

// GetSystemByInstanceAndNum gets a system by instance ID and system number
func (db *DB) GetSystemByInstanceAndNum(ctx context.Context, instanceID, sysNum int) (*models.System, error) {
	var sys models.System
	err := db.pool.QueryRow(ctx, `
		SELECT id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json
		FROM systems WHERE instance_id = $1 AND sys_num = $2
	`, instanceID, sysNum).Scan(
		&sys.ID, &sys.InstanceID, &sys.SysNum, &sys.ShortName, &sys.SystemType, &sys.SysID, &sys.WACN, &sys.NAC, &sys.RFSS, &sys.SiteID, &sys.ConfigJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &sys, err
}

// UpsertTalkgroup creates or updates a talkgroup
func (db *DB) UpsertTalkgroup(ctx context.Context, systemID, tgid int, alphaTag, description, group, tag string, priority int, mode string) (*models.Talkgroup, error) {
	var tg models.Talkgroup
	err := db.pool.QueryRow(ctx, `
		INSERT INTO talkgroups (system_id, tgid, alpha_tag, description, tg_group, tag, priority, mode, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (system_id, tgid) DO UPDATE SET
			alpha_tag = COALESCE(NULLIF(EXCLUDED.alpha_tag, ''), talkgroups.alpha_tag),
			description = COALESCE(NULLIF(EXCLUDED.description, ''), talkgroups.description),
			tg_group = COALESCE(NULLIF(EXCLUDED.tg_group, ''), talkgroups.tg_group),
			tag = COALESCE(NULLIF(EXCLUDED.tag, ''), talkgroups.tag),
			priority = COALESCE(NULLIF(EXCLUDED.priority, 0), talkgroups.priority),
			mode = COALESCE(NULLIF(EXCLUDED.mode, ''), talkgroups.mode),
			last_seen = NOW()
		RETURNING id, system_id, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen
	`, systemID, tgid, alphaTag, description, group, tag, priority, mode).Scan(
		&tg.ID, &tg.SystemID, &tg.TGID, &tg.AlphaTag, &tg.Description, &tg.Group, &tg.Tag, &tg.Priority, &tg.Mode, &tg.FirstSeen, &tg.LastSeen,
	)
	return &tg, err
}

// GetTalkgroup gets a talkgroup by system and TGID
func (db *DB) GetTalkgroup(ctx context.Context, systemID, tgid int) (*models.Talkgroup, error) {
	var tg models.Talkgroup
	err := db.pool.QueryRow(ctx, `
		SELECT id, system_id, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen
		FROM talkgroups WHERE system_id = $1 AND tgid = $2
	`, systemID, tgid).Scan(
		&tg.ID, &tg.SystemID, &tg.TGID, &tg.AlphaTag, &tg.Description, &tg.Group, &tg.Tag, &tg.Priority, &tg.Mode, &tg.FirstSeen, &tg.LastSeen,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &tg, err
}

// UpsertUnit creates or updates a unit
func (db *DB) UpsertUnit(ctx context.Context, systemID int, unitID int64, alphaTag, alphaTagSource string) (*models.Unit, error) {
	var unit models.Unit
	err := db.pool.QueryRow(ctx, `
		INSERT INTO units (system_id, unit_id, alpha_tag, alpha_tag_source, last_seen)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (system_id, unit_id) DO UPDATE SET
			alpha_tag = COALESCE(NULLIF(EXCLUDED.alpha_tag, ''), units.alpha_tag),
			alpha_tag_source = COALESCE(NULLIF(EXCLUDED.alpha_tag_source, ''), units.alpha_tag_source),
			last_seen = NOW()
		RETURNING id, system_id, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
	`, systemID, unitID, alphaTag, alphaTagSource).Scan(
		&unit.ID, &unit.SystemID, &unit.UnitID, &unit.AlphaTag, &unit.AlphaTagSource, &unit.FirstSeen, &unit.LastSeen,
	)
	return &unit, err
}

// GetUnit gets a unit by system and unit ID
func (db *DB) GetUnit(ctx context.Context, systemID int, unitID int64) (*models.Unit, error) {
	var unit models.Unit
	err := db.pool.QueryRow(ctx, `
		SELECT id, system_id, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
		FROM units WHERE system_id = $1 AND unit_id = $2
	`, systemID, unitID).Scan(
		&unit.ID, &unit.SystemID, &unit.UnitID, &unit.AlphaTag, &unit.AlphaTagSource, &unit.FirstSeen, &unit.LastSeen,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &unit, err
}

// UpsertRecorder creates or updates a recorder
func (db *DB) UpsertRecorder(ctx context.Context, instanceID int, sourceID *int, recNum int, recType string) (*models.Recorder, error) {
	var rec models.Recorder
	// Find existing recorder by instance + rec_num.
	// rec_num is globally unique per instance in trunk-recorder (sequentially assigned across sources).
	err := db.pool.QueryRow(ctx, `
		SELECT id, instance_id, source_id, rec_num, rec_type
		FROM recorders WHERE instance_id = $1 AND rec_num = $2
	`, instanceID, recNum).Scan(
		&rec.ID, &rec.InstanceID, &rec.SourceID, &rec.RecNum, &rec.RecType,
	)
	if err == pgx.ErrNoRows {
		// Insert new recorder
		err = db.pool.QueryRow(ctx, `
			INSERT INTO recorders (instance_id, source_id, rec_num, rec_type)
			VALUES ($1, $2, $3, $4)
			RETURNING id, instance_id, source_id, rec_num, rec_type
		`, instanceID, sourceID, recNum, recType).Scan(
			&rec.ID, &rec.InstanceID, &rec.SourceID, &rec.RecNum, &rec.RecType,
		)
		return &rec, err
	}
	if err != nil {
		return nil, err
	}
	// Update existing — backfill source_id if we now have it, update rec_type
	_, err = db.pool.Exec(ctx, `
		UPDATE recorders SET
			source_id = COALESCE($1, source_id),
			rec_type = $2
		WHERE id = $3
	`, sourceID, recType, rec.ID)
	if sourceID != nil {
		rec.SourceID = sourceID
	}
	rec.RecType = recType
	return &rec, err
}

// InsertCall inserts a new call record
func (db *DB) InsertCall(ctx context.Context, call *models.Call) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO calls (
			call_group_id, instance_id, system_id, talkgroup_id, recorder_id,
			tr_call_id, call_num, start_time, stop_time, duration,
			call_state, mon_state, encrypted, emergency, phase2_tdma, tdma_slot,
			conventional, analog, audio_type, freq, freq_error, error_count, spike_count,
			signal_db, noise_db, audio_path, audio_size, patched_tgids, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27, $28, $29
		) RETURNING id
	`,
		call.CallGroupID, call.InstanceID, call.SystemID, call.TalkgroupID, call.RecorderID,
		call.TRCallID, call.CallNum, call.StartTime, call.StopTime, call.Duration,
		call.CallState, call.MonState, call.Encrypted, call.Emergency, call.Phase2TDMA, call.TDMASlot,
		call.Conventional, call.Analog, call.AudioType, call.Freq, call.FreqError, call.ErrorCount, call.SpikeCount,
		call.SignalDB, call.NoiseDB, call.AudioPath, call.AudioSize, call.PatchedTGIDs, call.MetadataJSON,
	).Scan(&call.ID)
}

// UpdateCall updates a call record
func (db *DB) UpdateCall(ctx context.Context, call *models.Call) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE calls SET
			call_group_id = $1, stop_time = $2, duration = $3,
			call_state = $4, mon_state = $5, encrypted = $6, emergency = $7,
			error_count = $8, spike_count = $9, signal_db = $10, noise_db = $11,
			audio_path = $12, audio_size = $13, metadata_json = $14,
			tr_call_id = COALESCE(NULLIF($15, ''), tr_call_id)
		WHERE id = $16 AND start_time = $17
	`,
		call.CallGroupID, call.StopTime, call.Duration,
		call.CallState, call.MonState, call.Encrypted, call.Emergency,
		call.ErrorCount, call.SpikeCount, call.SignalDB, call.NoiseDB,
		call.AudioPath, call.AudioSize, call.MetadataJSON,
		call.TRCallID, call.ID, call.StartTime,
	)
	return err
}

// GetCallByTRID gets a call by its trunk-recorder ID
func (db *DB) GetCallByTRID(ctx context.Context, trCallID string, startTime time.Time) (*models.Call, error) {
	var call models.Call
	err := db.pool.QueryRow(ctx, `
		SELECT id, call_group_id, instance_id, system_id, talkgroup_id, recorder_id,
			tr_call_id, call_num, start_time, stop_time, duration,
			call_state, mon_state, encrypted, emergency, phase2_tdma, tdma_slot,
			conventional, analog, audio_type, freq, freq_error, error_count, spike_count,
			signal_db, noise_db, audio_path, audio_size, patched_tgids, metadata_json
		FROM calls WHERE tr_call_id = $1 AND start_time >= ($2::timestamptz - INTERVAL '1 hour')
		ORDER BY start_time DESC LIMIT 1
	`, trCallID, startTime).Scan(
		&call.ID, &call.CallGroupID, &call.InstanceID, &call.SystemID, &call.TalkgroupID, &call.RecorderID,
		&call.TRCallID, &call.CallNum, &call.StartTime, &call.StopTime, &call.Duration,
		&call.CallState, &call.MonState, &call.Encrypted, &call.Emergency, &call.Phase2TDMA, &call.TDMASlot,
		&call.Conventional, &call.Analog, &call.AudioType, &call.Freq, &call.FreqError, &call.ErrorCount, &call.SpikeCount,
		&call.SignalDB, &call.NoiseDB, &call.AudioPath, &call.AudioSize, &call.PatchedTGIDs, &call.MetadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &call, err
}

// GetCallByTGIDAndTime gets a call by system, tgid, and approximate start time
func (db *DB) GetCallByTGIDAndTime(ctx context.Context, systemID, tgid int, startTime time.Time) (*models.Call, error) {
	var call models.Call
	// Look for calls within 60 seconds of the given start time
	// Match by system and talkgroup (via join), prefer exact tgid match but also try without
	err := db.pool.QueryRow(ctx, `
		SELECT c.id, c.call_group_id, c.instance_id, c.system_id, c.talkgroup_id, c.recorder_id,
			c.tr_call_id, c.call_num, c.start_time, c.stop_time, c.duration,
			c.call_state, c.mon_state, c.encrypted, c.emergency, c.phase2_tdma, c.tdma_slot,
			c.conventional, c.analog, c.audio_type, c.freq, c.freq_error, c.error_count, c.spike_count,
			c.signal_db, c.noise_db, c.audio_path, c.audio_size, c.patched_tgids, c.metadata_json
		FROM calls c
		LEFT JOIN talkgroups t ON t.id = c.talkgroup_id
		WHERE c.system_id = $1
		AND c.start_time BETWEEN ($3::timestamptz - INTERVAL '60 seconds') AND ($3::timestamptz + INTERVAL '60 seconds')
		AND c.audio_path IS NULL
		ORDER BY
			CASE WHEN t.tgid = $2 THEN 0 ELSE 1 END,
			ABS(EXTRACT(EPOCH FROM (c.start_time - $3::timestamptz))) ASC
		LIMIT 1
	`, systemID, tgid, startTime).Scan(
		&call.ID, &call.CallGroupID, &call.InstanceID, &call.SystemID, &call.TalkgroupID, &call.RecorderID,
		&call.TRCallID, &call.CallNum, &call.StartTime, &call.StopTime, &call.Duration,
		&call.CallState, &call.MonState, &call.Encrypted, &call.Emergency, &call.Phase2TDMA, &call.TDMASlot,
		&call.Conventional, &call.Analog, &call.AudioType, &call.Freq, &call.FreqError, &call.ErrorCount, &call.SpikeCount,
		&call.SignalDB, &call.NoiseDB, &call.AudioPath, &call.AudioSize, &call.PatchedTGIDs, &call.MetadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &call, err
}

// CountRecentCallsWithoutAudio counts calls without audio for debugging
func (db *DB) CountRecentCallsWithoutAudio(ctx context.Context, systemID int, since time.Time) (total int, withoutAudio int, err error) {
	err = db.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE audio_path IS NULL) as without_audio
		FROM calls
		WHERE system_id = $1 AND start_time > $2
	`, systemID, since).Scan(&total, &withoutAudio)
	return
}

// GetCallBySystemTGIDAndTime finds a call by system, tgid and start_time (for linking call_end to audio-created calls)
// Unlike GetCallByTGIDAndTime, this doesn't filter by audio_path
func (db *DB) GetCallBySystemTGIDAndTime(ctx context.Context, systemID, tgid int, startTime time.Time) (*models.Call, error) {
	var call models.Call
	// Look for calls within 30 seconds of the given start time that match the tgid
	err := db.pool.QueryRow(ctx, `
		SELECT c.id, c.call_group_id, c.instance_id, c.system_id, c.talkgroup_id, c.recorder_id,
			c.tr_call_id, c.call_num, c.start_time, c.stop_time, c.duration,
			c.call_state, c.mon_state, c.encrypted, c.emergency, c.phase2_tdma, c.tdma_slot,
			c.conventional, c.analog, c.audio_type, c.freq, c.freq_error, c.error_count, c.spike_count,
			c.signal_db, c.noise_db, c.audio_path, c.audio_size, c.patched_tgids, c.metadata_json
		FROM calls c
		LEFT JOIN talkgroups t ON t.id = c.talkgroup_id
		WHERE c.system_id = $1
		AND t.tgid = $2
		AND c.start_time BETWEEN ($3::timestamptz - INTERVAL '30 seconds') AND ($3::timestamptz + INTERVAL '30 seconds')
		ORDER BY ABS(EXTRACT(EPOCH FROM (c.start_time - $3::timestamptz))) ASC
		LIMIT 1
	`, systemID, tgid, startTime).Scan(
		&call.ID, &call.CallGroupID, &call.InstanceID, &call.SystemID, &call.TalkgroupID, &call.RecorderID,
		&call.TRCallID, &call.CallNum, &call.StartTime, &call.StopTime, &call.Duration,
		&call.CallState, &call.MonState, &call.Encrypted, &call.Emergency, &call.Phase2TDMA, &call.TDMASlot,
		&call.Conventional, &call.Analog, &call.AudioType, &call.Freq, &call.FreqError, &call.ErrorCount, &call.SpikeCount,
		&call.SignalDB, &call.NoiseDB, &call.AudioPath, &call.AudioSize, &call.PatchedTGIDs, &call.MetadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &call, err
}

// InsertCallGroup inserts a new call group
func (db *DB) InsertCallGroup(ctx context.Context, group *models.CallGroup) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO call_groups (system_id, talkgroup_id, tgid, start_time, end_time, primary_call_id, call_count, encrypted, emergency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`,
		group.SystemID, group.TalkgroupID, group.TGID, group.StartTime, group.EndTime,
		group.PrimaryCallID, group.CallCount, group.Encrypted, group.Emergency,
	).Scan(&group.ID)
}

// UpdateCallGroup updates a call group
func (db *DB) UpdateCallGroup(ctx context.Context, group *models.CallGroup) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE call_groups SET
			end_time = $1, primary_call_id = $2, call_count = $3, encrypted = $4, emergency = $5
		WHERE id = $6
	`, group.EndTime, group.PrimaryCallID, group.CallCount, group.Encrypted, group.Emergency, group.ID)
	return err
}

// FindCallGroupCandidates finds potential duplicate call groups
// For P25 systems, matches on WACN + SysID to detect same logical system across different sites/NACs
func (db *DB) FindCallGroupCandidates(ctx context.Context, systemID, tgid int, startTime time.Time, windowSeconds int) ([]*models.CallGroup, error) {
	// First get the WACN and SysID of the incoming call's system
	var wacn, sysid string
	err := db.pool.QueryRow(ctx, `SELECT COALESCE(wacn, ''), COALESCE(sysid, '') FROM systems WHERE id = $1`, systemID).Scan(&wacn, &sysid)
	if err != nil {
		// If we can't get system info, fall back to simple TGID match
		wacn, sysid = "", ""
	}

	var rows pgx.Rows
	if wacn != "" && sysid != "" {
		// P25 system: match on WACN + SysID (same logical system, different sites)
		rows, err = db.pool.Query(ctx, `
			SELECT cg.id, cg.system_id, cg.talkgroup_id, cg.tgid, cg.start_time, cg.end_time,
			       cg.primary_call_id, cg.call_count, cg.encrypted, cg.emergency
			FROM call_groups cg
			JOIN systems s ON s.id = cg.system_id
			WHERE cg.tgid = $1
			AND cg.start_time BETWEEN ($2::timestamptz - make_interval(secs => $3)) AND ($2::timestamptz + make_interval(secs => $3))
			AND s.wacn = $4 AND s.sysid = $5
		`, tgid, startTime, windowSeconds, wacn, sysid)
	} else {
		// Non-P25 or missing info: match on TGID only within time window
		rows, err = db.pool.Query(ctx, `
			SELECT id, system_id, talkgroup_id, tgid, start_time, end_time, primary_call_id, call_count, encrypted, emergency
			FROM call_groups
			WHERE tgid = $1
			AND start_time BETWEEN ($2::timestamptz - make_interval(secs => $3)) AND ($2::timestamptz + make_interval(secs => $3))
		`, tgid, startTime, windowSeconds)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*models.CallGroup
	for rows.Next() {
		var g models.CallGroup
		if err := rows.Scan(
			&g.ID, &g.SystemID, &g.TalkgroupID, &g.TGID, &g.StartTime, &g.EndTime,
			&g.PrimaryCallID, &g.CallCount, &g.Encrypted, &g.Emergency,
		); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

// IsP25System checks if a system has WACN and SysID set (indicating P25 with cross-site dedup capability)
func (db *DB) IsP25System(ctx context.Context, systemID int) (bool, error) {
	var wacn, sysid string
	err := db.pool.QueryRow(ctx, `SELECT COALESCE(wacn, ''), COALESCE(sysid, '') FROM systems WHERE id = $1`, systemID).Scan(&wacn, &sysid)
	if err != nil {
		return false, err
	}
	return wacn != "" && sysid != "", nil
}

// InsertTransmission inserts a transmission record
func (db *DB) InsertTransmission(ctx context.Context, tx *models.Transmission) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO transmissions (call_id, unit_id, unit_rid, start_time, stop_time, duration, position, emergency, error_count, spike_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`,
		tx.CallID, tx.UnitID, tx.UnitRID, tx.StartTime, tx.StopTime, tx.Duration, tx.Position, tx.Emergency, tx.ErrorCount, tx.SpikeCount,
	).Scan(&tx.ID)
}

// InsertCallFrequency inserts a call frequency record
func (db *DB) InsertCallFrequency(ctx context.Context, cf *models.CallFrequency) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO call_frequencies (call_id, freq, time, position, duration, error_count, spike_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`,
		cf.CallID, cf.Freq, cf.Time, cf.Position, cf.Duration, cf.ErrorCount, cf.SpikeCount,
	).Scan(&cf.ID)
}

// GetTransmissionsByCallID returns all transmissions for a call, ordered by position
func (db *DB) GetTransmissionsByCallID(ctx context.Context, callID int64) ([]*models.Transmission, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT t.id, t.call_id, t.unit_id, t.unit_rid, t.start_time, t.stop_time,
			t.duration, t.position, t.emergency, t.error_count, t.spike_count
		FROM transmissions t
		WHERE t.call_id = $1
		ORDER BY t.position ASC
	`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []*models.Transmission
	for rows.Next() {
		var tx models.Transmission
		if err := rows.Scan(
			&tx.ID, &tx.CallID, &tx.UnitID, &tx.UnitRID, &tx.StartTime, &tx.StopTime,
			&tx.Duration, &tx.Position, &tx.Emergency, &tx.ErrorCount, &tx.SpikeCount,
		); err != nil {
			return nil, err
		}
		txs = append(txs, &tx)
	}
	return txs, rows.Err()
}

// GetFrequenciesByCallID returns all frequency entries for a call, ordered by position
func (db *DB) GetFrequenciesByCallID(ctx context.Context, callID int64) ([]*models.CallFrequency, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, call_id, freq, time, position, duration, error_count, spike_count
		FROM call_frequencies
		WHERE call_id = $1
		ORDER BY position ASC
	`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var freqs []*models.CallFrequency
	for rows.Next() {
		var f models.CallFrequency
		if err := rows.Scan(
			&f.ID, &f.CallID, &f.Freq, &f.Time, &f.Position, &f.Duration, &f.ErrorCount, &f.SpikeCount,
		); err != nil {
			return nil, err
		}
		freqs = append(freqs, &f)
	}
	return freqs, rows.Err()
}

// InsertUnitEvent inserts a unit event
func (db *DB) InsertUnitEvent(ctx context.Context, event *models.UnitEvent) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO unit_events (instance_id, system_id, unit_id, unit_rid, event_type, talkgroup_id, tgid, time, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`,
		event.InstanceID, event.SystemID, event.UnitID, event.UnitRID, event.EventType, event.TalkgroupID, event.TGID, event.Time, event.MetadataJSON,
	).Scan(&event.ID)
}

// InsertSystemRate inserts a system rate record
func (db *DB) InsertSystemRate(ctx context.Context, rate *models.SystemRate) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO system_rates (system_id, time, decode_rate, control_channel)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, rate.SystemID, rate.Time, rate.DecodeRate, rate.ControlChannel).Scan(&rate.ID)
}

// InsertRecorderStatus inserts a recorder status snapshot
func (db *DB) InsertRecorderStatus(ctx context.Context, status *models.RecorderStatus) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO recorder_status (recorder_id, time, state, freq, call_count, duration, squelched)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, status.RecorderID, status.Time, status.State, status.Freq, status.CallCount, status.Duration, status.Squelched).Scan(&status.ID)
}

// InsertTrunkMessage inserts a trunk message
func (db *DB) InsertTrunkMessage(ctx context.Context, msg *models.TrunkMessage) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO trunk_messages (system_id, time, msg_type, msg_type_name, opcode, opcode_type, opcode_desc, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, msg.SystemID, msg.Time, msg.MsgType, msg.MsgTypeName, msg.Opcode, msg.OpcodeType, msg.OpcodeDesc, msg.Meta).Scan(&msg.ID)
}

// ListSystems returns all systems
func (db *DB) ListSystems(ctx context.Context) ([]*models.System, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json
		FROM systems ORDER BY short_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []*models.System
	for rows.Next() {
		var s models.System
		if err := rows.Scan(
			&s.ID, &s.InstanceID, &s.SysNum, &s.ShortName, &s.SystemType, &s.SysID, &s.WACN, &s.NAC, &s.RFSS, &s.SiteID, &s.ConfigJSON,
		); err != nil {
			return nil, err
		}
		systems = append(systems, &s)
	}
	return systems, rows.Err()
}

// GetSystemByID gets a system by its database ID
func (db *DB) GetSystemByID(ctx context.Context, id int) (*models.System, error) {
	var sys models.System
	err := db.pool.QueryRow(ctx, `
		SELECT id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id, config_json
		FROM systems WHERE id = $1
	`, id).Scan(
		&sys.ID, &sys.InstanceID, &sys.SysNum, &sys.ShortName, &sys.SystemType, &sys.SysID, &sys.WACN, &sys.NAC, &sys.RFSS, &sys.SiteID, &sys.ConfigJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &sys, err
}

// ListTalkgroupsBySystem returns talkgroups for a system
func (db *DB) ListTalkgroupsBySystem(ctx context.Context, systemID int, limit, offset int) ([]*models.Talkgroup, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, system_id, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen
		FROM talkgroups WHERE system_id = $1
		ORDER BY tgid
		LIMIT $2 OFFSET $3
	`, systemID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var talkgroups []*models.Talkgroup
	for rows.Next() {
		var t models.Talkgroup
		if err := rows.Scan(
			&t.ID, &t.SystemID, &t.TGID, &t.AlphaTag, &t.Description, &t.Group, &t.Tag, &t.Priority, &t.Mode, &t.FirstSeen, &t.LastSeen,
		); err != nil {
			return nil, err
		}
		talkgroups = append(talkgroups, &t)
	}
	return talkgroups, rows.Err()
}

// ListCalls returns calls with optional filters
func (db *DB) ListCalls(ctx context.Context, systemID *int, talkgroupID *int, startTime, endTime *time.Time, limit, offset int) ([]*models.Call, error) {
	query := `
		SELECT c.id, c.call_group_id, c.instance_id, c.system_id, c.talkgroup_id, c.recorder_id,
			c.tr_call_id, c.call_num, c.start_time, c.stop_time, c.duration,
			c.call_state, c.mon_state, c.encrypted, c.emergency, c.phase2_tdma, c.tdma_slot,
			c.conventional, c.analog, c.audio_type, c.freq, c.freq_error, c.error_count, c.spike_count,
			c.signal_db, c.noise_db, c.audio_path, c.audio_size, c.patched_tgids, c.metadata_json,
			tg.tgid, tg.alpha_tag
		FROM calls c
		LEFT JOIN talkgroups tg ON tg.id = c.talkgroup_id
		WHERE c.audio_path IS NOT NULL AND c.audio_path != ''`
	args := []interface{}{}
	argNum := 1

	if systemID != nil {
		query += fmt.Sprintf(" AND c.system_id = $%d", argNum)
		args = append(args, *systemID)
		argNum++
	}
	if talkgroupID != nil {
		query += fmt.Sprintf(" AND c.talkgroup_id = $%d", argNum)
		args = append(args, *talkgroupID)
		argNum++
	}
	if startTime != nil {
		query += fmt.Sprintf(" AND c.start_time >= $%d", argNum)
		args = append(args, *startTime)
		argNum++
	}
	if endTime != nil {
		query += fmt.Sprintf(" AND c.start_time <= $%d", argNum)
		args = append(args, *endTime)
		argNum++
	}

	query += fmt.Sprintf(" ORDER BY c.start_time DESC LIMIT $%d", argNum)
	args = append(args, limit)
	argNum++
	query += fmt.Sprintf(" OFFSET $%d", argNum)
	args = append(args, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []*models.Call
	for rows.Next() {
		var c models.Call
		if err := rows.Scan(
			&c.ID, &c.CallGroupID, &c.InstanceID, &c.SystemID, &c.TalkgroupID, &c.RecorderID,
			&c.TRCallID, &c.CallNum, &c.StartTime, &c.StopTime, &c.Duration,
			&c.CallState, &c.MonState, &c.Encrypted, &c.Emergency, &c.Phase2TDMA, &c.TDMASlot,
			&c.Conventional, &c.Analog, &c.AudioType, &c.Freq, &c.FreqError, &c.ErrorCount, &c.SpikeCount,
			&c.SignalDB, &c.NoiseDB, &c.AudioPath, &c.AudioSize, &c.PatchedTGIDs, &c.MetadataJSON,
			&c.TGID, &c.TGAlphaTag,
		); err != nil {
			return nil, err
		}
		calls = append(calls, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch units for all returned calls
	if len(calls) > 0 {
		callIDs := make([]int64, len(calls))
		callMap := make(map[int64]*models.Call, len(calls))
		for i, c := range calls {
			callIDs[i] = c.ID
			callMap[c.ID] = c
		}
		unitQuery := `
			SELECT DISTINCT t.call_id, t.unit_rid, COALESCE(u.alpha_tag, '') as alpha_tag
			FROM transmissions t
			LEFT JOIN units u ON u.id = t.unit_id
			WHERE t.call_id = ANY($1)
			ORDER BY t.call_id, t.unit_rid
		`
		unitRows, err := db.pool.Query(ctx, unitQuery, callIDs)
		if err == nil {
			defer unitRows.Close()
			for unitRows.Next() {
				var callID, unitRID int64
				var alphaTag string
				if err := unitRows.Scan(&callID, &unitRID, &alphaTag); err != nil {
					continue
				}
				if call, ok := callMap[callID]; ok {
					call.Units = append(call.Units, models.CallUnit{
						UnitRID:  unitRID,
						AlphaTag: alphaTag,
					})
				}
			}
		}
	}

	return calls, nil
}

// GetCallByID gets a call by its ID
func (db *DB) GetCallByID(ctx context.Context, id int64) (*models.Call, error) {
	var call models.Call
	err := db.pool.QueryRow(ctx, `
		SELECT id, call_group_id, instance_id, system_id, talkgroup_id, recorder_id,
			tr_call_id, call_num, start_time, stop_time, duration,
			call_state, mon_state, encrypted, emergency, phase2_tdma, tdma_slot,
			conventional, analog, audio_type, freq, freq_error, error_count, spike_count,
			signal_db, noise_db, audio_path, audio_size, patched_tgids, metadata_json
		FROM calls WHERE id = $1
	`, id).Scan(
		&call.ID, &call.CallGroupID, &call.InstanceID, &call.SystemID, &call.TalkgroupID, &call.RecorderID,
		&call.TRCallID, &call.CallNum, &call.StartTime, &call.StopTime, &call.Duration,
		&call.CallState, &call.MonState, &call.Encrypted, &call.Emergency, &call.Phase2TDMA, &call.TDMASlot,
		&call.Conventional, &call.Analog, &call.AudioType, &call.Freq, &call.FreqError, &call.ErrorCount, &call.SpikeCount,
		&call.SignalDB, &call.NoiseDB, &call.AudioPath, &call.AudioSize, &call.PatchedTGIDs, &call.MetadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &call, err
}

// GetCallByTRCallID returns a call by trunk-recorder's call ID
func (db *DB) GetCallByTRCallID(ctx context.Context, trCallID string) (*models.Call, error) {
	var call models.Call
	err := db.pool.QueryRow(ctx, `
		SELECT id, call_group_id, instance_id, system_id, talkgroup_id, recorder_id,
			tr_call_id, call_num, start_time, stop_time, duration,
			call_state, mon_state, encrypted, emergency, phase2_tdma, tdma_slot,
			conventional, analog, audio_type, freq, freq_error, error_count, spike_count,
			signal_db, noise_db, audio_path, audio_size, patched_tgids, metadata_json
		FROM calls WHERE tr_call_id = $1
		ORDER BY start_time DESC LIMIT 1
	`, trCallID).Scan(
		&call.ID, &call.CallGroupID, &call.InstanceID, &call.SystemID, &call.TalkgroupID, &call.RecorderID,
		&call.TRCallID, &call.CallNum, &call.StartTime, &call.StopTime, &call.Duration,
		&call.CallState, &call.MonState, &call.Encrypted, &call.Emergency, &call.Phase2TDMA, &call.TDMASlot,
		&call.Conventional, &call.Analog, &call.AudioType, &call.Freq, &call.FreqError, &call.ErrorCount, &call.SpikeCount,
		&call.SignalDB, &call.NoiseDB, &call.AudioPath, &call.AudioSize, &call.PatchedTGIDs, &call.MetadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &call, err
}

// ListUnits returns units with optional system filter
func (db *DB) ListUnits(ctx context.Context, systemID *int, limit, offset int) ([]*models.Unit, error) {
	var query string
	var args []interface{}
	if systemID != nil {
		query = `SELECT id, system_id, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
			FROM units WHERE system_id = $1 ORDER BY unit_id LIMIT $2 OFFSET $3`
		args = []interface{}{*systemID, limit, offset}
	} else {
		query = `SELECT id, system_id, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
			FROM units ORDER BY unit_id LIMIT $1 OFFSET $2`
		args = []interface{}{limit, offset}
	}

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []*models.Unit
	for rows.Next() {
		var u models.Unit
		if err := rows.Scan(&u.ID, &u.SystemID, &u.UnitID, &u.AlphaTag, &u.AlphaTagSource, &u.FirstSeen, &u.LastSeen); err != nil {
			return nil, err
		}
		units = append(units, &u)
	}
	return units, rows.Err()
}

// GetUnitByID gets a unit by its database ID
func (db *DB) GetUnitByID(ctx context.Context, id int) (*models.Unit, error) {
	var unit models.Unit
	err := db.pool.QueryRow(ctx, `
		SELECT id, system_id, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
		FROM units WHERE id = $1
	`, id).Scan(&unit.ID, &unit.SystemID, &unit.UnitID, &unit.AlphaTag, &unit.AlphaTagSource, &unit.FirstSeen, &unit.LastSeen)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &unit, err
}

// Stats holds database statistics
type Stats struct {
	SystemCount     int
	TalkgroupCount  int
	UnitCount       int
	TotalCalls      int64
	CallsLastMinute int
	ActiveUnits     int
}

// GetStats returns database statistics for the status display
func (db *DB) GetStats(ctx context.Context) (*Stats, error) {
	var stats Stats

	// Get system count
	err := db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM systems`).Scan(&stats.SystemCount)
	if err != nil {
		return nil, err
	}

	// Get talkgroup count
	err = db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM talkgroups`).Scan(&stats.TalkgroupCount)
	if err != nil {
		return nil, err
	}

	// Get total unit count
	err = db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM units`).Scan(&stats.UnitCount)
	if err != nil {
		return nil, err
	}

	// Get total calls
	err = db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM calls`).Scan(&stats.TotalCalls)
	if err != nil {
		return nil, err
	}

	// Get calls in last minute
	err = db.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM calls
		WHERE start_time >= NOW() - INTERVAL '1 minute'
	`).Scan(&stats.CallsLastMinute)
	if err != nil {
		return nil, err
	}

	// Get active units (seen in last 5 minutes)
	err = db.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM units
		WHERE last_seen >= NOW() - INTERVAL '5 minutes'
	`).Scan(&stats.ActiveUnits)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// ListUnitEvents returns unit events with optional filters
func (db *DB) ListUnitEvents(ctx context.Context, unitID *int, systemID *int, eventType *string, tgid *int, startTime, endTime *time.Time, limit, offset int) ([]*models.UnitEvent, error) {
	query := `SELECT id, instance_id, system_id, unit_id, unit_rid, event_type, talkgroup_id, tgid, time, metadata_json
		FROM unit_events WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	if unitID != nil {
		query += " AND unit_id = $" + strconv.Itoa(argNum)
		args = append(args, *unitID)
		argNum++
	}
	if systemID != nil {
		query += " AND system_id = $" + strconv.Itoa(argNum)
		args = append(args, *systemID)
		argNum++
	}
	if eventType != nil {
		query += " AND event_type = $" + strconv.Itoa(argNum)
		args = append(args, *eventType)
		argNum++
	}
	if tgid != nil {
		query += " AND tgid = $" + strconv.Itoa(argNum)
		args = append(args, *tgid)
		argNum++
	}
	if startTime != nil {
		query += " AND time >= $" + strconv.Itoa(argNum)
		args = append(args, *startTime)
		argNum++
	}
	if endTime != nil {
		query += " AND time <= $" + strconv.Itoa(argNum)
		args = append(args, *endTime)
		argNum++
	}

	query += " ORDER BY time DESC LIMIT $" + strconv.Itoa(argNum)
	args = append(args, limit)
	argNum++
	query += " OFFSET $" + strconv.Itoa(argNum)
	args = append(args, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*models.UnitEvent
	for rows.Next() {
		var e models.UnitEvent
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.SystemID, &e.UnitID, &e.UnitRID, &e.EventType, &e.TalkgroupID, &e.TGID, &e.Time, &e.MetadataJSON); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// ActiveCallFilters contains filters for active calls query
type ActiveCallFilters struct {
	SystemID    *int
	ShortName   *string
	TalkgroupID *int
	Emergency   *bool
	Encrypted   *bool
}

// ListActiveCalls returns calls that are currently active (no stop_time)
func (db *DB) ListActiveCalls(ctx context.Context, filters ActiveCallFilters, limit, offset int) ([]*models.Call, error) {
	query := `
		SELECT c.id, c.call_group_id, c.instance_id, c.system_id, c.talkgroup_id, c.recorder_id,
			c.tr_call_id, c.call_num, c.start_time, c.stop_time, c.duration,
			c.call_state, c.mon_state, c.encrypted, c.emergency, c.phase2_tdma, c.tdma_slot,
			c.conventional, c.analog, c.audio_type, c.freq, c.freq_error, c.error_count, c.spike_count,
			c.signal_db, c.noise_db, c.audio_path, c.audio_size, c.patched_tgids, c.metadata_json
		FROM calls c`

	// Join with systems if filtering by short_name
	if filters.ShortName != nil {
		query += ` JOIN systems s ON s.id = c.system_id`
	}

	query += ` WHERE c.stop_time IS NULL AND c.start_time > NOW() - INTERVAL '30 minutes'`

	args := []interface{}{}
	argNum := 1

	if filters.SystemID != nil {
		query += fmt.Sprintf(" AND c.system_id = $%d", argNum)
		args = append(args, *filters.SystemID)
		argNum++
	}
	if filters.ShortName != nil {
		query += fmt.Sprintf(" AND s.short_name = $%d", argNum)
		args = append(args, *filters.ShortName)
		argNum++
	}
	if filters.TalkgroupID != nil {
		query += fmt.Sprintf(" AND c.talkgroup_id = $%d", argNum)
		args = append(args, *filters.TalkgroupID)
		argNum++
	}
	if filters.Emergency != nil {
		query += fmt.Sprintf(" AND c.emergency = $%d", argNum)
		args = append(args, *filters.Emergency)
		argNum++
	}
	if filters.Encrypted != nil {
		query += fmt.Sprintf(" AND c.encrypted = $%d", argNum)
		args = append(args, *filters.Encrypted)
		argNum++
	}

	query += fmt.Sprintf(" ORDER BY c.start_time DESC LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []*models.Call
	for rows.Next() {
		var c models.Call
		if err := rows.Scan(
			&c.ID, &c.CallGroupID, &c.InstanceID, &c.SystemID, &c.TalkgroupID, &c.RecorderID,
			&c.TRCallID, &c.CallNum, &c.StartTime, &c.StopTime, &c.Duration,
			&c.CallState, &c.MonState, &c.Encrypted, &c.Emergency, &c.Phase2TDMA, &c.TDMASlot,
			&c.Conventional, &c.Analog, &c.AudioType, &c.Freq, &c.FreqError, &c.ErrorCount, &c.SpikeCount,
			&c.SignalDB, &c.NoiseDB, &c.AudioPath, &c.AudioSize, &c.PatchedTGIDs, &c.MetadataJSON,
		); err != nil {
			return nil, err
		}
		calls = append(calls, &c)
	}
	return calls, rows.Err()
}

// ActiveUnitFilters contains filters for active units query
type ActiveUnitFilters struct {
	SystemID    *int
	ShortName   *string
	TalkgroupID *int
	WindowMins  int // How many minutes back to consider "active"
}

// ListActiveUnits returns units that have been active within the specified window
func (db *DB) ListActiveUnits(ctx context.Context, filters ActiveUnitFilters, limit, offset int) ([]*models.Unit, error) {
	windowMins := filters.WindowMins
	if windowMins <= 0 {
		windowMins = 5 // Default 5 minutes
	}

	query := `
		SELECT DISTINCT u.id, u.system_id, u.unit_id, u.alpha_tag, u.alpha_tag_source, u.first_seen, u.last_seen
		FROM units u`

	// Join with systems if filtering by short_name
	if filters.ShortName != nil {
		query += ` JOIN systems s ON s.id = u.system_id`
	}

	// Join with unit_events to filter by recent activity or talkgroup
	if filters.TalkgroupID != nil {
		query += ` JOIN unit_events ue ON ue.unit_id = u.id`
	}

	query += fmt.Sprintf(` WHERE u.last_seen > NOW() - INTERVAL '%d minutes'`, windowMins)

	args := []interface{}{}
	argNum := 1

	if filters.SystemID != nil {
		query += fmt.Sprintf(" AND u.system_id = $%d", argNum)
		args = append(args, *filters.SystemID)
		argNum++
	}
	if filters.ShortName != nil {
		query += fmt.Sprintf(" AND s.short_name = $%d", argNum)
		args = append(args, *filters.ShortName)
		argNum++
	}
	if filters.TalkgroupID != nil {
		query += fmt.Sprintf(" AND ue.tgid = $%d AND ue.time > NOW() - INTERVAL '%d minutes'", argNum, windowMins)
		args = append(args, *filters.TalkgroupID)
		argNum++
	}

	query += fmt.Sprintf(" ORDER BY u.last_seen DESC LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []*models.Unit
	var unitIDs []int
	unitMap := make(map[int]*models.Unit)
	for rows.Next() {
		var u models.Unit
		if err := rows.Scan(&u.ID, &u.SystemID, &u.UnitID, &u.AlphaTag, &u.AlphaTagSource, &u.FirstSeen, &u.LastSeen); err != nil {
			return nil, err
		}
		units = append(units, &u)
		unitIDs = append(unitIDs, u.ID)
		unitMap[u.ID] = &u
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch most recent event per unit
	if len(unitIDs) > 0 {
		eventQuery := `
			SELECT DISTINCT ON (ue.unit_id)
				ue.unit_id, ue.event_type, ue.tgid, ue.time,
				COALESCE(tg.alpha_tag, '') as tg_alpha_tag
			FROM unit_events ue
			LEFT JOIN talkgroups tg ON tg.tgid = ue.tgid AND tg.system_id = ue.system_id
			WHERE ue.unit_id = ANY($1)
			ORDER BY ue.unit_id, ue.time DESC
		`
		eventRows, err := db.pool.Query(ctx, eventQuery, unitIDs)
		if err == nil {
			defer eventRows.Close()
			for eventRows.Next() {
				var unitID int
				var eventType string
				var tgid *int64
				var eventTime time.Time
				var tgTag string
				if err := eventRows.Scan(&unitID, &eventType, &tgid, &eventTime, &tgTag); err != nil {
					continue
				}
				if u, ok := unitMap[unitID]; ok {
					u.LastEventType = &eventType
					u.LastEventTGID = tgid
					u.LastEventTime = &eventTime
					if tgTag != "" {
						u.LastEventTGTag = &tgTag
					}
				}
			}
		}
	}

	return units, nil
}

// RecentCallInfo contains call info with system/talkgroup details for display
type RecentCallInfo struct {
	ID         int64            `json:"id"`
	CallID     string           `json:"call_id,omitempty"`
	TRCallID   string           `json:"tr_call_id,omitempty"`
	CallNum    int64            `json:"call_num"`
	StartTime  time.Time        `json:"start_time"`
	StopTime   time.Time        `json:"stop_time"`
	Duration   float32          `json:"duration"`
	System     string           `json:"system"`
	TGID       int              `json:"tgid"`
	TGAlphaTag string           `json:"tg_alpha_tag"`
	Freq       int64            `json:"freq"`
	Encrypted  bool             `json:"encrypted"`
	Emergency  bool             `json:"emergency"`
	AudioPath  string           `json:"audio_path,omitempty"`
	HasAudio   bool             `json:"has_audio"`
	Units      []RecentCallUnit `json:"units"`
}

// RecentCallUnit contains unit info for a call
type RecentCallUnit struct {
	UnitID  int64  `json:"unit_id"`
	UnitTag string `json:"unit_tag"`
}

// ListRecentCalls returns recently completed calls with system/talkgroup info and units
func (db *DB) ListRecentCalls(ctx context.Context, limit int) ([]*RecentCallInfo, error) {
	// Get recent completed calls with system and talkgroup info
	query := `
		SELECT
			c.tr_call_id,
			c.call_num,
			c.start_time,
			c.stop_time,
			c.duration,
			COALESCE(s.short_name, '') as system,
			COALESCE(t.tgid, 0) as tgid,
			COALESCE(t.alpha_tag, '') as tg_alpha_tag,
			c.freq,
			c.encrypted,
			c.emergency,
			COALESCE(c.audio_path, '') as audio_path,
			c.id
		FROM calls c
		LEFT JOIN systems s ON s.id = c.system_id
		LEFT JOIN talkgroups t ON t.id = c.talkgroup_id
		WHERE c.audio_path IS NOT NULL AND c.audio_path != ''
		ORDER BY c.stop_time DESC
		LIMIT $1
	`

	rows, err := db.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []*RecentCallInfo
	var callIDs []int64
	callMap := make(map[int64]*RecentCallInfo)

	for rows.Next() {
		var c RecentCallInfo
		var stopTime *time.Time
		if err := rows.Scan(
			&c.TRCallID, &c.CallNum, &c.StartTime, &stopTime, &c.Duration,
			&c.System, &c.TGID, &c.TGAlphaTag, &c.Freq,
			&c.Encrypted, &c.Emergency, &c.AudioPath, &c.ID,
		); err != nil {
			return nil, err
		}
		if stopTime != nil {
			c.StopTime = *stopTime
		}
		c.CallID = c.TRCallID // backwards compat
		c.HasAudio = c.AudioPath != ""
		c.Units = []RecentCallUnit{}
		calls = append(calls, &c)
		callIDs = append(callIDs, c.ID)
		callMap[c.ID] = &c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Now get units for all these calls from transmissions table
	if len(callIDs) > 0 {
		unitQuery := `
			SELECT DISTINCT
				t.call_id,
				t.unit_rid,
				COALESCE(u.alpha_tag, '') as unit_tag
			FROM transmissions t
			LEFT JOIN units u ON u.id = t.unit_id
			WHERE t.call_id = ANY($1)
			ORDER BY t.call_id, t.unit_rid
		`
		unitRows, err := db.pool.Query(ctx, unitQuery, callIDs)
		if err != nil {
			// Just return calls without units if query fails
			return calls, nil
		}
		defer unitRows.Close()

		for unitRows.Next() {
			var callID int64
			var unitRID int64
			var unitTag string
			if err := unitRows.Scan(&callID, &unitRID, &unitTag); err != nil {
				continue
			}
			if call, ok := callMap[callID]; ok {
				call.Units = append(call.Units, RecentCallUnit{
					UnitID:  unitRID,
					UnitTag: unitTag,
				})
			}
		}
	}

	return calls, nil
}

// GetCallUnits returns all units that transmitted during a call
func (db *DB) GetCallUnits(ctx context.Context, callID int64) ([]RecentCallUnit, error) {
	query := `
		SELECT DISTINCT
			t.unit_rid,
			COALESCE(u.alpha_tag, '') as unit_tag
		FROM transmissions t
		LEFT JOIN units u ON u.id = t.unit_id
		WHERE t.call_id = $1
		ORDER BY t.unit_rid
	`

	rows, err := db.pool.Query(ctx, query, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []RecentCallUnit
	for rows.Next() {
		var u RecentCallUnit
		if err := rows.Scan(&u.UnitID, &u.UnitTag); err != nil {
			return nil, err
		}
		units = append(units, u)
	}
	return units, rows.Err()
}
