# DVCF Transcription Race Condition Fix

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the race condition where `handleAudio` enqueues IMBE transcription before the `.dvcf` file arrives on a separate MQTT topic, causing transcription to fail.

**Architecture:** Move transcription enqueueing for IMBE from `handleAudio` to `handleDvcf`. When `STT_PROVIDER=imbe`, `handleAudio` skips enqueueing unless the DVCF data was embedded in the audio message. `handleDvcf` finds the matching call by `(system_name, tgid, start_time)` and enqueues transcription. A new DB query supports lookup by `system_name` instead of `system_id` since the DVCF plugin doesn't send `instance_id`.

**Tech Stack:** Go, PostgreSQL, pgx

---

### Task 1: Add `ProviderName()` to WorkerPool

**Files:**
- Modify: `internal/transcribe/worker.go:291-294`

- [ ] **Step 1: Add `ProviderName` method**

In `internal/transcribe/worker.go`, after the existing `Model()` method on line 291, add:

```go
// ProviderName returns the STT provider name (e.g. "whisper", "imbe").
func (wp *WorkerPool) ProviderName() string { return wp.provider.Name() }
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/transcribe/worker.go
git commit -m "feat(transcribe): expose ProviderName on WorkerPool"
```

---

### Task 2: Add `FindCallBySystemName` DB query

**Files:**
- Modify: `internal/database/sqlcdb/calls.sql.go` (add query + types + method)
- Modify: `internal/database/calls.go` (add wrapper)

The DVCF plugin sends `short_name` but no `instance_id`, so we can't resolve `system_id` through identity resolution. Instead, query `calls` directly using the denormalized `system_name` column.

- [ ] **Step 1: Add the sqlc-style query to `internal/database/sqlcdb/calls.sql.go`**

Add after the `FindCallForAudio` function (after line 65):

```go
const findCallBySystemName = `-- name: FindCallBySystemName :one
SELECT call_id, start_time, system_id, tgid,
       COALESCE(audio_file_path, '') AS audio_file_path,
       COALESCE(call_filename, '') AS call_filename,
       COALESCE(duration, 0) AS duration,
       COALESCE(src_list, 'null'::jsonb) AS src_list,
       COALESCE(tg_alpha_tag, '') AS tg_alpha_tag,
       COALESCE(tg_description, '') AS tg_description,
       COALESCE(tg_tag, '') AS tg_tag,
       COALESCE(tg_group, '') AS tg_group
FROM calls
WHERE system_name = $1 AND tgid = $2
    AND start_time BETWEEN $3::timestamptz - interval '5 seconds' AND $3::timestamptz + interval '5 seconds'
ORDER BY ABS(EXTRACT(EPOCH FROM (start_time - $3::timestamptz)))
LIMIT 1
`

type FindCallBySystemNameParams struct {
	SystemName string
	Tgid       int
	Column3    pgtype.Timestamptz
}

type FindCallBySystemNameRow struct {
	CallID        int64
	StartTime     pgtype.Timestamptz
	SystemID      int
	Tgid          int
	AudioFilePath string
	CallFilename  string
	Duration      float32
	SrcList       []byte
	TgAlphaTag    string
	TgDescription string
	TgTag         string
	TgGroup       string
}

func (q *Queries) FindCallBySystemName(ctx context.Context, arg FindCallBySystemNameParams) (FindCallBySystemNameRow, error) {
	row := q.db.QueryRow(ctx, findCallBySystemName, arg.SystemName, arg.Tgid, arg.Column3)
	var i FindCallBySystemNameRow
	err := row.Scan(
		&i.CallID, &i.StartTime, &i.SystemID, &i.Tgid,
		&i.AudioFilePath, &i.CallFilename, &i.Duration, &i.SrcList,
		&i.TgAlphaTag, &i.TgDescription, &i.TgTag, &i.TgGroup,
	)
	return i, err
}
```

- [ ] **Step 2: Add wrapper in `internal/database/calls.go`**

Add after the `FindCallFuzzy` method (after line 477):

```go
// DvcfCallMatch holds the call fields needed to enqueue IMBE transcription
// from a standalone DVCF message (which lacks instance_id for identity resolution).
type DvcfCallMatch struct {
	CallID        int64
	StartTime     time.Time
	SystemID      int
	Tgid          int
	AudioFilePath string
	CallFilename  string
	Duration      float32
	SrcList       json.RawMessage
	TgAlphaTag    string
	TgDescription string
	TgTag         string
	TgGroup       string
}

// FindCallBySystemName finds a call by system_name + tgid + start_time (±5s).
// Used by the DVCF handler which has short_name but no instance_id.
func (db *DB) FindCallBySystemName(ctx context.Context, systemName string, tgid int, startTime time.Time) (*DvcfCallMatch, error) {
	row, err := db.Q.FindCallBySystemName(ctx, sqlcdb.FindCallBySystemNameParams{
		SystemName: systemName,
		Tgid:       tgid,
		Column3:    pgtz(startTime),
	})
	if err != nil {
		return nil, err
	}
	return &DvcfCallMatch{
		CallID:        row.CallID,
		StartTime:     row.StartTime.Time,
		SystemID:      row.SystemID,
		Tgid:          row.Tgid,
		AudioFilePath: row.AudioFilePath,
		CallFilename:  row.CallFilename,
		Duration:      row.Duration,
		SrcList:       row.SrcList,
		TgAlphaTag:    row.TgAlphaTag,
		TgDescription: row.TgDescription,
		TgTag:         row.TgTag,
		TgGroup:       row.TgGroup,
	}, nil
}
```

You'll need to add `"encoding/json"` to the imports of `calls.go` if not already present.

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/database/sqlcdb/calls.sql.go internal/database/calls.go
git commit -m "feat(database): add FindCallBySystemName for DVCF handler lookup"
```

---

### Task 3: Skip transcription enqueueing in `handleAudio` for IMBE provider

**Files:**
- Modify: `internal/ingest/handler_audio.go:112-118`

When the STT provider is `"imbe"` and the audio message did NOT include embedded DVCF data (`audio_dvcf_base64` was empty), skip `enqueueTranscription`. The standalone `/dvcf` message will handle it.

If `audio_dvcf_base64` WAS present, the `.dvcf` file was already saved alongside the audio (lines 84-98), so it's safe to enqueue immediately — no race.

- [ ] **Step 1: Add `isIMBEProvider` helper to Pipeline**

In `internal/ingest/pipeline.go`, add after the `shouldTranscribeTG` method:

```go
// isIMBEProvider returns true if the transcription provider is the IMBE ASR provider.
func (p *Pipeline) isIMBEProvider() bool {
	if p.transcriber == nil {
		return false
	}
	return p.transcriber.ProviderName() == "imbe"
}
```

- [ ] **Step 2: Guard the transcription enqueue in `handleAudio`**

In `internal/ingest/handler_audio.go`, replace lines 112-118:

```go
	// Enqueue for transcription if audio was saved and call is not encrypted
	if callID > 0 && meta.Encrypted == 0 {
		if meta.Transcript != "" {
			p.insertSourceTranscription(callID, callStartTime, identity.SystemID, meta.Talkgroup, meta)
		} else {
			p.enqueueTranscription(callID, callStartTime, identity.SystemID, audioPath, meta)
		}
	}
```

with:

```go
	// Enqueue for transcription if audio was saved and call is not encrypted
	if callID > 0 && meta.Encrypted == 0 {
		if meta.Transcript != "" {
			p.insertSourceTranscription(callID, callStartTime, identity.SystemID, meta.Talkgroup, meta)
		} else if p.isIMBEProvider() && msg.Call.AudioDvcfBase64 == "" {
			// IMBE provider needs the .dvcf file. If it wasn't embedded in this audio
			// message, the standalone /dvcf handler will enqueue transcription when
			// the DVCF message arrives.
			p.log.Debug().Int64("call_id", callID).Msg("skipping transcription enqueue — waiting for standalone DVCF message")
		} else {
			p.enqueueTranscription(callID, callStartTime, identity.SystemID, audioPath, meta)
		}
	}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/ingest/pipeline.go internal/ingest/handler_audio.go
git commit -m "fix(ingest): skip IMBE transcription enqueue when DVCF not yet available"
```

---

### Task 4: Enqueue transcription from `handleDvcf`

**Files:**
- Modify: `internal/ingest/handler_dvcf.go`

After saving the `.dvcf` file, look up the matching call and enqueue transcription if the IMBE provider is active.

- [ ] **Step 1: Rewrite `handleDvcf` to enqueue transcription**

Replace the entire contents of `internal/ingest/handler_dvcf.go` with:

```go
package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/snarg/tr-engine/internal/transcribe"
)

func (p *Pipeline) handleDvcf(payload []byte) error {
	var msg DvcfMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return fmt.Errorf("unmarshal dvcf message: %w", err)
	}

	if msg.AudioDvcfBase64 == "" {
		p.log.Warn().Msg("dvcf message has no audio_dvcf_base64, skipping")
		return nil
	}

	meta := &msg.Metadata
	if meta.Filename == "" {
		p.log.Warn().Int("tgid", meta.Talkgroup).Msg("dvcf message has no filename in metadata, skipping")
		return nil
	}

	// Decode the base64 DVCF data
	dvcfData, err := base64.StdEncoding.DecodeString(msg.AudioDvcfBase64)
	if err != nil {
		return fmt.Errorf("decode dvcf base64: %w", err)
	}

	// Derive the save path: same structure as audio files but with .dvcf extension
	startTime := time.Unix(meta.StartTime, 0)
	audioFilename := meta.Filename
	ext := filepath.Ext(audioFilename)
	dvcfFilename := strings.TrimSuffix(audioFilename, ext) + ".dvcf"
	dvcfKey := buildAudioRelPath(meta.ShortName, startTime, dvcfFilename)

	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	if err := p.saveAudio(ctx, dvcfKey, dvcfData, "application/octet-stream"); err != nil {
		return fmt.Errorf("save dvcf file: %w", err)
	}

	p.log.Debug().
		Str("dvcf_key", dvcfKey).
		Int("dvcf_size", len(dvcfData)).
		Int("tgid", meta.Talkgroup).
		Str("sys_name", meta.ShortName).
		Msg("dvcf file saved")

	// Enqueue transcription if IMBE provider is active
	if !p.isIMBEProvider() {
		return nil
	}

	// Find the matching call by system_name + tgid + start_time
	call, err := p.db.FindCallBySystemName(ctx, meta.ShortName, meta.Talkgroup, startTime)
	if err != nil {
		p.log.Warn().
			Err(err).
			Str("sys_name", meta.ShortName).
			Int("tgid", meta.Talkgroup).
			Int64("start_time", meta.StartTime).
			Msg("dvcf: no matching call found, transcription will rely on backfill")
		return nil
	}

	// Check duration and talkgroup filters
	if call.Duration < float32(p.transcriber.MinDuration()) || call.Duration > float32(p.transcriber.MaxDuration()) {
		return nil
	}
	if !p.shouldTranscribeTG(call.SystemID, call.Tgid) {
		return nil
	}

	job := transcribe.Job{
		CallID:        call.CallID,
		CallStartTime: call.StartTime,
		SystemID:      call.SystemID,
		Tgid:          call.Tgid,
		Duration:      call.Duration,
		AudioFilePath: call.AudioFilePath,
		CallFilename:  call.CallFilename,
		SrcList:       call.SrcList,
		TgAlphaTag:    call.TgAlphaTag,
		TgDescription: call.TgDescription,
		TgTag:         call.TgTag,
		TgGroup:       call.TgGroup,
	}
	if !p.transcriber.Enqueue(job) {
		p.log.Warn().Int64("call_id", call.CallID).Msg("dvcf: transcription queue full, skipping")
	}

	return nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/ingest/handler_dvcf.go
git commit -m "feat(ingest): enqueue IMBE transcription from handleDvcf after file save"
```

---

### Task 5: Integration verification

- [ ] **Step 1: Verify full build**

Run: `go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 2: Run existing tests**

Run: `go test ./...`
Expected: All existing tests pass (no regressions).

- [ ] **Step 3: Verify the three code paths work logically**

Review the three scenarios end-to-end:

1. **Embedded DVCF (audio message includes `audio_dvcf_base64`):** `handleAudio` saves audio + `.dvcf`, enqueues transcription immediately. `handleDvcf` may also fire but the `.dvcf` is already saved, and it enqueues a duplicate job (harmless — the worker will transcribe and the second insert will be a no-op since `is_primary` is already set).

2. **Standalone DVCF (separate `/dvcf` topic):** `handleAudio` saves audio, skips transcription enqueue (IMBE provider + no embedded DVCF). `handleDvcf` saves `.dvcf`, finds the call, enqueues transcription.

3. **DVCF arrives before audio (unlikely but possible):** `handleDvcf` saves `.dvcf`, can't find call → logs warning, returns nil. `handleAudio` arrives, saves audio, skips enqueue. Backfill manager picks up the untranscribed call later.

4. **Non-IMBE provider:** Both handlers behave as before. `handleAudio` enqueues normally. `handleDvcf` just saves the file.

- [ ] **Step 4: Commit all changes together if not already committed**

If any uncommitted changes remain:

```bash
git add -A
git commit -m "feat(ingest): fix DVCF transcription race condition

Move IMBE transcription enqueueing from handleAudio to handleDvcf for
standalone DVCF messages. handleAudio still enqueues when DVCF data is
embedded in the audio message. Adds FindCallBySystemName query for
DVCF handler since the plugin doesn't send instance_id."
```
