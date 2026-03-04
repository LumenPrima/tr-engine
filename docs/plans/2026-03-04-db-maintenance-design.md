# DB Maintenance: Configurable + Visible

**Date:** 2026-03-04
**Status:** Approved

## Problem

The maintenance system (decimation, purging, partition management) runs daily but is invisible and unconfigurable. Users cannot tune retention periods or see what maintenance is doing.

## Solution

Make existing maintenance configurable via env vars and visible via admin API endpoints. No new maintenance logic — just expose what's already there.

## Config (env vars)

| Env Var | Default | Applies To |
|---------|---------|------------|
| `RETENTION_RAW_MESSAGES` | `7d` | `mqtt_raw_messages` weekly partition drops |
| `RETENTION_CONSOLE_LOGS` | `30d` | `console_messages` row purge |
| `RETENTION_PLUGIN_STATUS` | `30d` | `plugin_statuses` row purge |
| `RETENTION_CHECKPOINTS` | `7d` | `call_active_checkpoints` row purge |
| `RETENTION_STALE_CALLS` | `1h` | Incomplete RECORDING calls with no audio |

Parsed as Go `time.Duration` with shorthand support (`7d` → `168h`).

## API Endpoints

### GET /api/v1/admin/maintenance (WRITE_TOKEN required)

Returns current config and last run results:

```json
{
  "config": {
    "retention_raw_messages": "168h0m0s",
    "retention_console_logs": "720h0m0s",
    "retention_plugin_status": "720h0m0s",
    "retention_checkpoints": "168h0m0s",
    "retention_stale_calls": "1h0m0s",
    "schedule": "every 24h"
  },
  "last_run": {
    "started_at": "2026-03-04T02:00:00Z",
    "duration_ms": 1234,
    "results": {
      "partitions_created": 0,
      "partitions_dropped": ["mqtt_raw_messages_w2026_07"],
      "decimation": {
        "recorder_snapshots": {"phase1_deleted": 500, "phase2_deleted": 120},
        "decode_rates": {"phase1_deleted": 300, "phase2_deleted": 80}
      },
      "purged": {
        "console_messages": 42,
        "plugin_statuses": 0,
        "call_active_checkpoints": 15,
        "stale_calls": 3,
        "orphan_call_groups": 1
      }
    }
  }
}
```

### POST /api/v1/admin/maintenance (WRITE_TOKEN required)

Triggers immediate maintenance run. Returns same `last_run` shape. Returns 409 if already running.

## Implementation Changes

| File | Change |
|------|--------|
| `internal/config/config.go` | Add 5 retention duration fields with `d`-suffix parsing |
| `internal/ingest/pipeline.go` | Replace hardcoded durations with config values; store `MaintenanceResult`; expose getter |
| `internal/api/admin.go` | Add `GET`/`POST /admin/maintenance` handlers |
| `internal/api/server.go` | Wire new routes |
| `openapi.yaml` | Document endpoints + schemas |
| `sample.env` | Add retention vars with comments |

## What stays the same

- 24h schedule (hardcoded)
- Decimation thresholds (1w/1mo) — structural, not retention policy
- Partition creation lookahead (3 months/3 weeks)
- All existing maintenance logic
