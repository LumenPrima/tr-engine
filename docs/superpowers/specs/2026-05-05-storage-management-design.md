# Storage Management Feature — Design Spec

**Date:** 2026-05-05  
**Branch:** feat/storage-management (to be created)  
**Status:** Ready for implementation

## Problem

Users accumulate gigabytes of PostgreSQL data without any visibility into what's taking space or an easy way to manage it. `RAW_STORE=true` (the default) archives every raw MQTT message indefinitely until the maintenance scheduler purges them, and there's no UI to see sizes, adjust retention, or trigger a manual purge.

---

## Scope

Three capabilities, delivered as a new dedicated page + admin widget + API surface:

1. **Storage visibility** — per-table row counts and sizes, partition breakdown for large partitioned tables
2. **Retention config UI** — view and edit retention durations via the existing maintenance API; env-var-set values are immutable in the UI
3. **Manual purge** — trigger immediate purge of any purgeable table with a user-specified cutoff

---

## Architecture

### New files

```
internal/database/storage_stats.go    — pg_stat_user_tables + partition size queries
internal/database/config_overrides.go — persistence layer for DB-stored retention overrides
internal/api/storage.go               — StorageHandler: stats + purge only
web/storage.html                      — dedicated admin-only page
```

### Modified files

```
internal/database/migrations.go       — add config_overrides table
internal/config/config.go             — add RetentionTrunkingMessages (env: RETENTION_TRUNKING_MESSAGES, default: 720h)
internal/ingest/pipeline.go           — add TrunkingMessages to retentionConfig + PipelineOptions;
                                        add SetRetention(key, d) for live reload;
                                        add trunking_messages purge step to maintenanceLoop;
                                        update MaintenanceStatus() to include new field + source/locked info
internal/api/live_data.go             — extend MaintenanceConfigData: add RetentionTrunkingMessages;
                                        add source/locked fields per retention setting
internal/api/admin.go                 — add PUT /admin/maintenance/config and DELETE /admin/maintenance/config/{key}
internal/api/server.go                — register StorageHandler routes
  openapi.yaml                           — document new storage stats, purge, and maintenance config endpoints
  web/admin.html                         — add storage summary widget
  sample.env                            — add RETENTION_TRUNKING_MESSAGES example (default 720h)
  internal/api/debug_report.go          — include RetentionTrunkingMessages + source/locked in maintenance section
  docs/AGENTS.md                        — update trunking_messages from "Permanent" to "Configurable retention (default 720h)"
  ```

---

## API

All endpoints require admin role.

### Storage Stats (new)

```
GET /api/v1/admin/storage/stats
```

Response:
```json
{
  "total_db_bytes": 8589934592,
  "tables": [
    {
      "name": "mqtt_raw_messages",
      "row_count": 4200000,
      "table_bytes": 5368709120,
      "index_bytes": 536870912,
      "total_bytes": 5905580032,
      "partitions": [
        { "name": "mqtt_raw_messages_2026_w14", "total_bytes": 2684354560 }
      ]
    }
  ],
  "errors": []
}
```

`partitions` only appears for tables with child partitions. If a per-table stat fails, the table is omitted and an entry added to `errors` — rest of response still returns.

### Manual Purge (new)

```
POST /api/v1/admin/storage/purge/{table}
```

Server-side allowlist (maps purge key → table, time column, purge method):

| Purge key | Table | Time column | Method |
|---|---|---|---|
| `mqtt_raw_messages` | `mqtt_raw_messages` | weekly partitions | `DropOldWeeklyPartitions` |
| `console_messages` | `console_messages` | `log_time` | `PurgeOlderThan` |
| `trunking_messages` | `trunking_messages` | `time` | `PurgeOlderThan` |
| `plugin_statuses` | `plugin_statuses` | `time` | `PurgeOlderThan` |
| `call_active_checkpoints` | `call_active_checkpoints` | `snapshot_time` | `PurgeOlderThan` |

`unit_events` is NOT purgeable — it remains permanent per the data retention policy.

Body: `{ "older_than": "48h" }`

Response:
```json
{
  "table": "mqtt_raw_messages",
  "rows_deleted": 1200000,
  "partitions_dropped": 2,
  "duration_ms": 4823,
  "timed_out": false,
  "warning": "This action is irreversible."
}
```

Purge runs with a 5-minute context deadline. If it times out, `timed_out: true` and `rows_deleted` reflects partial count.

### Retention Config (extended from existing maintenance API)

```
GET    /api/v1/admin/maintenance          — existing; response extended (see below)
PUT    /api/v1/admin/maintenance/config   — new; update one retention setting
DELETE /api/v1/admin/maintenance/config/{key}  — new; remove DB override, fall back to env/default
```

Extended `MaintenanceConfigData` — adds `RetentionTrunkingMessages` and per-field source/locked:

```json
{
  "config": {
    "retention_raw_messages":         "168h",
    "retention_raw_messages_source":  "env",
    "retention_raw_messages_locked":  true,
    "retention_console_logs":         "48h",
    "retention_console_logs_source":  "db",
    "retention_console_logs_locked":  false,
    "retention_trunking_messages":    "720h",
    "retention_trunking_messages_source": "default",
    "retention_trunking_messages_locked": false,
    "retention_plugin_status":        "720h",
    "retention_plugin_status_source": "default",
    "retention_plugin_status_locked": false,
    "retention_checkpoints":          "168h",
    "retention_checkpoints_source":   "env",
    "retention_checkpoints_locked":   true,
    "retention_stale_calls":          "1h",
    "retention_stale_calls_source":   "default",
    "retention_stale_calls_locked":   false,
    "schedule": "every 24h"
  },
  "last_run": { ... }
}
```

Adding fields is backward-compatible; existing consumers that only read the existing string fields are unaffected.

PUT body: `{ "key": "retention_console_logs", "value": "48h" }`  
Returns 409 if `locked: true`. Value must parse via `time.ParseDuration`.  
DELETE returns 409 if `locked: true`.

On PUT/DELETE, the handler:
1. Writes to / removes from `config_overrides` table
2. Calls `pipeline.SetRetention(key, d)` to update the live `retentionConfig` immediately

---

## Data Layer

### `config_overrides` table (new migration, internal persistence only)

```sql
CREATE TABLE config_overrides (
    key        text PRIMARY KEY,
    value      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
```

### Retention resolution (priority order)

1. `os.Getenv("RETENTION_*") != ""` → `source: "env"`, `locked: true`
2. Row in `config_overrides` → `source: "db"`
3. Neither → `source: "default"`, coded default from `config.go`

On startup, the pipeline loads `config_overrides` after env var initialization — DB values take precedence over defaults, env vars take precedence over DB. `SetRetention` updates both `config_overrides` and the live `retentionConfig` atomically.

### Storage stats queries

```sql
-- Per-table sizes (base user tables in public schema only)
SELECT s.relname,
       s.n_live_tup,
       pg_table_size(c.oid)          AS table_bytes,
       pg_indexes_size(c.oid)        AS index_bytes,
       pg_total_relation_size(c.oid) AS total_bytes
FROM pg_stat_user_tables s
JOIN pg_class c ON c.oid = s.relid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r'
  AND n.nspname = 'public'
ORDER BY pg_total_relation_size(c.oid) DESC;

-- Total DB size
SELECT pg_database_size(current_database());

-- Partition breakdown for a given parent table
SELECT c.relname,
       pg_total_relation_size(c.oid) AS total_bytes
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = (
    SELECT oid FROM pg_class
    WHERE relname = $1
      AND relnamespace = 'public'::regnamespace
)
ORDER BY c.relname;
```

---

## UI

### Admin widget (added to `admin.html`)

- Card showing total DB size + top 3 tables by size
- Visible to admin role only (guarded by `me.role === 'admin'` check)
- Loads on page load, links to `storage.html`

### `storage.html` (new built-in page)

Must include `<meta name="card-title" content="Storage">` per web-frontend page registration convention.

Admin-only: on load, fetches `/api/v1/users/me` and redirects to `/` if `me.role !== 'admin'` (same pattern as `admin.html`).

**Storage Overview panel**  
Calls `GET /admin/storage/stats`.  
- Table: name, rows, table size, index size, total — sorted by total descending, re-sortable by column header
- Partitioned tables have an expand chevron revealing per-partition breakdown
- Refresh button + last-fetched timestamp

**Retention Config panel**  
Calls `GET /admin/maintenance`; writes via `PUT /admin/maintenance/config` and `DELETE /admin/maintenance/config/{key}`.  
- One row per retention setting (6 settings)
- Locked (env): value displayed, input disabled, 🔒 "set in .env" badge
- DB/default: editable duration input, Save button per row
- DB-overridden rows have a "Reset to default" link (calls DELETE)
- Inline success/error feedback per row (no full-page reload)

**Manual Purge panel**  
Calls `POST /admin/storage/purge/{table}`.  
- One row per purgeable table (5 entries)
- "Older than" duration input pre-filled from that table's active retention value
- Purge button (destructive red) → confirmation modal: *"This will permanently delete rows from `{table}` older than `{duration}`. This cannot be undone."*
- After purge: shows rows deleted, partitions dropped, time taken inline
- Each panel fails independently

---

## Error Handling

| Scenario | Behavior |
|---|---|
| Stats query fails for one table | Omit table, add to `errors[]`, return rest |
| Purge on unknown/unlisted table | 400 `"unknown table"` |
| Invalid duration format | 400 `"expected duration like '48h' or '7d'"` |
| Edit/delete env-locked key | 409 `"key is locked by environment variable"` |
| Purge timeout (>5 min) | Partial count returned, `timed_out: true` |
| UI panel load failure | Inline error banner per panel, others unaffected |

---

## AGENTS.md Change

Update the data retention table:

| Before | After |
|---|---|
| `trunking_messages` — Permanent / Forever (partitioned) | `trunking_messages` — Configurable retention, default 720h/30d (partitioned) |

`unit_events` remains Permanent.

---

## Out of Scope

- Retention config for `unit_events` (remains permanent)
- Non-retention config settings in the UI
- Writing back to `.env`
- VACUUM / ANALYZE triggers (PostgreSQL autovacuum handles this)
- Disk usage outside of PostgreSQL (audio files, etc.)
