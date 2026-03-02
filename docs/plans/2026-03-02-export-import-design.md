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
| System | P25: `(sysid, wacn)`, Conv: `(instance_id, short_name)` via site | — |
| Site | `(instance_id, short_name)` | system by natural key |
| Talkgroup | `(tgid)` | system by natural key |
| Unit | `(unit_id)` | system by natural key |
| Call Group | `(tgid, start_time)` | system by natural key |
| Call | `(tgid, start_time, instance_id)` | system, site, call_group by natural keys |
| Transcription | `(source, created_at)` anchored to parent call | call by `(system_ref, tgid, start_time ± 5s)` |

### Per-Record Schema Version

Every JSONL record includes a `_v` field indicating the record schema version. This enables future format evolution without requiring the importer to infer schema from the manifest alone. All records in an archive MUST conform to the same version (matching the manifest `version`). Mixed-version archives are explicitly prohibited and rejected on import.

### Example Records

**System (P25):**
```json
{"_v": 1, "sysid": "348", "wacn": "BEE00", "name": "Butler/Warren", "type": "p25"}
```

**System (Conventional):**
Conventional systems are identified via their site's `(instance_id, short_name)`, matching the actual identity resolution in `FindOrCreateSystem` → `FindSystemViaSite`. The system record carries its sites inline for export since the identity lives there:
```json
{"_v": 1, "type": "conventional", "name": "Local Fire", "sites": [{"instance_id": "trunk-recorder", "short_name": "local_fire"}]}
```

**Talkgroup:**
```json
{"_v": 1, "system_ref": {"sysid": "348", "wacn": "BEE00"}, "tgid": 24513, "alpha_tag": "BC Fire Dispatch", "alpha_tag_source": "manual", "tag": "Fire Dispatch", "group": "Butler County", "description": "Butler County Fire/EMS Dispatch", "mode": "D", "priority": 1}
```

**Call:**
```json
{"_v": 1, "system_ref": {"sysid": "348", "wacn": "BEE00"}, "site_ref": {"instance_id": "tr-1", "short_name": "butco"}, "tgid": 24513, "start_time": "2026-03-01T14:30:00Z", "stop_time": "2026-03-01T14:30:45Z", "duration": 45.2, "freq": 851250000, "emergency": false, "encrypted": false, "src_list": [...], "freq_list": [...], "audio_file_path": "audio/2026/03/01/24513-1709300200.wav", "metadata_json": {...}}
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
- P25: match by `(sysid, wacn)` using `FindSystemBySysidWacn`. Found → merge (fill empty fields). Not found → create.
- Conventional: match by `(instance_id, short_name)` via the sites table using `FindSystemViaSite`, mirroring the actual identity resolution path. The system `name` field has no uniqueness constraint and is never used for matching. Found → merge. Not found → create.
- Optional: `--system-map` for manual mapping when `instance_id` values differ between source and target (see CLI section for syntax).
- Result: `system_ref → local system_id` map.

**Stage 2: Sites**
- First check: `(instance_id, short_name)` is globally unique in the DB (constraint `uq_sites_instance_short_name`). Look up by `(instance_id, short_name)` first.
  - If found AND belongs to the resolved `system_id` → update empty fields. Normal case.
  - If found BUT belongs to a *different* `system_id` → **conflict**. This means the same TR instance/site exists locally under a different system (e.g., due to system remapping). Abort with a clear error explaining the conflict, suggesting `--system-map` to resolve. Do NOT silently create or reassign.
  - If not found → create under the resolved `system_id`.
- Result: `site_ref → local site_id` map.

**Stage 3: Talkgroups**
- Match by `(system_id, tgid)`.
- **Does NOT use existing `UpsertTalkgroup`** — that function doesn't write `alpha_tag_source` on conflict and implicitly treats all incoming tags as mqtt-priority. A dedicated import upsert is required (`ImportUpsertTalkgroup`) that:
  1. Reads the existing row's `alpha_tag_source` (if any).
  2. Compares source priority: `manual > csv > mqtt > directory`.
  3. If the imported source outranks or equals the existing source → update `alpha_tag` AND `alpha_tag_source`.
  4. If the existing source outranks the imported source → keep existing `alpha_tag` unchanged.
  5. Always fill empty `description`, `tag`, `group`, `mode` from export data regardless of source priority (enrichment).
- This ensures a `manual`-tagged talkgroup from instance A correctly overwrites a `csv`-tagged one on instance B, and vice versa that a `csv` import doesn't overwrite a `manual` tag.

**Stage 4: Units**
- Match by `(system_id, unit_id)`.
- Same approach as talkgroups: a dedicated `ImportUpsertUnit` that correctly compares `alpha_tag_source` priority (`manual > csv > mqtt`) and updates both `alpha_tag` and `alpha_tag_source` when the imported source outranks the existing one. Enriches empty fields regardless of source priority.

**Stage 5+6: Call Groups and Calls (merged — call groups are created on demand)**

Call groups and calls are processed together because their dedup logic is coupled. The call group unique constraint is exact `(system_id, tgid, start_time)` but call dedup uses fuzzy `±5s` matching. Processing them separately would create orphan call groups for calls that are correctly skipped as duplicates.

For each call in the export:
1. **Dedup the call** by fuzzy match: `(system_id, tgid, start_time ± 5s)`.
   - Duplicate found → skip the call. Reuse the existing call's `call_group_id` (no new call group needed).
   - Not a duplicate → continue to step 2.
2. **Find or create the call group** using fuzzy match `(system_id, tgid, start_time ± 5s)` — same window as calls.
   - If an existing call group matches → reuse its ID.
   - If no match → create a new call group.
3. **Insert the call** with resolved system_id, site_id, and call_group_id.
4. Denormalized fields populated from current local data at import time.
5. Audio files copied to local audio directory if included.
6. Relational tables (`call_frequencies`, `call_transmissions`) rebuilt from `freq_list`/`src_list` JSONB.

**Stage 7: Transcriptions**
- Resolve call by `(system_id, tgid, start_time ± 5s)` fuzzy match to find the local `call_id`.
- If call not found locally → skip with warning.
- If call found, dedup by `(resolved_call_id, source, created_at)`:
  - No existing transcription for this call → insert.
  - Existing transcription: `source='human'` always wins over `source='auto'`/`source='llm'`. For two `source='auto'` transcriptions, keep higher `confidence`. Ties → keep target's version.
- After insert, update `calls.transcription_status` and `calls.has_transcription` to reflect the new state (mirroring `InsertTranscription` logic that derives status from source).

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

Different entity types use different batch strategies:

- **Talkgroups, units** (exact PK dedup): can use `COPY FROM` with `ON CONFLICT` or batch `INSERT ... ON CONFLICT` since dedup is by exact composite key.
- **Calls** (fuzzy dedup): processed in chunks of 1000. Each chunk pre-fetches candidate duplicates for the chunk's time range (`start_time ± 5s` window), resolves the non-duplicate set locally, then bulk-inserts the survivors via batch `INSERT`. `COPY FROM` is NOT used for calls because it doesn't support the per-row fuzzy dedup check.
- **Events** (unit_events, trunking_messages): can use `COPY FROM` via the existing `Batcher` pattern since they're append-only with exact-match skip logic.

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

**Body size handling:** The import endpoint needs a dedicated `MaxBodySize` exemption (similar to the upload endpoint's 50MB limit, but much larger). Two options:
- **Streaming upload:** The handler reads the tar.gz as a stream directly from the multipart body without buffering the full file in memory. The `MaxBodySize` middleware is bypassed for this endpoint, and the handler enforces its own limit (configurable `IMPORT_MAX_SIZE`, default 10GB). The archive is either streamed directly or spooled to a temp file.
- **Server-side path (CLI preference):** For very large imports, the CLI `import` command reads directly from a local file path, bypassing HTTP entirely. The HTTP endpoint is practical for metadata-only and moderate-sized imports.

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
# Maps exported site identity → target site identity (natural keys, no DB IDs)
./tr-engine.exe import --file export.tar.gz \
  --system-map "trunk-recorder:local_fire=target-instance:target_sys"
# For P25 systems (rarely needed since sysid/wacn match automatically)
./tr-engine.exe import --file export.tar.gz \
  --system-map "p25:348:BEE00=p25:348:BEE00"
```

CLI connects directly to database (no HTTP server needed).

## Error Handling

- **Corrupt JSONL lines:** Skip with warning, continue import. Summary shows skipped count.
- **Missing references:** If a call references a system not in the export and not in local DB → skip with warning.
- **Conventional system matching:** When `instance_id` differs between source and target, use `--system-map` with natural key syntax (`instance_id:short_name=target_instance_id:target_short_name`) for explicit mapping. No DB integer IDs.
- **Site constraint conflicts:** If an imported site's `(instance_id, short_name)` already exists locally under a different system, abort with a clear error rather than silently creating or reassigning. Suggest `--system-map` to resolve.
- **Transcription conflicts:** `source='human'` always wins over `source='auto'`/`source='llm'`. Higher `confidence` wins for two auto transcriptions. Ties → keep target's version. The `transcriptions` table has no `status` column — status is derived and stored on `calls.transcription_status`.
- **Idempotency:** Running the same import twice produces the same result. Everything already imported is skipped.
- **Schema version mismatch:** Manifest version checked before processing. Unknown versions rejected with clear error. Per-record `_v` field validated against manifest version — mixed-version archives are rejected. Records missing `_v` are rejected.

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
