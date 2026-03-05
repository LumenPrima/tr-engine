# Call Data Export/Import Design (Phase 2)

**Date:** 2026-03-05
**Status:** Approved
**Depends on:** Phase 1 metadata export/import (committed 2026-03-05, `9f0246b`)

## Problem

Phase 1 implemented metadata-only export/import (systems, sites, talkgroups, units). Phase 2 adds call data: calls, call_groups, transcriptions, and optionally audio files. This enables full instance migration and backup/restore.

## CLI Interface

### Export

```bash
# Metadata only (existing, default)
tr-engine export --output backup.tar.gz

# Full: metadata + calls + transcriptions (no audio)
tr-engine export --output backup.tar.gz --mode full

# Full with audio files
tr-engine export --output backup.tar.gz --mode full --include-audio

# Time-filtered
tr-engine export --output backup.tar.gz --mode full --start 2026-02-20 --end 2026-02-23

# System-filtered (existing)
tr-engine export --output backup.tar.gz --mode full --systems 3
```

New flags:
- `--mode metadata|full` (default: `metadata`)
- `--include-audio` (default: false, only with `--mode full`)
- `--start`, `--end` (ISO 8601 date or datetime, optional time range for calls)

### Import

No new flags needed. Existing `--mode full|metadata|calls` already covers it:
- `metadata` — systems, sites, talkgroups, units only
- `calls` — call_groups, calls, transcriptions, audio only (requires systems already exist)
- `full` — everything

## Archive Structure (Full Mode)

```
tr-engine-export-{timestamp}.tar.gz
├── manifest.json           # updated with call counts + time range
├── systems.jsonl           # existing
├── sites.jsonl             # existing
├── talkgroups.jsonl        # existing
├── talkgroup_directory.jsonl # existing
├── units.jsonl             # existing
├── calls.jsonl             # NEW
├── transcriptions.jsonl    # NEW
└── audio/                  # NEW (only with --include-audio)
    └── {relative_path}     # matches calls[].audio_file_path
```

Call groups are NOT a separate file — they are created on-demand during import from call data.

## JSONL Record Types

### CallRecord

```json
{
  "_v": 1,
  "system_ref": {"sysid": "348", "wacn": "BEE00"},
  "site_ref": {"instance_id": "tr-1", "short_name": "butco"},
  "tgid": 24513,
  "start_time": "2026-03-01T14:30:00Z",
  "stop_time": "2026-03-01T14:30:45Z",
  "duration": 45.2,
  "freq": 851250000,
  "emergency": false,
  "encrypted": false,
  "analog": false,
  "conventional": false,
  "phase2_tdma": false,
  "audio_type": "digital",
  "audio_file_path": "butco/2026-02-21/9101-1771661569_855962500.0-call_31405.m4a",
  "src_list": [...],
  "freq_list": [...],
  "unit_ids": [12345, 67890],
  "patched_tgids": [],
  "metadata_json": {...}
}
```

**Included fields:** All non-derived call fields that carry actual data.

**Excluded fields** (recomputed on import from local data):
- `call_id`, `system_id`, `site_id`, `call_group_id` (DB-internal IDs)
- `system_name`, `site_short_name`, `tg_alpha_tag`, `tg_description`, `tg_tag`, `tg_group` (denormalized cache)
- `has_transcription`, `transcription_status`, `transcription_text`, `transcription_word_count` (derived from transcriptions table)
- `call_frequencies`, `call_transmissions` (rebuilt from `freq_list`/`src_list` JSONB)
- `search_vector` (rebuilt by PostgreSQL triggers)

### TranscriptionRecord

```json
{
  "_v": 1,
  "system_ref": {"sysid": "348", "wacn": "BEE00"},
  "tgid": 24513,
  "call_start_time": "2026-03-01T14:30:00Z",
  "text": "Engine 5 responding to Main Street",
  "source": "auto",
  "confidence": 0.92,
  "language": "en",
  "model": "whisper-large-v3-turbo",
  "provider": "deepinfra",
  "word_count": 6,
  "duration_ms": 1200,
  "provider_ms": 800,
  "words": [...]
}
```

Anchored to parent call by `(system_ref, tgid, call_start_time)` — resolved via fuzzy ±5s match on import.

## Export Implementation

### Data Flow

1. Load metadata (existing Phase 1 logic)
2. Query calls with optional time range and system filter
3. Query transcriptions joined to calls for same filters
4. Write metadata JSONL files (existing)
5. Write `calls.jsonl` — buffered in memory (~50MB for 62K calls, acceptable)
6. Write `transcriptions.jsonl` — buffered in memory
7. If `--include-audio`: for each call with `audio_file_path`, stat the file and add to `audio/` prefix in tar

### New DB Functions

```go
// ExportCalls returns all calls for the given systems and optional time range.
func (db *DB) ExportCalls(ctx context.Context, systemIDs []int, start, end *time.Time) ([]CallExport, error)

// ExportTranscriptions returns all transcriptions for the given systems and optional time range.
func (db *DB) ExportTranscriptions(ctx context.Context, systemIDs []int, start, end *time.Time) ([]TranscriptionExport, error)
```

### Audio File Handling

Audio files are added to the tar archive under `audio/` prefix, preserving the relative path from `calls.audio_file_path`. Files are read from the configured audio directory (`AUDIO_DIR`). Missing audio files are skipped with a warning (calls may exist without audio).

## Import Implementation

Extends the existing staged import pipeline.

### Stage 5: Calls (mode `full` or `calls`)

For each call record:
1. Resolve `system_ref` → `system_id` via sysMap
2. Resolve `site_ref` → `site_id` via siteMap (skip if site not found)
3. **Fuzzy dedup**: check `(system_id, tgid, start_time ± 5s)` for existing call
   - Duplicate found → skip
   - Not duplicate → continue
4. Find or create call_group via existing `UpsertCallGroup` (has built-in fuzzy match)
5. Insert call via `InsertCall` with denormalized fields populated from current local data
6. Rebuild `call_frequencies` and `call_transmissions` from `freq_list`/`src_list` JSONB using existing `InsertCallFrequencies`/`InsertCallTransmissions`

### Stage 6: Transcriptions

For each transcription record:
1. Resolve parent call by `(system_id, tgid, call_start_time ± 5s)` fuzzy match
2. If call not found → skip with warning
3. Dedup/merge logic:
   - No existing transcription → insert
   - `source='human'` always wins over `auto`/`llm`
   - For same source type, higher `confidence` wins
   - Ties → keep existing
4. Insert via `InsertTranscription`, update `calls.transcription_status`

### Stage 7: Audio Files (when archive contains `audio/`)

1. Scan tar entries under `audio/` prefix
2. For each audio file, write to local audio directory (`AUDIO_DIR`)
3. Preserve relative path structure
4. Skip if file already exists at target path (idempotent)

### Post-Import

Same as Phase 1 plus:
- Ensure partitions exist for imported time ranges (call `create_monthly_partition` for each month in range)
- Refresh talkgroup stats

## Manifest Changes

```go
type ManifestFilters struct {
    SystemIDs    []int      `json:"system_ids,omitempty"`
    TimeRange    *TimeRange `json:"time_range,omitempty"`
    IncludeAudio bool       `json:"include_audio"`
}

type TimeRange struct {
    Start *time.Time `json:"start,omitempty"`
    End   *time.Time `json:"end,omitempty"`
}

type ManifestCounts struct {
    Systems            int `json:"systems"`
    Sites              int `json:"sites"`
    Talkgroups         int `json:"talkgroups"`
    TalkgroupDirectory int `json:"talkgroup_directory"`
    Units              int `json:"units"`
    Calls              int `json:"calls"`
    Transcriptions     int `json:"transcriptions"`
    AudioFiles         int `json:"audio_files"`
}
```

## Size Estimates (Dev DB)

| Entity | Count | Est. JSONL Size |
|--------|-------|-----------------|
| Calls | 62K | ~50MB |
| Transcriptions | 25K | ~20MB |
| Audio files | ~62K | ~2-8GB |
| Metadata | ~8K entities | ~1MB |

Without audio, a full export is ~70MB compressed. With audio, potentially several GB.

## Not In Scope

- Resumability/checkpointing (future enhancement)
- Batch/COPY FROM optimization (row-by-row is fine for import volumes)
- unit_events and trunking_messages export (optional in design doc, deferred)
- HTTP endpoints (CLI only for now)
