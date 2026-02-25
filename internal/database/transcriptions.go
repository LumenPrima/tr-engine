package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

// TranscriptionRow is the input for inserting a transcription.
type TranscriptionRow struct {
	CallID        int64
	CallStartTime time.Time
	Text          string
	Source        string // "auto", "human", "llm"
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
	ID         int             `json:"id"`
	CallID     int64           `json:"call_id"`
	Text       string          `json:"text"`
	Source     string          `json:"source"`
	IsPrimary  bool            `json:"is_primary"`
	Confidence *float32        `json:"confidence,omitempty"`
	Language   string          `json:"language,omitempty"`
	Model      string          `json:"model,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	WordCount  int             `json:"word_count"`
	DurationMs int             `json:"duration_ms"`
	Words      json.RawMessage `json:"words,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
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
	SystemIDs   []int
	SiteIDs     []int
	Tgids       []int
	StartTime   *time.Time
	EndTime     *time.Time
	PrimaryOnly *bool // default true; set to false to include all variants
	Limit     int
	Offset    int
}

// TranscriptionSearchHit is a search result with relevance score and call context.
type TranscriptionSearchHit struct {
	TranscriptionAPI
	Rank           float32   `json:"rank"`
	CallSystemID   int       `json:"system_id"`
	CallSystemName string    `json:"system_name,omitempty"`
	CallTgid       int       `json:"tgid"`
	CallTgAlphaTag string    `json:"tg_alpha_tag,omitempty"`
	CallStartTime  time.Time `json:"call_start_time"`
	CallDuration   *float32  `json:"call_duration,omitempty"`
}

func primaryTranscriptionToAPI(r sqlcdb.GetPrimaryTranscriptionRow) TranscriptionAPI {
	t := TranscriptionAPI{
		ID:         r.ID,
		CallID:     r.CallID,
		Source:     r.Source,
		IsPrimary:  r.IsPrimary,
		Confidence: r.Confidence,
		Words:      r.Words,
	}
	if r.Text != nil {
		t.Text = *r.Text
	}
	if r.Language != nil {
		t.Language = *r.Language
	}
	if r.Model != nil {
		t.Model = *r.Model
	}
	if r.Provider != nil {
		t.Provider = *r.Provider
	}
	if r.WordCount != nil {
		t.WordCount = int(*r.WordCount)
	}
	if r.DurationMs != nil {
		t.DurationMs = int(*r.DurationMs)
	}
	if r.CreatedAt.Valid {
		t.CreatedAt = r.CreatedAt.Time
	}
	return t
}

func listTranscriptionToAPI(r sqlcdb.ListTranscriptionsByCallRow) TranscriptionAPI {
	t := TranscriptionAPI{
		ID:         r.ID,
		CallID:     r.CallID,
		Source:     r.Source,
		IsPrimary:  r.IsPrimary,
		Confidence: r.Confidence,
		Words:      r.Words,
	}
	if r.Text != nil {
		t.Text = *r.Text
	}
	if r.Language != nil {
		t.Language = *r.Language
	}
	if r.Model != nil {
		t.Model = *r.Model
	}
	if r.Provider != nil {
		t.Provider = *r.Provider
	}
	if r.WordCount != nil {
		t.WordCount = int(*r.WordCount)
	}
	if r.DurationMs != nil {
		t.DurationMs = int(*r.DurationMs)
	}
	if r.CreatedAt.Valid {
		t.CreatedAt = r.CreatedAt.Time
	}
	return t
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

	qtx := db.Q.WithTx(tx)

	if row.IsPrimary {
		if err := qtx.ClearPrimaryTranscription(ctx, sqlcdb.ClearPrimaryTranscriptionParams{
			CallID:        row.CallID,
			CallStartTime: pgtype.Timestamptz{Time: row.CallStartTime, Valid: true},
		}); err != nil {
			return 0, fmt.Errorf("clear is_primary: %w", err)
		}
	}

	wc := int32(row.WordCount)
	dm := int32(row.DurationMs)
	id, err := qtx.InsertTranscriptionRow(ctx, sqlcdb.InsertTranscriptionRowParams{
		CallID:        row.CallID,
		CallStartTime: pgtype.Timestamptz{Time: row.CallStartTime, Valid: true},
		Text:          &row.Text,
		Source:        row.Source,
		IsPrimary:     row.IsPrimary,
		Confidence:    row.Confidence,
		Language:      &row.Language,
		Model:         &row.Model,
		Provider:      &row.Provider,
		WordCount:     &wc,
		DurationMs:    &dm,
		Words:         row.Words,
	})
	if err != nil {
		return 0, fmt.Errorf("insert transcription: %w", err)
	}

	if row.IsPrimary {
		status := row.Source
		if status == "human" {
			status = "verified"
		}
		if err := qtx.UpdateCallTranscriptionDenorm(ctx, sqlcdb.UpdateCallTranscriptionDenormParams{
			CallID:                 row.CallID,
			StartTime:              pgtype.Timestamptz{Time: row.CallStartTime, Valid: true},
			TranscriptionStatus:    status,
			TranscriptionText:      &row.Text,
			TranscriptionWordCount: &wc,
		}); err != nil {
			return 0, fmt.Errorf("update calls denorm: %w", err)
		}

		if err := qtx.UpdateCallGroupTranscriptionDenorm(ctx, sqlcdb.UpdateCallGroupTranscriptionDenormParams{
			CallID:              row.CallID,
			StartTime:           pgtype.Timestamptz{Time: row.CallStartTime, Valid: true},
			TranscriptionText:   &row.Text,
			TranscriptionStatus: &status,
		}); err != nil {
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
	row, err := db.Q.GetPrimaryTranscription(ctx, callID)
	if err != nil {
		return nil, err
	}
	t := primaryTranscriptionToAPI(row)
	return &t, nil
}

// ListTranscriptionsByCall returns all transcription variants for a call.
func (db *DB) ListTranscriptionsByCall(ctx context.Context, callID int64) ([]TranscriptionAPI, error) {
	rows, err := db.Q.ListTranscriptionsByCall(ctx, callID)
	if err != nil {
		return nil, err
	}
	result := make([]TranscriptionAPI, len(rows))
	for i, r := range rows {
		result[i] = listTranscriptionToAPI(r)
	}
	return result, nil
}

// SearchTranscriptions performs full-text search across transcriptions with call context.
// Defaults to primary transcriptions only; pass primary_only=false to include all variants.
func (db *DB) SearchTranscriptions(ctx context.Context, query string, filter TranscriptionSearchFilter) ([]TranscriptionSearchHit, int, error) {
	primaryOnly := filter.PrimaryOnly == nil || *filter.PrimaryOnly

	const fromClause = `FROM transcriptions t JOIN calls c ON c.call_id = t.call_id AND c.start_time = t.call_start_time`
	const whereClause = `
		WHERE t.search_vector @@ plainto_tsquery('english', $1)
		  AND ($2::boolean IS NOT TRUE OR t.is_primary = true)
		  AND ($3::timestamptz IS NULL OR t.call_start_time >= $3)
		  AND ($4::timestamptz IS NULL OR t.call_start_time < $4)
		  AND ($5::int[] IS NULL OR c.system_id = ANY($5))
		  AND ($6::int[] IS NULL OR c.site_id = ANY($6))
		  AND ($7::int[] IS NULL OR c.tgid = ANY($7))`
	args := []any{query, primaryOnly, filter.StartTime, filter.EndTime,
		pqIntArray(filter.SystemIDs), pqIntArray(filter.SiteIDs), pqIntArray(filter.Tgids)}

	// Count
	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Results with rank — reuse $1 for the rank expression
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	dataQuery := `
		SELECT t.id, t.call_id, t.text, t.source, t.is_primary,
			t.confidence, t.language, t.model, t.provider,
			t.word_count, t.duration_ms, t.words, t.created_at,
			ts_rank(t.search_vector, plainto_tsquery('english', $1)) AS rank,
			c.system_id, COALESCE(c.system_name, ''), c.tgid,
			COALESCE(c.tg_alpha_tag, ''), c.start_time, c.duration
		` + fromClause + whereClause + `
		ORDER BY rank DESC
		LIMIT $8 OFFSET $9`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, limit, filter.Offset)...)
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
	row, err := db.Q.GetCallForTranscription(ctx, callID)
	if err != nil {
		return nil, err
	}
	encrypted := false
	if row.Encrypted != nil {
		encrypted = *row.Encrypted
	}
	return &CallTranscriptionInfo{
		CallID:           row.CallID,
		StartTime:        row.StartTime.Time,
		SystemID:         row.SystemID,
		Tgid:             row.Tgid,
		Duration:         row.Duration,
		AudioFilePath:    row.AudioFilePath,
		CallFilename:     row.CallFilename,
		SrcList:          row.SrcList,
		Encrypted:        encrypted,
		HasTranscription: row.HasTranscription,
		TgAlphaTag:       row.TgAlphaTag,
		TgDescription:    row.TgDescription,
		TgTag:            row.TgTag,
		TgGroup:          row.TgGroup,
	}, nil
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

	qtx := db.Q.WithTx(tx)

	if err := qtx.UpdateCallTranscriptionStatus(ctx, sqlcdb.UpdateCallTranscriptionStatusParams{
		CallID:              callID,
		StartTime:           pgtype.Timestamptz{Time: startTime, Valid: true},
		TranscriptionStatus: status,
	}); err != nil {
		return err
	}

	if err := qtx.UpdateCallGroupTranscriptionStatus(ctx, sqlcdb.UpdateCallGroupTranscriptionStatusParams{
		CallID:              callID,
		StartTime:           pgtype.Timestamptz{Time: startTime, Valid: true},
		TranscriptionStatus: &status,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
