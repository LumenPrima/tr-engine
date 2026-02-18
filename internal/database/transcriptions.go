package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TranscriptionRow is the input for inserting a transcription.
type TranscriptionRow struct {
	CallID        int64
	CallStartTime time.Time
	Text          string
	Source        string  // "auto", "human", "llm"
	IsPrimary     bool
	Confidence    *float32
	Language      string
	Model         string
	Provider      string
	WordCount     int
	DurationMs    int
	Words         json.RawMessage // word-level timestamps with unit attribution
}

// TranscriptionAPI is the transcription representation for API responses.
type TranscriptionAPI struct {
	ID         int              `json:"id"`
	CallID     int64            `json:"call_id"`
	Text       string           `json:"text"`
	Source     string           `json:"source"`
	IsPrimary  bool             `json:"is_primary"`
	Confidence *float32         `json:"confidence,omitempty"`
	Language   string           `json:"language,omitempty"`
	Model      string           `json:"model,omitempty"`
	Provider   string           `json:"provider,omitempty"`
	WordCount  int              `json:"word_count"`
	DurationMs int              `json:"duration_ms"`
	Words      json.RawMessage  `json:"words,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
}

// CallTranscriptionInfo is a lightweight view of a call for the transcription worker.
type CallTranscriptionInfo struct {
	CallID           int64
	StartTime        time.Time
	SystemID         int
	Tgid             int
	Duration         *float32
	AudioFilePath    string
	CallFilename     string
	SrcList          json.RawMessage
	Encrypted        bool
	HasTranscription bool
	TgAlphaTag       string
	TgDescription    string
	TgTag            string
	TgGroup          string
}

// TranscriptionSearchFilter specifies filters for full-text search.
type TranscriptionSearchFilter struct {
	SystemIDs []int
	Tgids     []int
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// TranscriptionSearchHit is a search result with relevance score and call context.
type TranscriptionSearchHit struct {
	TranscriptionAPI
	Rank          float32   `json:"rank"`
	CallSystemID  int       `json:"system_id"`
	CallSystemName string   `json:"system_name,omitempty"`
	CallTgid      int       `json:"tgid"`
	CallTgAlphaTag string   `json:"tg_alpha_tag,omitempty"`
	CallStartTime time.Time `json:"call_start_time"`
	CallDuration  *float32  `json:"call_duration,omitempty"`
}

// InsertTranscription inserts a new transcription in a transaction:
// 1) Clears is_primary on existing transcriptions for this call
// 2) Inserts the new transcription
// 3) Updates the calls table denormalized fields
// 4) Updates the call_groups table transcription fields
func (db *DB) InsertTranscription(ctx context.Context, row *TranscriptionRow) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Clear is_primary on existing transcriptions for this call
	if row.IsPrimary {
		_, err = tx.Exec(ctx, `
			UPDATE transcriptions SET is_primary = false
			WHERE call_id = $1 AND call_start_time = $2 AND is_primary = true
		`, row.CallID, row.CallStartTime)
		if err != nil {
			return 0, fmt.Errorf("clear is_primary: %w", err)
		}
	}

	// Insert the new transcription
	var id int
	err = tx.QueryRow(ctx, `
		INSERT INTO transcriptions (
			call_id, call_start_time, text, source, is_primary,
			confidence, language, model, provider,
			word_count, duration_ms, words
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`,
		row.CallID, row.CallStartTime, row.Text, row.Source, row.IsPrimary,
		row.Confidence, row.Language, row.Model, row.Provider,
		row.WordCount, row.DurationMs, row.Words,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert transcription: %w", err)
	}

	// Update calls denormalized fields
	if row.IsPrimary {
		status := row.Source
		if status == "human" {
			status = "verified"
		}
		_, err = tx.Exec(ctx, `
			UPDATE calls SET
				has_transcription = true,
				transcription_status = $3,
				transcription_text = $4,
				transcription_word_count = $5
			WHERE call_id = $1 AND start_time = $2
		`, row.CallID, row.CallStartTime, status, row.Text, row.WordCount)
		if err != nil {
			return 0, fmt.Errorf("update calls denorm: %w", err)
		}

		// Update call_groups transcription fields (if call belongs to a group)
		_, err = tx.Exec(ctx, `
			UPDATE call_groups SET
				transcription_text = $3,
				transcription_status = $4
			WHERE id = (
				SELECT call_group_id FROM calls
				WHERE call_id = $1 AND start_time = $2 AND call_group_id IS NOT NULL
			)
		`, row.CallID, row.CallStartTime, row.Text, status)
		if err != nil {
			return 0, fmt.Errorf("update call_groups denorm: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return id, nil
}

// GetPrimaryTranscription returns the primary transcription for a call.
func (db *DB) GetPrimaryTranscription(ctx context.Context, callID int64) (*TranscriptionAPI, error) {
	var t TranscriptionAPI
	err := db.Pool.QueryRow(ctx, `
		SELECT id, call_id, text, source, is_primary,
			confidence, language, model, provider,
			word_count, duration_ms, words, created_at
		FROM transcriptions
		WHERE call_id = $1 AND is_primary = true
		ORDER BY created_at DESC
		LIMIT 1
	`, callID).Scan(
		&t.ID, &t.CallID, &t.Text, &t.Source, &t.IsPrimary,
		&t.Confidence, &t.Language, &t.Model, &t.Provider,
		&t.WordCount, &t.DurationMs, &t.Words, &t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTranscriptionsByCall returns all transcription variants for a call.
func (db *DB) ListTranscriptionsByCall(ctx context.Context, callID int64) ([]TranscriptionAPI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, call_id, text, source, is_primary,
			confidence, language, model, provider,
			word_count, duration_ms, words, created_at
		FROM transcriptions
		WHERE call_id = $1
		ORDER BY created_at DESC
	`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TranscriptionAPI
	for rows.Next() {
		var t TranscriptionAPI
		if err := rows.Scan(
			&t.ID, &t.CallID, &t.Text, &t.Source, &t.IsPrimary,
			&t.Confidence, &t.Language, &t.Model, &t.Provider,
			&t.WordCount, &t.DurationMs, &t.Words, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if result == nil {
		result = []TranscriptionAPI{}
	}
	return result, rows.Err()
}

// SearchTranscriptions performs full-text search across transcriptions with call context.
func (db *DB) SearchTranscriptions(ctx context.Context, query string, filter TranscriptionSearchFilter) ([]TranscriptionSearchHit, int, error) {
	qb := newQueryBuilder()
	qb.Add("t.search_vector @@ plainto_tsquery('english', %s)", query)

	if filter.StartTime != nil {
		qb.Add("t.call_start_time >= %s", *filter.StartTime)
	}
	if filter.EndTime != nil {
		qb.Add("t.call_start_time < %s", *filter.EndTime)
	}
	if len(filter.SystemIDs) > 0 {
		qb.Add("c.system_id = ANY(%s)", filter.SystemIDs)
	}
	if len(filter.Tgids) > 0 {
		qb.Add("c.tgid = ANY(%s)", filter.Tgids)
	}
	qb.AddRaw("t.is_primary = true")

	whereClause := qb.WhereClause()
	fromClause := "FROM transcriptions t JOIN calls c ON c.call_id = t.call_id AND c.start_time = t.call_start_time"

	// Count
	var total int
	countQuery := "SELECT count(*) " + fromClause + whereClause
	if err := db.Pool.QueryRow(ctx, countQuery, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Results with rank
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset

	rankExpr := fmt.Sprintf("ts_rank(t.search_vector, plainto_tsquery('english', $%d))", qb.argIdx)
	qb.args = append(qb.args, query)
	qb.argIdx++

	dataQuery := fmt.Sprintf(`
		SELECT t.id, t.call_id, t.text, t.source, t.is_primary,
			t.confidence, t.language, t.model, t.provider,
			t.word_count, t.duration_ms, t.words, t.created_at,
			%s AS rank,
			c.system_id, COALESCE(c.system_name, ''), c.tgid,
			COALESCE(c.tg_alpha_tag, ''), c.start_time, c.duration
		%s%s
		ORDER BY rank DESC
		LIMIT %d OFFSET %d
	`, rankExpr, fromClause, whereClause, limit, offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var hits []TranscriptionSearchHit
	for rows.Next() {
		var h TranscriptionSearchHit
		if err := rows.Scan(
			&h.ID, &h.CallID, &h.Text, &h.Source, &h.IsPrimary,
			&h.Confidence, &h.Language, &h.Model, &h.Provider,
			&h.WordCount, &h.DurationMs, &h.Words, &h.CreatedAt,
			&h.Rank,
			&h.CallSystemID, &h.CallSystemName, &h.CallTgid,
			&h.CallTgAlphaTag, &h.CallStartTime, &h.CallDuration,
		); err != nil {
			return nil, 0, err
		}
		hits = append(hits, h)
	}
	if hits == nil {
		hits = []TranscriptionSearchHit{}
	}
	return hits, total, rows.Err()
}

// GetCallForTranscription returns a lightweight call view for the transcription worker.
func (db *DB) GetCallForTranscription(ctx context.Context, callID int64) (*CallTranscriptionInfo, error) {
	var c CallTranscriptionInfo
	err := db.Pool.QueryRow(ctx, `
		SELECT call_id, start_time, system_id, tgid, duration,
			COALESCE(audio_file_path, ''), COALESCE(call_filename, ''),
			src_list, encrypted, has_transcription,
			COALESCE(tg_alpha_tag, ''), COALESCE(tg_description, ''),
			COALESCE(tg_tag, ''), COALESCE(tg_group, '')
		FROM calls
		WHERE call_id = $1
		ORDER BY start_time DESC
		LIMIT 1
	`, callID).Scan(
		&c.CallID, &c.StartTime, &c.SystemID, &c.Tgid, &c.Duration,
		&c.AudioFilePath, &c.CallFilename,
		&c.SrcList, &c.Encrypted, &c.HasTranscription,
		&c.TgAlphaTag, &c.TgDescription, &c.TgTag, &c.TgGroup,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCallTranscriptionStatus updates the transcription_status on a call and its group.
func (db *DB) UpdateCallTranscriptionStatus(ctx context.Context, callID int64, startTime time.Time, status string) error {
	valid := map[string]bool{"none": true, "auto": true, "reviewed": true, "verified": true, "excluded": true}
	if !valid[status] {
		return fmt.Errorf("invalid transcription status: %s", status)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE calls SET transcription_status = $3
		WHERE call_id = $1 AND start_time = $2
	`, callID, startTime, status)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE call_groups SET transcription_status = $3
		WHERE id = (
			SELECT call_group_id FROM calls
			WHERE call_id = $1 AND start_time = $2 AND call_group_id IS NOT NULL
		)
	`, callID, startTime, status)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

