package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// queryBuilder builds parameterized WHERE clauses for dynamic queries.
type queryBuilder struct {
	where  []string
	args   []any
	argIdx int
}

func newQueryBuilder() *queryBuilder {
	return &queryBuilder{argIdx: 1}
}

// Add appends a WHERE condition. The clause should contain %s which will be replaced with $N.
func (qb *queryBuilder) Add(clause string, val any) {
	parameterized := strings.Replace(clause, "%s", fmt.Sprintf("$%d", qb.argIdx), 1)
	qb.where = append(qb.where, parameterized)
	qb.args = append(qb.args, val)
	qb.argIdx++
}

// AddRaw appends a WHERE condition with no parameters.
func (qb *queryBuilder) AddRaw(clause string) {
	qb.where = append(qb.where, clause)
}

// WhereClause returns the full WHERE clause (including "WHERE") or empty string if no conditions.
func (qb *queryBuilder) WhereClause() string {
	if len(qb.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(qb.where, " AND ")
}

// Args returns all accumulated arguments.
func (qb *queryBuilder) Args() []any {
	return qb.args
}

// CallFilter specifies filters for listing calls.
type CallFilter struct {
	SystemID    *int
	SiteID      *int
	Sysid       *string
	Tgid        *int
	UnitID      *int
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
	Phase2TDMA    bool      `json:"phase2_tdma"`
	TDMASlot      *int16    `json:"tdma_slot,omitempty"`
	PatchedTgids  []int32   `json:"patched_tgids,omitempty"`
}

// ListCalls returns calls matching the filter with a total count.
func (db *DB) ListCalls(ctx context.Context, filter CallFilter) ([]CallAPI, int, error) {
	qb := newQueryBuilder()

	// Default time bounds for partition pruning
	if filter.StartTime != nil {
		qb.Add("c.start_time >= %s", *filter.StartTime)
	} else {
		qb.Add("c.start_time >= %s", time.Now().Add(-24*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("c.start_time < %s", *filter.EndTime)
	}

	if filter.SystemID != nil {
		qb.Add("c.system_id = %s", *filter.SystemID)
	}
	if filter.SiteID != nil {
		qb.Add("c.site_id = %s", *filter.SiteID)
	}
	if filter.Sysid != nil {
		qb.Add("s.sysid = %s", *filter.Sysid)
	}
	if filter.Tgid != nil {
		qb.Add("c.tgid = %s", *filter.Tgid)
	}
	if filter.UnitID != nil {
		qb.Add(fmt.Sprintf("EXISTS (SELECT 1 FROM call_transmissions ct WHERE ct.call_id = c.call_id AND ct.call_start_time = c.start_time AND ct.src = %%s)"), *filter.UnitID)
	}
	if filter.Emergency != nil {
		qb.Add("c.emergency = %s", *filter.Emergency)
	}
	if filter.Encrypted != nil {
		qb.Add("c.encrypted = %s", *filter.Encrypted)
	}

	fromClause := "FROM calls c JOIN systems s ON s.system_id = c.system_id"
	if filter.Deduplicate {
		fromClause += " LEFT JOIN call_groups cg ON cg.id = c.call_group_id"
		qb.AddRaw("(c.call_group_id IS NULL OR c.call_id = cg.primary_call_id OR cg.primary_call_id IS NULL)")
	}

	whereClause := qb.WhereClause()

	// Count query
	countQuery := "SELECT count(*) " + fromClause + whereClause
	var total int
	if err := db.Pool.QueryRow(ctx, countQuery, qb.Args()...).Scan(&total); err != nil {
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
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids
		%s %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, orderBy, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
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
			&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
			&c.CallState, &c.MonState,
			&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
			&c.Phase2TDMA, &c.TDMASlot,
			&c.PatchedTgids,
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
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids
		FROM calls c
		JOIN systems s ON s.system_id = c.system_id
		WHERE c.call_id = $1
		ORDER BY c.start_time DESC
		LIMIT 1
	`, callID).Scan(
		&c.CallID, &c.CallGroupID, &c.SystemID, &c.SystemName, &c.Sysid,
		&c.SiteID, &c.SiteShortName,
		&c.Tgid, &c.TgAlphaTag, &c.TgDescription, &c.TgTag, &c.TgGroup,
		&c.StartTime, &c.StopTime, &c.Duration,
		&audioPath, &c.AudioType, &c.AudioSize,
		&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
		&c.CallState, &c.MonState,
		&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
		&c.Phase2TDMA, &c.TDMASlot,
		&c.PatchedTgids,
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

// GetCallAudioPath returns the audio file path for a call.
func (db *DB) GetCallAudioPath(ctx context.Context, callID int64) (string, error) {
	var path *string
	err := db.Pool.QueryRow(ctx, `
		SELECT audio_file_path FROM calls WHERE call_id = $1
		ORDER BY start_time DESC LIMIT 1
	`, callID).Scan(&path)
	if err != nil {
		return "", err
	}
	if path == nil {
		return "", nil
	}
	return *path, nil
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

// GetCallFrequencies returns frequency entries for a call.
func (db *DB) GetCallFrequencies(ctx context.Context, callID int64) ([]CallFrequencyAPI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cf.freq, extract(epoch from cf.time)::bigint, cf.pos, cf.len,
			cf.error_count, cf.spike_count
		FROM call_frequencies cf
		WHERE cf.call_id = $1
		ORDER BY cf.pos
	`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var freqs []CallFrequencyAPI
	for rows.Next() {
		var f CallFrequencyAPI
		if err := rows.Scan(&f.Freq, &f.Time, &f.Pos, &f.Len, &f.ErrorCount, &f.SpikeCount); err != nil {
			return nil, err
		}
		freqs = append(freqs, f)
	}
	if freqs == nil {
		freqs = []CallFrequencyAPI{}
	}
	return freqs, rows.Err()
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

// GetCallTransmissions returns transmission entries for a call.
func (db *DB) GetCallTransmissions(ctx context.Context, callID int64) ([]CallTransmissionAPI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT ct.src, COALESCE(ct.tag, ''), extract(epoch from ct.time)::bigint,
			ct.pos, ct.duration, COALESCE(ct.emergency, 0), COALESCE(ct.signal_system, '')
		FROM call_transmissions ct
		WHERE ct.call_id = $1
		ORDER BY ct.pos
	`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []CallTransmissionAPI
	for rows.Next() {
		var t CallTransmissionAPI
		if err := rows.Scan(&t.Src, &t.Tag, &t.Time, &t.Pos, &t.Duration, &t.Emergency, &t.SignalSystem); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	if txs == nil {
		txs = []CallTransmissionAPI{}
	}
	return txs, rows.Err()
}

// CallGroupFilter specifies filters for listing call groups.
type CallGroupFilter struct {
	Sysid     *string
	Tgid      *int
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// CallGroupAPI represents a call group for API responses.
type CallGroupAPI struct {
	ID            int        `json:"id"`
	SystemID      int        `json:"system_id"`
	SystemName    string     `json:"system_name,omitempty"`
	Sysid         string     `json:"sysid,omitempty"`
	Tgid          int        `json:"tgid"`
	TgAlphaTag    string     `json:"tg_alpha_tag,omitempty"`
	TgDescription string     `json:"tg_description,omitempty"`
	TgTag         string     `json:"tg_tag,omitempty"`
	TgGroup       string     `json:"tg_group,omitempty"`
	StartTime     time.Time  `json:"start_time"`
	StopTime      *time.Time `json:"stop_time,omitempty"`
	Duration      *float32   `json:"duration,omitempty"`
	CallCount     int        `json:"call_count"`
	PrimaryCallID *int64     `json:"primary_call_id,omitempty"`
}

// ListCallGroups returns call groups matching the filter.
func (db *DB) ListCallGroups(ctx context.Context, filter CallGroupFilter) ([]CallGroupAPI, int, error) {
	qb := newQueryBuilder()

	if filter.StartTime != nil {
		qb.Add("cg.start_time >= %s", *filter.StartTime)
	} else {
		qb.Add("cg.start_time >= %s", time.Now().Add(-24*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("cg.start_time < %s", *filter.EndTime)
	}
	if filter.Sysid != nil {
		qb.Add("s.sysid = %s", *filter.Sysid)
	}
	if filter.Tgid != nil {
		qb.Add("cg.tgid = %s", *filter.Tgid)
	}

	fromClause := "FROM call_groups cg JOIN systems s ON s.system_id = cg.system_id"
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf(`
		SELECT cg.id, cg.system_id, COALESCE(s.name, ''), COALESCE(s.sysid, ''),
			cg.tgid, COALESCE(cg.tg_alpha_tag, ''), COALESCE(cg.tg_description, ''),
			COALESCE(cg.tg_tag, ''), COALESCE(cg.tg_group, ''),
			cg.start_time, cg.primary_call_id,
			(SELECT count(*) FROM calls c WHERE c.call_group_id = cg.id)
		%s %s
		ORDER BY cg.start_time DESC
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var groups []CallGroupAPI
	for rows.Next() {
		var g CallGroupAPI
		if err := rows.Scan(
			&g.ID, &g.SystemID, &g.SystemName, &g.Sysid,
			&g.Tgid, &g.TgAlphaTag, &g.TgDescription, &g.TgTag, &g.TgGroup,
			&g.StartTime, &g.PrimaryCallID, &g.CallCount,
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
			cg.tgid, COALESCE(cg.tg_alpha_tag, ''), COALESCE(cg.tg_description, ''),
			COALESCE(cg.tg_tag, ''), COALESCE(cg.tg_group, ''),
			cg.start_time, cg.primary_call_id,
			(SELECT count(*) FROM calls c WHERE c.call_group_id = cg.id)
		FROM call_groups cg
		JOIN systems s ON s.system_id = cg.system_id
		WHERE cg.id = $1
	`, id).Scan(
		&g.ID, &g.SystemID, &g.SystemName, &g.Sysid,
		&g.Tgid, &g.TgAlphaTag, &g.TgDescription, &g.TgTag, &g.TgGroup,
		&g.StartTime, &g.PrimaryCallID, &g.CallCount,
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
			c.freq, c.freq_error, c.signal_db, c.noise_db, c.error_count, c.spike_count,
			COALESCE(c.call_state_type, ''), COALESCE(c.mon_state_type, ''),
			COALESCE(c.emergency, false), COALESCE(c.encrypted, false),
			COALESCE(c.analog, false), COALESCE(c.conventional, false),
			COALESCE(c.phase2_tdma, false), c.tdma_slot,
			c.patched_tgids
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
			&c.Freq, &c.FreqError, &c.SignalDB, &c.NoiseDB, &c.ErrorCount, &c.SpikeCount,
			&c.CallState, &c.MonState,
			&c.Emergency, &c.Encrypted, &c.Analog, &c.Conventional,
			&c.Phase2TDMA, &c.TDMASlot,
			&c.PatchedTgids,
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
