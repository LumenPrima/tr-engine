# Transcription Performance Metrics Design

**Date:** 2026-03-03
**Status:** Approved

## Problem

Users want to compare transcription performance across providers, models, and hardware configurations. The existing `duration_ms` field on transcriptions captures total job time (file I/O + preprocessing + STT call + DB insert), which is too noisy for provider-to-provider comparison.

## Solution

Add a `provider_ms` column that isolates the STT provider call time. Compute real-time ratio (`provider_ms / call_duration_ms`) at query time. Surface aggregate stats via a rolling in-memory window.

## Schema Change

Add one column to `transcriptions`:

```sql
ALTER TABLE transcriptions ADD COLUMN IF NOT EXISTS provider_ms INT;
```

- `provider_ms` — wall-clock time spent in `Provider.Transcribe()` only
- Existing `duration_ms` stays as total job time
- `NULL` for legacy transcriptions and human corrections

## Worker Change

In `worker.go`, wrap `provider.Transcribe()` with its own timer:

```go
providerStart := time.Now()
resp, err := wp.provider.Transcribe(ctx, audioPath, opts)
providerMs := time.Since(providerStart).Milliseconds()
```

Pass `providerMs` into the `TranscriptionRow`.

## In-Memory Stats Ring Buffer

Fixed-size circular array (100 entries) in `WorkerPool`:

```go
type completionRecord struct {
    providerMs   int64
    callDuration float32  // seconds
    provider     string
    model        string
}
```

Push on each successful transcription. Read by `/transcriptions/queue` to compute averages.

## API Response Changes

### Transcription responses

Add two fields:

```json
{
  "provider_ms": 1200,
  "real_time_ratio": 0.42
}
```

- `real_time_ratio` = `provider_ms / (call_duration_seconds * 1000)`
- Values < 1.0 mean faster than real-time
- `null` when `provider_ms` or call duration is missing

### GET /transcriptions/queue

Add performance section:

```json
{
  "status": "ok",
  "pending": 15,
  "completed": 5000,
  "failed": 12,
  "performance": {
    "sample_size": 100,
    "avg_real_time_ratio": 0.38,
    "avg_provider_ms": 1150,
    "by_provider": {
      "whisper": { "count": 85, "avg_real_time_ratio": 0.35, "avg_provider_ms": 1050 },
      "deepinfra": { "count": 15, "avg_real_time_ratio": 0.52, "avg_provider_ms": 1580 }
    }
  }
}
```

## SSE Event

Add `provider_ms` and `real_time_ratio` to the existing `transcription` SSE event payload.

## OpenAPI Spec

Update `openapi.yaml` with new fields on `Transcription` schema and `TranscriptionQueueStats` schema.

## Migration

Single migration in `migrations.go`:
- `ALTER TABLE transcriptions ADD COLUMN IF NOT EXISTS provider_ms INT`
- Check: column exists in `information_schema.columns`
- Backfill: not needed (NULL is acceptable for historical data)
