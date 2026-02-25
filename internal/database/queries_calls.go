package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CallFilter specifies filters for listing calls.
type CallFilter struct {
	SystemIDs   []int
	SiteIDs     []int
	Sysids      []string
	Tgids       []int
	UnitIDs     []int
	Emergency   *bool
	Encrypted   *bool
	Deduplicate bool
	StartTime   *time.Time
	EndTime     *time.Time
	Limit       int
	Offset      int
	Sort        string
}

// CallAPI represents a call for API responses.
type CallAPI struct {
	CallID        int64     `json:"call_id"`
	CallGroupID   *int      `json:"call_group_id,omitempty"`
	SystemID      int       `json:"system_id"`
	SystemName    string    `json:"system_name,omitempty"`
	Sysid         string    `json:"sysid,omitempty"`
	SiteID        *int      `json:"site_id,omitempty"`
	SiteShortName string    `json:"site_short_name,omitempty"`
	Tgid          int       `json:"tgid"`
	TgAlphaTag    string    `json:"tg_alpha_tag,omitempty"`
	TgDescription string    `json:"tg_description,omitempty"`
	TgTag         string    `json:"tg_tag,omitempty"`
	TgGroup       string    `json:"tg_group,omitempty"`
	StartTime     time.Time `json:"start_time"`
	StopTime      *time.Time `json:"stop_time,omitempty"`
	Duration      *float32  `json:"duration,omitempty"`
	AudioURL      *string   `json:"audio_url,omitempty"`
	AudioType     string    `json:"audio_type,omitempty"`
	AudioSize     *int      `json:"audio_size,omitempty"`
	Freq          *int64    `json:"freq,omitempty"`
	FreqError     *int      `json:"freq_error,omitempty"`
	SignalDB      *float32  `json:"signal_db,omitempty"`
	NoiseDB       *float32  `json:"noise_db,omitempty"`
	ErrorCount    *int      `json:"error_count,omitempty"`
	SpikeCount    *int      `json:"spike_count,omitempty"`
	CallState     string    `json:"call_state,omitempty"`
	MonState      string    `json:"mon_state,omitempty"`
	Emergency     bool      `json:"emergency"`
	Encrypted     bool      `json:"encrypted"`
	Analog        bool      `json:"analog"`
	Conventional  bool      `json:"conventional"`
	Phase2TDMA    bool              `json:"phase2_tdma"`
	TDMASlot      *int16            `json:"tdma_slot,omitempty"`
	PatchedTgids  []int32           `json:"patched_tgids,omitempty"`
	SrcList       json.RawMessage   `json:"src_list,omitempty"`
	FreqList      json.RawMessage   `json:"freq_list,omitempty"`
	UnitIDs              []int32         `json:"unit_ids,omitempty"`
	HasTranscription     bool            `json:"has_transcription"`
	TranscriptionStatus  string          `json:"transcription_status,omitempty"`
	TranscriptionText    *string         `json:"transcription_text,omitempty"`
	TranscriptionWordCt  *int            `json:"transcription_word_count,omitempty"`
	MetadataJSON         json.RawMessage `json:"metadata_json,omitempty"`
	IncidentData         json.RawMessage `json:"incident_data,omitempty"`
	CallFilename         string          `json:"-"` // TR's original path, not exposed in JSON; used for audio resolution
}

// ListCalls returns calls matching the filter with a total count.
func (db *DB) ListCalls(ctx context.Context, filter CallFilter) ([]CallAPI, int, error) {
	// Always include the LEFT JOIN; the dedup condition skips it when not active.
	const fromClause = `FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		LEFT JOIN call_groups cg ON cg.id = c.call_group_id`
	const whereClause = `
		WHERE ($1::timestamptz IS NULL OR c.start_time >= $1)
		  AND ($2::timestamptz IS NULL OR c.start_time < $2)
		  AND ($3::int[] IS NULL OR c.system_id = ANY($3))
		  AND ($4::int[] IS NULL OR c.site_id = ANY($4))
		  AND ($5::text[] IS NULL OR s.sysid = ANY($5))
		  AND ($6::int[] IS NULL OR c.tgid = ANY($6))
		  AND ($7::int[] IS NULL OR c.unit_ids && $7)
		  AND ($8::boolean IS NULL OR c.emergency = $8)
		  AND ($9::boolean IS NULL OR c.encrypted = $9)
		  AND ($10::boolean IS NOT TRUE OR c.call_group_id IS NULL OR c.call_id = cg.primary_call_id OR cg.primary_call_id IS NULL)`
	args := []any{
		filter.StartTime, filter.EndTime,
		pqIntArray(filter.SystemIDs), pqIntArray(filter.SiteIDs),
		pqStringArray(filter.Sysids), pqIntArray(filter.Tgids),
		pqIntArray(filter.UnitIDs), filter.Emergency, filter.Encrypted,
		filter.Deduplicate,
	}

	// Count query
	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort
	orderBy := "c.start_time DESC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	// Data query
	dataQuery := fmt.Sprintf(`
		SELECT c.call_id, c.call_group_id, c.system_id, COALESCE(c.system_name, ''), COALESCE(s.sysid, ''),
			c.site_id, COALESCE(c.site_short_name, ''),
			c.tgid, COALESCE(c.tg_alpha_tag, ''), COALESCE(c.tg_description, ''),
			COALESCE(c.tg_tag, ''), COALESCE(c.tg_group, ''),
			c.start_time, c.stop_time, c.duration,
			c.audio_file_path, COALESCE(c.audio_type, ''), c.audio_file_size,
			COALESCE(c.call_filename, ''),
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids,
			c.src_list, c.freq_list, c.unit_ids,
			COALESCE(c.has_transcription, false), COALESCE(c.transcription_status, 'none'),
			c.transcription_text, c.transcription_word_count,
			c.metadata_json, c.incidentdata
		%s %s
		ORDER BY %s
		LIMIT $11 OFFSET $12
	`, fromClause, whereClause, orderBy)

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var calls []CallAPI
	for rows.Next() {
		var c CallAPI
		var audioPath *string
		if err := rows.Scan(
			&c.CallID, &c.CallGroupID, &c.SystemID, &c.SystemName, &c.Sysid,
			&c.SiteID, &c.SiteShortName,
			&c.Tgid, &c.TgAlphaTag, &c.TgDescription, &c.TgTag, &c.TgGroup,
			&c.StartTime, &c.StopTime, &c.Duration,
			&audioPath, &c.AudioType, &c.AudioSize,
			&c.CallFilename,
			&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
			&c.CallState, &c.MonState,
			&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
			&c.Phase2TDMA, &c.TDMASlot,
			&c.PatchedTgids,
			&c.SrcList, &c.FreqList, &c.UnitIDs,
			&c.HasTranscription, &c.TranscriptionStatus,
			&c.TranscriptionText, &c.TranscriptionWordCt,
			&c.MetadataJSON, &c.IncidentData,
		); err != nil {
			return nil, 0, err
		}
		if audioPath != nil && *audioPath != "" {
			url := fmt.Sprintf("/api/v1/calls/%d/audio", c.CallID)
			c.AudioURL = &url
		}
		calls = append(calls, c)
	}
	if calls == nil {
		calls = []CallAPI{}
	}
	return calls, total, rows.Err()
}

// GetCallByID returns a single call.
func (db *DB) GetCallByID(ctx context.Context, callID int64) (*CallAPI, error) {
	var c CallAPI
	var audioPath *string
	err := db.Pool.QueryRow(ctx, `
		SELECT c.call_id, c.call_group_id, c.system_id, COALESCE(c.system_name, ''), COALESCE(s.sysid, ''),
			c.site_id, COALESCE(c.site_short_name, ''),
			c.tgid, COALESCE(c.tg_alpha_tag, ''), COALESCE(c.tg_description, ''),
			COALESCE(c.tg_tag, ''), COALESCE(c.tg_group, ''),
			c.start_time, c.stop_time, c.duration,
			c.audio_file_path, COALESCE(c.audio_type, ''), c.audio_file_size,
			COALESCE(c.call_filename, ''),
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids,
			c.src_list, c.freq_list, c.unit_ids,
			COALESCE(c.has_transcription, false), COALESCE(c.transcription_status, 'none'),
			c.transcription_text, c.transcription_word_count,
			c.metadata_json, c.incidentdata
		FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		WHERE c.call_id = $1
	`, callID).Scan(
		&c.CallID, &c.CallGroupID, &c.SystemID, &c.SystemName, &c.Sysid,
		&c.SiteID, &c.SiteShortName,
		&c.Tgid, &c.TgAlphaTag, &c.TgDescription, &c.TgTag, &c.TgGroup,
		&c.StartTime, &c.StopTime, &c.Duration,
		&audioPath, &c.AudioType, &c.AudioSize,
		&c.CallFilename,
		&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
		&c.CallState, &c.MonState,
		&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
		&c.Phase2TDMA, &c.TDMASlot,
		&c.PatchedTgids,
		&c.SrcList, &c.FreqList, &c.UnitIDs,
		&c.HasTranscription, &c.TranscriptionStatus,
		&c.TranscriptionText, &c.TranscriptionWordCt,
		&c.MetadataJSON, &c.IncidentData,
	)
	if err != nil {
		return nil, err
	}
	if audioPath != nil && *audioPath != "" {
		url := fmt.Sprintf("/api/v1/calls/%d/audio", c.CallID)
		c.AudioURL = &url
	}
	return &c, nil
}

// CallFrequencyAPI represents a frequency entry for API responses.
type CallFrequencyAPI struct {
	Freq       int64    `json:"freq"`
	Time       *int64   `json:"time,omitempty"`
	Pos        *float32 `json:"pos,omitempty"`
	Len        *float32 `json:"len,omitempty"`
	ErrorCount *int     `json:"error_count,omitempty"`
	SpikeCount *int     `json:"spike_count,omitempty"`
}

// CallTransmissionAPI represents a transmission entry for API responses.
type CallTransmissionAPI struct {
	Src          int      `json:"src"`
	Tag          string   `json:"tag,omitempty"`
	Time         *int64   `json:"time,omitempty"`
	Pos          *float32 `json:"pos,omitempty"`
	Duration     *float32 `json:"duration,omitempty"`
	Emergency    int16    `json:"emergency"`
	SignalSystem string   `json:"signal_system,omitempty"`
}

// CallGroupFilter specifies filters for listing call groups.
type CallGroupFilter struct {
	Sysids    []string
	Tgids     []int
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// CallGroupAPI represents a call group for API responses.
type CallGroupAPI struct {
	ID                  int        `json:"id"`
	SystemID            int        `json:"system_id"`
	SystemName          string     `json:"system_name,omitempty"`
	Sysid               string     `json:"sysid,omitempty"`
	SiteID              *int       `json:"site_id,omitempty"`
	SiteShortName       string     `json:"site_short_name,omitempty"`
	Tgid                int        `json:"tgid"`
	TgAlphaTag          string     `json:"tg_alpha_tag,omitempty"`
	TgDescription       string     `json:"tg_description,omitempty"`
	TgTag               string     `json:"tg_tag,omitempty"`
	TgGroup             string     `json:"tg_group,omitempty"`
	StartTime           time.Time  `json:"start_time"`
	StopTime            *time.Time `json:"stop_time,omitempty"`
	Duration            *float32   `json:"duration,omitempty"`
	CallCount           int        `json:"call_count"`
	PrimaryCallID       *int64     `json:"primary_call_id,omitempty"`
	HasTranscription    bool       `json:"has_transcription"`
	TranscriptionStatus string     `json:"transcription_status,omitempty"`
	TranscriptionText   *string    `json:"transcription_text,omitempty"`
}

// ListCallGroups returns call groups matching the filter.
func (db *DB) ListCallGroups(ctx context.Context, filter CallGroupFilter) ([]CallGroupAPI, int, error) {
	const fromClause = `FROM call_groups cg
		JOIN systems s ON s.system_id = cg.system_id
		LEFT JOIN calls pc ON pc.call_id = cg.primary_call_id AND pc.start_time >= cg.start_time - interval '10 seconds'`
	const whereClause = `
		WHERE ($1::timestamptz IS NULL OR cg.start_time >= $1)
		  AND ($2::timestamptz IS NULL OR cg.start_time < $2)
		  AND ($3::text[] IS NULL OR s.sysid = ANY($3))
		  AND ($4::int[] IS NULL OR cg.tgid = ANY($4))`
	args := []any{filter.StartTime, filter.EndTime, pqStringArray(filter.Sysids), pqIntArray(filter.Tgids)}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := `
		SELECT cg.id, cg.system_id, COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			pc.site_id, COALESCE(pc.site_short_name, ''),
			cg.tgid, COALESCE(cg.tg_alpha_tag, ''), COALESCE(cg.tg_description, ''),
			COALESCE(cg.tg_tag, ''), COALESCE(cg.tg_group, ''),
			cg.start_time, cg.primary_call_id,
			(SELECT count(*) FROM calls c WHERE c.call_group_id = cg.id),
			COALESCE(cg.transcription_text IS NOT NULL, false),
			COALESCE(cg.transcription_status, 'none'),
			cg.transcription_text
		` + fromClause + whereClause + `
		ORDER BY cg.start_time DESC
		LIMIT $5 OFFSET $6`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var groups []CallGroupAPI
	for rows.Next() {
		var g CallGroupAPI
		if err := rows.Scan(
			&g.ID, &g.SystemID, &g.SystemName, &g.Sysid,
			&g.SiteID, &g.SiteShortName,
			&g.Tgid, &g.TgAlphaTag, &g.TgDescription, &g.TgTag, &g.TgGroup,
			&g.StartTime, &g.PrimaryCallID, &g.CallCount,
			&g.HasTranscription, &g.TranscriptionStatus, &g.TranscriptionText,
		); err != nil {
			return nil, 0, err
		}
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []CallGroupAPI{}
	}
	return groups, total, rows.Err()
}

// GetCallGroupByID returns a call group with its individual recordings.
func (db *DB) GetCallGroupByID(ctx context.Context, id int) (*CallGroupAPI, []CallAPI, error) {
	var g CallGroupAPI
	err := db.Pool.QueryRow(ctx, `
		SELECT cg.id, cg.system_id, COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			pc.site_id, COALESCE(pc.site_short_name, ''),
			cg.tgid, COALESCE(cg.tg_alpha_tag, ''), COALESCE(cg.tg_description, ''),
			COALESCE(cg.tg_tag, ''), COALESCE(cg.tg_group, ''),
			cg.start_time, cg.primary_call_id,
			(SELECT count(*) FROM calls c WHERE c.call_group_id = cg.id),
			COALESCE(cg.transcription_text IS NOT NULL, false),
			COALESCE(cg.transcription_status, 'none'),
			cg.transcription_text
		FROM call_groups cg
		JOIN systems s ON s.system_id = cg.system_id
		LEFT JOIN calls pc ON pc.call_id = cg.primary_call_id AND pc.start_time >= cg.start_time - interval '10 seconds'
		WHERE cg.id = $1
	`, id).Scan(
		&g.ID, &g.SystemID, &g.SystemName, &g.Sysid,
		&g.SiteID, &g.SiteShortName,
		&g.Tgid, &g.TgAlphaTag, &g.TgDescription, &g.TgTag, &g.TgGroup,
		&g.StartTime, &g.PrimaryCallID, &g.CallCount,
		&g.HasTranscription, &g.TranscriptionStatus, &g.TranscriptionText,
	)
	if err != nil {
		return nil, nil, err
	}

	// Fetch calls in this group
	rows, err := db.Pool.Query(ctx, `
		SELECT c.call_id, c.call_group_id, c.system_id, COALESCE(c.system_name, ''), COALESCE(s.sysid, ''),
			c.site_id, COALESCE(c.site_short_name, ''),
			c.tgid, COALESCE(c.tg_alpha_tag, ''), COALESCE(c.tg_description, ''),
			COALESCE(c.tg_tag, ''), COALESCE(c.tg_group, ''),
			c.start_time, c.stop_time, c.duration,
			c.audio_file_path, COALESCE(c.audio_type, ''), c.audio_file_size,
			COALESCE(c.call_filename, ''),
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids,
			c.src_list, c.freq_list, c.unit_ids,
			COALESCE(c.has_transcription, false), COALESCE(c.transcription_status, 'none'),
			c.transcription_text, c.transcription_word_count,
			c.metadata_json, c.incidentdata
		FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		WHERE c.call_group_id = $1
		ORDER BY c.start_time DESC
	`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var calls []CallAPI
	for rows.Next() {
		var c CallAPI
		var audioPath *string
		if err := rows.Scan(
			&c.CallID, &c.CallGroupID, &c.SystemID, &c.SystemName, &c.Sysid,
			&c.SiteID, &c.SiteShortName,
			&c.Tgid, &c.TgAlphaTag, &c.TgDescription, &c.TgTag, &c.TgGroup,
			&c.StartTime, &c.StopTime, &c.Duration,
			&audioPath, &c.AudioType, &c.AudioSize,
			&c.CallFilename,
			&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
			&c.CallState, &c.MonState,
			&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
			&c.Phase2TDMA, &c.TDMASlot,
			&c.PatchedTgids,
			&c.SrcList, &c.FreqList, &c.UnitIDs,
			&c.HasTranscription, &c.TranscriptionStatus,
			&c.TranscriptionText, &c.TranscriptionWordCt,
			&c.MetadataJSON, &c.IncidentData,
		); err != nil {
			return nil, nil, err
		}
		if audioPath != nil && *audioPath != "" {
			url := fmt.Sprintf("/api/v1/calls/%d/audio", c.CallID)
			c.AudioURL = &url
		}
		calls = append(calls, c)
	}
	if calls == nil {
		calls = []CallAPI{}
	}
	return &g, calls, rows.Err()
}
