# Export/Import Feature Design

**Date:** 2026-03-02
**Status:** Approved

## Problem

tr-engine needs a portable export/import capability that serves four use cases:

1. **Instance migration** — move data between deployments
2. **Data sharing/federation** — share data between independent instances
3. **Backup & restore** — disaster recovery and archival
4. **Seed new instances** — bootstrap with metadata (talkgroups, units, system config)

Data must be DB-agnostic: no internal database IDs, no PostgreSQL-specific formats. Imports into existing instances must deduplicate, merge, and enrich without destroying existing data.

## Architecture: Flat Entity Files (Approach A)

Export produces a tar.gz archive containing one JSONL file per entity type, plus a manifest. Every record uses natural keys for cross-references instead of database IDs. Import processes files in dependency order, resolving natural keys to local DB IDs.

## Export Format

### Archive Structure

```
tr-engine-export-{timestamp}.tar.gz
├── manifest.json
├── systems.jsonl
├── sites.jsonl
├── talkgroups.jsonl
├── talkgroup_directory.jsonl
├── units.jsonl
├── call_groups.jsonl
├── calls.jsonl
├── transcriptions.jsonl
├── unit_events.jsonl          # optional (opt-in)
├── trunking_messages.jsonl    # optional (opt-in)
└── audio/                     # optional (opt-in)
    └── {relative_path}        # matching calls[].audio_file_path
```

### Manifest

```json
{
  "version": 1,
  "format": "tr-engine-export",
  "created_at": "2026-03-02T15:04:05Z",
  "source_instance": "tr-engine v1.2.3",
  "filters": {
    "system_ids": [348],
    "time_range": {"start": "2026-02-01T00:00:00Z", "end": "2026-03-01T00:00:00Z"},
    "include_audio": true,
    "include_events": false
  },
  "counts": {
    "systems": 1, "sites": 2, "talkgroups": 150,
    "units": 500, "calls": 12000, "transcriptions": 11500
  }
}
```

### Entity Natural Keys

| Entity | Natural Key | Reference to Parent |
|--------|------------|-------------------|
| System | P25: `(sysid, wacn)`, Conv: `(type, name)` | — |
| Site | `(instance_id, short_name)` | system by natural key |
| Talkgroup | `(tgid)` | system by natural key |
| Unit | `(unit_id)` | system by natural key |
| Call Group | `(tgid, start_time)` | system by natural key |
| Call | `(tgid, start_time, instance_id)` | system, site, call_group by natural keys |
| Transcription | `(source, created_at)` | call by `(tgid, start_time, instance_id)` |

### Example Records

**System (P25):**
```json
{"sysid": "348", "wacn": "BEE00", "name": "Butler/Warren", "type": "p25"}
```

**System (Conventional):**
```json
{"type": "conventional", "name": "Local Fire", "conv_instance_id": "trunk-recorder", "conv_sys_name": "local_fire"}
```

**Talkgroup:**
```json
{"system_ref": {"sysid": "348", "wacn": "BEE00"}, "tgid": 24513, "alpha_tag": "BC Fire Dispatch", "alpha_tag_source": "manual", "tag": "Fire Dispatch", "group": "Butler County", "description": "Butler County Fire/EMS Dispatch", "mode": "D", "priority": 1}
```

**Call:**
```json
{"system_ref": {"sysid": "348", "wacn": "BEE00"}, "site_ref": {"instance_id": "tr-1", "short_name": "butco"}, "tgid": 24513, "start_time": "2026-03-01T14:30:00Z", "stop_time": "2026-03-01T14:30:45Z", "duration": 45.2, "freq": 851250000, "emergency": false, "encrypted": false, "src_list": [...], "freq_list": [...], "audio_file_path": "audio/2026/03/01/24513-1709300200.wav", "metadata_json": {...}}
```

### Fields Excluded from Export

These are DB-internal or derived and are not exported:

- All auto-increment IDs (`system_id`, `site_id`, `call_id`, etc.)
- Foreign key IDs (replaced by natural key references)
- Denormalized cache fields (`system_name`, `tg_alpha_tag` on calls — recomputed on import)
- Computed stats (`call_count_30d`, `calls_1h`, `calls_24h`, `unit_count_30d` — refreshed after import)
- Search vectors (`search_vector` tsvector columns — rebuilt by PostgreSQL triggers)
- Relational copies (`call_frequencies`, `call_transmissions` — rebuilt from `freq_list`/`src_list` JSONB)

## Import Logic

### Resolution & Merge Strategy

Import processes files in dependency order. Each stage builds a resolution map for subsequent stages.

**Stage 1: Systems**
- P25: match by `(sysid, wacn)`. Found → merge (fill empty fields). Not found → create.
- Conventional: match by `(type, name)` or `(instance_id, sys_name)`. Found → merge. Not found → create.
- Optional: `--system-map "Exported Name=local_system_id"` for manual mapping.
- Result: `system_ref → local system_id` map.

**Stage 2: Sites**
- Match by `(system_id, instance_id, short_name)` using resolved system_id.
- Found → update empty fields. Not found → create.
- Result: `site_ref → local site_id` map.

**Stage 3: Talkgroups**
- Match by `(system_id, tgid)`. Uses existing `UpsertTalkgroup` logic.
- Respects tag source priority: `manual > csv > mqtt > directory`.
- Enrichment: fills empty `description`, `tag`, `group`, `mode` from export data.

**Stage 4: Units**
- Match by `(system_id, unit_id)`. Uses existing `UpsertUnit` logic.
- Tag source priority: `manual > csv > mqtt`.

**Stage 5: Call Groups**
- Match by `(system_id, tgid, start_time)`. Found → skip. Not found → create.

**Stage 6: Calls**
- Dedup by fuzzy match: `(system_id, tgid, start_time ± 5s)`.
- Duplicate → skip. New → insert with resolved IDs.
- Denormalized fields populated from current local data at import time.
- Audio files copied to local audio directory if included.
- Relational tables (`call_frequencies`, `call_transmissions`) rebuilt from JSONB.

**Stage 7: Transcriptions**
- Resolve call by `(system_id, tgid, start_time)` fuzzy match.
- No existing transcription → insert.
- Existing transcription → keep user-reviewed (`source=human`, `status=verified`) over auto. Keep higher confidence if both auto.

**Stage 8: Events (optional)**
- Unit events: append, skip if `(system_id, unit_rid, time, event_type)` already exists.
- Trunking messages: append, skip if `(system_id, opcode, time)` already exists.

### Import Modes

| Mode | Imports | Use Case |
|------|---------|----------|
| `full` | Everything in archive | Migration, backup restore |
| `metadata` | Systems, sites, talkgroups, units | Seed new instance |
| `calls` | Call groups, calls, transcriptions, audio | Add recordings |

### Dry Run

Both API and CLI support `--dry-run`. Returns counts:
```json
{
  "systems": {"create": 0, "update": 1, "skip": 0},
  "talkgroups": {"create": 12, "update": 45, "skip": 93},
  "calls": {"create": 5000, "update": 0, "skip": 7000}
}
```

## Resumability

### Export

Streams directly to tar.gz as it queries. No in-memory buffering. Handles arbitrarily large exports.

### Import: Checkpoint-Based Resume

A `.checkpoint` file tracks progress per-entity-file:

```json
{
  "import_id": "abc123",
  "file": "export-2026-03-02.tar.gz",
  "file_hash": "sha256:...",
  "progress": {
    "systems": {"done": true, "count": 2},
    "sites": {"done": true, "count": 4},
    "talkgroups": {"done": true, "count": 150},
    "calls": {"done": false, "last_line": 8432, "count": 8432}
  }
}
```

On resume: verify archive hash, skip completed files, seek to `last_line + 1` in the in-progress file.

### Batch Processing

Calls imported in batches of 1000 using `COPY FROM` where possible (existing `Batcher` pattern).

### Export Size Estimation

```
GET /api/v1/export/estimate?systems=1&include=metadata,calls,audio
→ {"estimated_size_bytes": 2147483648, "call_count": 45000, "audio_file_count": 44000}
```

## API & CLI Interface

### HTTP Endpoints

**Export:**
```
GET /api/v1/export
  ?systems=1,2           # filter by system (optional, default all)
  &start=2026-02-01      # time range for calls (optional)
  &end=2026-03-01
  &include=metadata,calls,transcriptions,events,audio
  &format=tar.gz
```
Returns `Content-Type: application/gzip` with chunked transfer encoding.

**Export estimate:**
```
GET /api/v1/export/estimate
  ?systems=1,2&include=metadata,calls,audio
```

**Import:**
```
POST /api/v1/import
  Content-Type: multipart/form-data
  file: <tar.gz>
  mode: full|metadata|calls
  dry_run: true|false
```
Returns job_id for large imports, inline JSON summary for small ones.

**Import status/resume:**
```
GET  /api/v1/import/{id}
POST /api/v1/import/{id}/resume
```

All endpoints require write token auth.

### CLI Commands

```bash
# Export
./tr-engine.exe export --output export.tar.gz \
  --systems 1,2 --start 2026-02-01 \
  --include metadata,calls,transcriptions,audio

# Import (dry run)
./tr-engine.exe import --file export.tar.gz --mode full --dry-run

# Import (real)
./tr-engine.exe import --file export.tar.gz --mode metadata

# Resume interrupted import
./tr-engine.exe import --file export.tar.gz --resume

# Manual system mapping for conventional systems
./tr-engine.exe import --file export.tar.gz --system-map "Local Fire=3"
```

CLI connects directly to database (no HTTP server needed).

## Error Handling

- **Corrupt JSONL lines:** Skip with warning, continue import. Summary shows skipped count.
- **Missing references:** If a call references a system not in the export and not in local DB → skip with warning.
- **Conventional system matching:** When `instance_id` differs between source and target, use `--system-map` for explicit mapping.
- **Transcription conflicts:** `source=human`/`status=verified` always wins. Higher confidence wins for auto transcriptions. Ties → keep target's version.
- **Idempotency:** Running the same import twice produces the same result. Everything already imported is skipped.
- **Schema version mismatch:** Manifest version checked before processing. Unknown versions rejected with clear error.

## Post-Import Actions

After import completes:
1. Refresh talkgroup stats (`RefreshTalkgroupStatsHot`, `RefreshTalkgroupStatsCold`)
2. Rebuild search vectors (handled by PostgreSQL triggers on insert)
3. Ensure partitions exist for imported time ranges
4. Clear identity resolution cache (force re-resolution from DB)

## Future: Federated Sync

This export/import format is the foundation for future instance federation. The natural key system and idempotent import logic enable:

- **Incremental exports:** Export only records newer than last sync timestamp
- **Bidirectional sync:** Each instance exports its new data, imports the other's
- **Conflict resolution:** Same merge logic, with an additional "last writer wins with vector clock" for simultaneous edits

This is a future feature — the current design intentionally supports it without requiring it.

## Not In Scope

- Real-time sync/replication (future federation feature)
- Export to third-party formats (rdio-scanner, OpenMHz, etc.)
- Web UI for import/export (API + CLI first)
- Automatic scheduled exports (use cron + CLI)
