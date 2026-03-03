# Transcription Performance Metrics Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `provider_ms` timing to transcriptions and surface real-time ratio metrics in API responses and queue stats.

**Architecture:** Add a `provider_ms` column that isolates STT provider call time from total job time (`duration_ms`). Compute `real_time_ratio` at query time from `provider_ms / (call_duration * 1000)`. Add an in-memory ring buffer of last 100 completions to `WorkerPool` for aggregate stats in the queue stats endpoint.

**Tech Stack:** Go, PostgreSQL, sqlc (code generation), OpenAPI 3.0.3

---

### Task 1: Schema — Add `provider_ms` Column

**Files:**
- Modify: `schema.sql:450-468` (transcriptions table)
- Modify: `internal/database/migrations.go:61-72` (add migration after last one)

**Step 1: Add `provider_ms` to `schema.sql` transcriptions table**

In `schema.sql`, add `provider_ms int,` after the `duration_ms` line (line 462):

```sql
    duration_ms     int,
    provider_ms     int,
    words           jsonb,
```

**Step 2: Add migration to `migrations.go`**

Append a new migration to the `migrations` slice (after the "expand system_type CHECK" entry at line 71, before the closing `}`):

```go
	{
		name:  "add provider_ms to transcriptions",
		sql:   `ALTER TABLE transcriptions ADD COLUMN IF NOT EXISTS provider_ms int`,
		check: `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'transcriptions' AND column_name = 'provider_ms')`,
	},
```

**Step 3: Commit**

```bash
git add schema.sql internal/database/migrations.go
git commit -m "schema: add provider_ms column to transcriptions"
```

---

### Task 2: sqlc — Add `provider_ms` to SQL Queries

**Files:**
- Modify: `sql/queries/transcriptions.sql`
- Regenerate: `internal/database/sqlcdb/transcriptions.sql.go` (via `sqlc generate`)

**Step 1: Update `InsertTranscriptionRow` query**

In `sql/queries/transcriptions.sql` lines 5-11, add `provider_ms` to the INSERT:

```sql
-- name: InsertTranscriptionRow :one
INSERT INTO transcriptions (
    call_id, call_start_time, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, provider_ms, words
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;
```

**Step 2: Update `GetPrimaryTranscription` query**

In `sql/queries/transcriptions.sql` lines 30-37, add `provider_ms` to the SELECT:

```sql
-- name: GetPrimaryTranscription :one
SELECT id, call_id, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, provider_ms, words, created_at
FROM transcriptions
WHERE call_id = $1 AND is_primary = true
ORDER BY created_at DESC
LIMIT 1;
```

**Step 3: Update `ListTranscriptionsByCall` query**

In `sql/queries/transcriptions.sql` lines 39-45, add `provider_ms` to the SELECT:

```sql
-- name: ListTranscriptionsByCall :many
SELECT id, call_id, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, provider_ms, words, created_at
FROM transcriptions
WHERE call_id = $1
ORDER BY created_at DESC;
```

**Step 4: Regenerate sqlc**

Run: `sqlc generate`
Expected: No errors. `internal/database/sqlcdb/transcriptions.sql.go` updated with `ProviderMs *int32` fields in all affected structs.

**Step 5: Commit**

```bash
git add sql/queries/transcriptions.sql internal/database/sqlcdb/
git commit -m "sqlc: add provider_ms to transcription queries"
```

---

### Task 3: Database Layer — Wire `provider_ms` Through Structs

**Files:**
- Modify: `internal/database/transcriptions.go:13-27` (TranscriptionRow struct)
- Modify: `internal/database/transcriptions.go:29-44` (TranscriptionAPI struct)
- Modify: `internal/database/transcriptions.go:76-85` (TranscriptionSearchHit struct)
- Modify: `internal/database/transcriptions.go:86-152` (toAPI converter functions)
- Modify: `internal/database/transcriptions.go:154-226` (InsertTranscription)
- Modify: `internal/database/transcriptions.go:251-316` (SearchTranscriptions)

**Step 1: Add `ProviderMs` to `TranscriptionRow`**

In `internal/database/transcriptions.go`, add `ProviderMs` field after `DurationMs` (line 25):

```go
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
	ProviderMs    *int
	Words         json.RawMessage // word-level timestamps with unit attribution
}
```

**Step 2: Add `ProviderMs` to `TranscriptionAPI`**

Add after `DurationMs` (line 41):

```go
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
	ProviderMs *int            `json:"provider_ms,omitempty"`
	Words      json.RawMessage `json:"words,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
```

**Step 3: Update `primaryTranscriptionToAPI` converter**

Add after the `DurationMs` mapping (~line 113):

```go
	if r.ProviderMs != nil {
		pm := int(*r.ProviderMs)
		t.ProviderMs = &pm
	}
```

**Step 4: Update `listTranscriptionToAPI` converter**

Same pattern, after the `DurationMs` mapping (~line 146):

```go
	if r.ProviderMs != nil {
		pm := int(*r.ProviderMs)
		t.ProviderMs = &pm
	}
```

**Step 5: Update `InsertTranscription` to pass `ProviderMs`**

In `InsertTranscription` (~line 178), add `ProviderMs` param. The field needs int32 conversion like DurationMs:

```go
	wc := int32(row.WordCount)
	dm := int32(row.DurationMs)
	var pm *int32
	if row.ProviderMs != nil {
		v := int32(*row.ProviderMs)
		pm = &v
	}
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
		ProviderMs:    pm,
		Words:         row.Words,
	})
```

**Step 6: Update `SearchTranscriptions` query to select `provider_ms`**

In the `dataQuery` string (~line 280), add `t.provider_ms` after `t.duration_ms`:

```go
	dataQuery := `
		SELECT t.id, t.call_id, t.text, t.source, t.is_primary,
			t.confidence, t.language, t.model, t.provider,
			t.word_count, t.duration_ms, t.provider_ms, t.words, t.created_at,
			ts_rank(t.search_vector, plainto_tsquery('english', $1)) AS rank,
			c.system_id, COALESCE(c.system_name, ''), c.tgid,
			COALESCE(c.tg_alpha_tag, ''), c.start_time, c.duration
		` + fromClause + whereClause + `
		ORDER BY rank DESC
		LIMIT $8 OFFSET $9`
```

And update the Scan call (~line 300) to include `&h.ProviderMs`:

```go
		if err := rows.Scan(
			&h.ID, &h.CallID, &h.Text, &h.Source, &h.IsPrimary,
			&h.Confidence, &h.Language, &h.Model, &h.Provider,
			&h.WordCount, &h.DurationMs, &h.ProviderMs, &h.Words, &h.CreatedAt,
			&h.Rank,
			&h.CallSystemID, &h.CallSystemName, &h.CallTgid,
			&h.CallTgAlphaTag, &h.CallStartTime, &h.CallDuration,
		); err != nil {
```

**Step 7: Verify it compiles**

Run: `go build ./...`
Expected: Clean build (no errors).

**Step 8: Commit**

```bash
git add internal/database/transcriptions.go
git commit -m "feat: wire provider_ms through database layer"
```

---

### Task 4: Worker — Time Provider Separately and Add Ring Buffer

**Files:**
- Modify: `internal/transcribe/worker.go:36-41` (QueueStats struct)
- Modify: `internal/transcribe/worker.go:77-91` (WorkerPool struct)
- Modify: `internal/transcribe/worker.go:94-99` (NewWorkerPool)
- Modify: `internal/transcribe/worker.go:151-158` (Stats method)
- Modify: `internal/transcribe/worker.go:240-300` (processJob — provider timing)
- Modify: `internal/transcribe/worker.go:308-319` (SSE event payload)

**Step 1: Add ring buffer types and extend QueueStats**

After the existing `QueueStats` struct (line 41), add the ring buffer type:

```go
// ProviderPerformance reports aggregate STT provider performance.
type ProviderPerformance struct {
	SampleSize       int                        `json:"sample_size"`
	AvgRealTimeRatio *float64                   `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64                   `json:"avg_provider_ms"`
	ByProvider       map[string]ProviderMetrics `json:"by_provider,omitempty"`
}

// ProviderMetrics reports per-provider aggregate metrics.
type ProviderMetrics struct {
	Count            int      `json:"count"`
	AvgRealTimeRatio *float64 `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64 `json:"avg_provider_ms"`
}

// completionRecord is a single entry in the performance ring buffer.
type completionRecord struct {
	providerMs   int64
	callDuration float32 // seconds
	provider     string
	model        string
}

const perfRingSize = 100

// perfRing is a fixed-size circular buffer for recent completion metrics.
type perfRing struct {
	mu    sync.Mutex
	buf   [perfRingSize]completionRecord
	pos   int
	count int
}

func (r *perfRing) push(rec completionRecord) {
	r.mu.Lock()
	r.buf[r.pos] = rec
	r.pos = (r.pos + 1) % perfRingSize
	if r.count < perfRingSize {
		r.count++
	}
	r.mu.Unlock()
}

func (r *perfRing) snapshot() []completionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]completionRecord, r.count)
	if r.count < perfRingSize {
		copy(out, r.buf[:r.count])
	} else {
		n := copy(out, r.buf[r.pos:])
		copy(out[n:], r.buf[:r.pos])
	}
	return out
}

func (r *perfRing) performance() *ProviderPerformance {
	records := r.snapshot()
	if len(records) == 0 {
		return nil
	}

	perf := &ProviderPerformance{
		SampleSize: len(records),
		ByProvider: make(map[string]ProviderMetrics),
	}

	var totalProviderMs float64
	var totalRatio float64
	var ratioCount int

	byProvider := make(map[string]struct {
		count      int
		providerMs float64
		ratio      float64
		ratioCount int
	})

	for _, rec := range records {
		totalProviderMs += float64(rec.providerMs)
		if rec.callDuration > 0 {
			ratio := float64(rec.providerMs) / (float64(rec.callDuration) * 1000)
			totalRatio += ratio
			ratioCount++
		}

		entry := byProvider[rec.provider]
		entry.count++
		entry.providerMs += float64(rec.providerMs)
		if rec.callDuration > 0 {
			ratio := float64(rec.providerMs) / (float64(rec.callDuration) * 1000)
			entry.ratio += ratio
			entry.ratioCount++
		}
		byProvider[rec.provider] = entry
	}

	avgMs := totalProviderMs / float64(len(records))
	perf.AvgProviderMs = &avgMs
	if ratioCount > 0 {
		avgRatio := totalRatio / float64(ratioCount)
		perf.AvgRealTimeRatio = &avgRatio
	}

	for name, entry := range byProvider {
		m := ProviderMetrics{Count: entry.count}
		avgMs := entry.providerMs / float64(entry.count)
		m.AvgProviderMs = &avgMs
		if entry.ratioCount > 0 {
			avgRatio := entry.ratio / float64(entry.ratioCount)
			m.AvgRealTimeRatio = &avgRatio
		}
		perf.ByProvider[name] = m
	}

	return perf
}
```

**Step 2: Add `perfRing` field to `WorkerPool`**

In the `WorkerPool` struct (line 77-91), add after `failed`:

```go
type WorkerPool struct {
	jobs     chan Job
	db       *database.DB
	provider Provider
	opts     WorkerPoolOptions
	log      zerolog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	stopped   atomic.Bool
	completed atomic.Int64
	failed    atomic.Int64
	perf      perfRing
}
```

**Step 3: Add `Performance()` method**

After the existing `Stats()` method (line 158):

```go
// Performance returns aggregate provider performance metrics from recent completions.
func (wp *WorkerPool) Performance() *ProviderPerformance {
	return wp.perf.performance()
}
```

**Step 4: Time provider separately in `processJob`**

In `processJob`, wrap the `provider.Transcribe()` call (lines 241-255) with its own timer:

```go
	// 3. Send to STT provider
	providerStart := time.Now()
	resp, err := wp.provider.Transcribe(ctx, transcribePath, TranscribeOpts{
		// ... (existing opts unchanged)
	})
	providerMs := int(time.Since(providerStart).Milliseconds())
	if err != nil {
		return errorf("%s: %w", wp.provider.Name(), err)
	}
```

**Step 5: Pass `providerMs` to `TranscriptionRow` and push to ring buffer**

At line 288-300, update the TranscriptionRow construction:

```go
	row := &database.TranscriptionRow{
		CallID:        job.CallID,
		CallStartTime: job.CallStartTime,
		Text:          text,
		Source:        "auto",
		IsPrimary:     true,
		Language:      resp.Language,
		Model:         wp.provider.Model(),
		Provider:      wp.provider.Name(),
		WordCount:     wordCount,
		DurationMs:    durationMs,
		ProviderMs:    &providerMs,
		Words:         wordsJSON,
	}
```

After the DB insert succeeds (after line 305), push to the ring buffer:

```go
	// Track provider performance
	wp.perf.push(completionRecord{
		providerMs:   int64(providerMs),
		callDuration: job.Duration,
		provider:     wp.provider.Name(),
		model:        wp.provider.Model(),
	})
```

**Step 6: Add `provider_ms` and `real_time_ratio` to SSE event payload**

Update the PublishEvent call (lines 308-319):

```go
	if wp.opts.PublishEvent != nil {
		payload := map[string]any{
			"call_id":     job.CallID,
			"system_id":   job.SystemID,
			"tgid":        job.Tgid,
			"text":        text,
			"word_count":  wordCount,
			"segments":    len(tw.Segments),
			"model":       wp.provider.Model(),
			"duration_ms": durationMs,
			"provider_ms": providerMs,
		}
		if job.Duration > 0 {
			payload["real_time_ratio"] = float64(providerMs) / (float64(job.Duration) * 1000)
		}
		wp.opts.PublishEvent("transcription", job.SystemID, job.Tgid, payload)
	}
```

**Step 7: Update the debug log to include `provider_ms`**

Update the log at lines 321-327:

```go
	log.Debug().
		Int64("call_id", job.CallID).
		Int("tgid", job.Tgid).
		Int("words", wordCount).
		Int("segments", len(tw.Segments)).
		Int("duration_ms", durationMs).
		Int("provider_ms", providerMs).
		Msg("transcription complete")
```

**Step 8: Verify it compiles**

Run: `go build ./...`
Expected: Clean build.

**Step 9: Commit**

```bash
git add internal/transcribe/worker.go
git commit -m "feat: track provider_ms separately and add performance ring buffer"
```

---

### Task 5: Pipeline + API — Surface Performance in Queue Stats

**Files:**
- Modify: `internal/api/live_data.go:150-155` (TranscriptionQueueStatsData struct)
- Modify: `internal/ingest/pipeline.go:317-328` (TranscriptionQueueStats method)
- Modify: `internal/api/transcriptions.go:258-281` (GetQueueStats handler)
- Modify: `internal/api/affiliations_test.go:25` (mock)

**Step 1: Add `Performance` field to `TranscriptionQueueStatsData`**

In `internal/api/live_data.go`, import `transcribe` and update the struct:

```go
// TranscriptionQueueStatsData reports transcription queue statistics.
type TranscriptionQueueStatsData struct {
	Pending     int                          `json:"pending"`
	Completed   int64                        `json:"completed"`
	Failed      int64                        `json:"failed"`
	Performance *TranscriptionPerformanceData `json:"performance,omitempty"`
}

// TranscriptionPerformanceData reports aggregate STT performance.
type TranscriptionPerformanceData struct {
	SampleSize       int                                    `json:"sample_size"`
	AvgRealTimeRatio *float64                               `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64                               `json:"avg_provider_ms"`
	ByProvider       map[string]TranscriptionProviderMetrics `json:"by_provider,omitempty"`
}

// TranscriptionProviderMetrics reports per-provider metrics.
type TranscriptionProviderMetrics struct {
	Count            int      `json:"count"`
	AvgRealTimeRatio *float64 `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64 `json:"avg_provider_ms"`
}
```

Note: We define these types in the `api` package (not import from `transcribe`) to avoid a circular dependency. The pipeline bridges the two.

**Step 2: Update `Pipeline.TranscriptionQueueStats()`**

In `internal/ingest/pipeline.go` lines 317-328, extend to include performance:

```go
func (p *Pipeline) TranscriptionQueueStats() *api.TranscriptionQueueStatsData {
	if p.transcriber == nil {
		return nil
	}
	stats := p.transcriber.Stats()
	result := &api.TranscriptionQueueStatsData{
		Pending:   stats.Pending,
		Completed: stats.Completed,
		Failed:    stats.Failed,
	}

	if perf := p.transcriber.Performance(); perf != nil {
		pd := &api.TranscriptionPerformanceData{
			SampleSize:       perf.SampleSize,
			AvgRealTimeRatio: perf.AvgRealTimeRatio,
			AvgProviderMs:    perf.AvgProviderMs,
		}
		if len(perf.ByProvider) > 0 {
			pd.ByProvider = make(map[string]api.TranscriptionProviderMetrics, len(perf.ByProvider))
			for name, m := range perf.ByProvider {
				pd.ByProvider[name] = api.TranscriptionProviderMetrics{
					Count:            m.Count,
					AvgRealTimeRatio: m.AvgRealTimeRatio,
					AvgProviderMs:    m.AvgProviderMs,
				}
			}
		}
		result.Performance = pd
	}

	return result
}
```

**Step 3: Update the mock in `affiliations_test.go`**

If the mock returns `nil`, the signature doesn't change — it should still compile since `TranscriptionQueueStatsData` is the return type. Verify: `go build ./...`

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Clean build.

**Step 5: Commit**

```bash
git add internal/api/live_data.go internal/ingest/pipeline.go internal/api/affiliations_test.go
git commit -m "feat: surface transcription performance metrics in queue stats API"
```

---

### Task 6: OpenAPI — Update Spec

**Files:**
- Modify: `openapi.yaml:3693-3696` (Transcription schema — add `provider_ms`)
- Modify: `openapi.yaml:3858-3872` (TranscriptionQueueStats schema — add `performance`)

**Step 1: Add `provider_ms` and `real_time_ratio` to Transcription schema**

After the `duration_ms` property (~line 3696), add:

```yaml
        provider_ms:
          type: integer
          nullable: true
          description: Time spent in the STT provider call in milliseconds (excludes file I/O, preprocessing, DB writes)
          example: 1200
        real_time_ratio:
          type: number
          nullable: true
          description: "Ratio of provider processing time to call audio duration. Values < 1.0 mean faster than real-time. Computed as provider_ms / (call_duration * 1000)."
          example: 0.42
```

**Step 2: Add `performance` to TranscriptionQueueStats schema**

After the `failed` property (~line 3872), add:

```yaml
        performance:
          type: object
          nullable: true
          description: Aggregate STT performance from the last 100 completions
          properties:
            sample_size:
              type: integer
              example: 100
            avg_real_time_ratio:
              type: number
              nullable: true
              example: 0.38
            avg_provider_ms:
              type: number
              nullable: true
              example: 1150
            by_provider:
              type: object
              additionalProperties:
                type: object
                properties:
                  count:
                    type: integer
                    example: 85
                  avg_real_time_ratio:
                    type: number
                    nullable: true
                    example: 0.35
                  avg_provider_ms:
                    type: number
                    nullable: true
                    example: 1050
```

**Step 3: Commit**

```bash
git add openapi.yaml
git commit -m "docs: add provider_ms and performance metrics to OpenAPI spec"
```

---

### Task 7: CLAUDE.md — Update Documentation

**Files:**
- Modify: `CLAUDE.md` (mention `provider_ms` in the transcription pipeline section)

**Step 1: Update the Implementation Status section**

In the transcription pipeline bullet under "Completed", append mention of performance metrics:

Add after "anti-hallucination parameters":
```
Performance tracking: `provider_ms` isolates STT call time from total `duration_ms`; queue stats endpoint includes rolling real-time ratio averages.
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document transcription performance metrics in CLAUDE.md"
```

---

### Task 8: Build and Smoke Test

**Step 1: Full build**

Run: `go build ./...`
Expected: Clean build with no errors.

**Step 2: Run any existing tests**

Run: `go test ./internal/...`
Expected: All existing tests pass.

**Step 3: Commit (if any fixes were needed)**

Only if changes were required to make tests pass.
