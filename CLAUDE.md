# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

tr-engine is a backend service that ingests MQTT messages from one or more [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances and serves them via a REST API. It handles radio system monitoring data: calls, talkgroups, units, transcriptions, and recorder state.

Current dev/test uses 2 counties (Butler/Warren, NACs 340/34D).

## Technology Stack

- **Language**: Go
- **Database**: PostgreSQL 18 (we have changed spec to v 18, make sure all references are updated)
- **MQTT**: ingests from trunk-recorder instances
- **Real-time push**: Server-Sent Events (SSE) at `GET /api/v1/events/stream` with server-side filtering (systems, sites, tgids, units, event types). Clients reconnect with `Last-Event-ID` for gapless recovery on filter changes.
- **API**: REST under `/api/v1`, defined in `openapi.yaml`

Go was chosen over Node.js for multi-core utilization and headroom at high message rates.

## Key Files

- `openapi.yaml` — Complete REST API specification (OpenAPI 3.0.3), including SSE event stream endpoint. This is the **source of truth** for API contracts.
- `schema.sql` — PostgreSQL 18 DDL. All tables, indexes, triggers, partitioning, and helper functions. Run with `psql -f schema.sql`.
- `.env` — Local environment config (gitignored). Contains `DATABASE_URL`, `MQTT_BROKER_URL`, credentials, and `HTTP_ADDR`.
- `cmd/tr-engine/main.go` — Entry point. Startup order: config → logger → database → MQTT → pipeline → HTTP server. Graceful shutdown via SIGINT/SIGTERM with 10s timeout. Version injected via `-ldflags`.
- `cmd/mqtt-dump/` — Dev tool to capture and display live MQTT traffic.
- `cmd/dbcheck/` — DB inspection tool (table counts, call group analysis, cleanup).
- `internal/config/config.go` — Env-based config (`DATABASE_URL`, `MQTT_BROKER_URL`, `HTTP_ADDR`, `AUTH_TOKEN`, `LOG_LEVEL`, timeouts). Uses `caarlos0/env/v11`.
- `internal/database/` — pgxpool wrapper (20 max / 4 min conns, 2s health-check ping) plus query files for all tables: systems, sites, talkgroups, units, calls, call_groups, recorders, stats, etc.
- `internal/mqttclient/client.go` — Paho MQTT client. Auto-reconnect (5s), QoS 0, `atomic.Bool` connection tracking.
- `internal/ingest/` — Complete MQTT ingestion pipeline. Message routing (`router.go`), identity resolution (`identity.go`), event bus for SSE (`eventbus.go`), batch writers (`batcher.go`), and handlers for all message types (calls, units, recorders, rates, systems, config, audio, status, trunking messages, console logs). Raw archival supports three modes: disabled (`RAW_STORE=false`), allowlist (`RAW_INCLUDE_TOPICS` with `_unknown` for unrecognized topics), or denylist (`RAW_EXCLUDE_TOPICS`). Audio messages have base64 audio data stripped before raw archival since the audio is already saved to disk.
- `internal/api/server.go` — Chi router + HTTP server lifecycle. All 30 endpoints wired via handler `Routes()` methods.
- `internal/api/query.go` — Ad-hoc read-only SQL query handler (`POST /query`). Read-only transaction, 30s statement timeout, row cap, semicolon rejection.
- `internal/database/query.go` — `ExecuteReadOnlyQuery()` — runs SQL in a `BEGIN READ ONLY` transaction with `SET LOCAL statement_timeout = '30s'`.
- `internal/api/middleware.go` — RequestID, structured request Logger (zerolog/hlog), Recoverer (JSON 500), BearerAuth (passthrough when `AUTH_TOKEN` empty).

## Go Dependencies

| Library | Purpose |
|---------|---------|
| `jackc/pgx/v5` | PostgreSQL driver + connection pool (pgxpool) |
| `go-chi/chi/v5` | HTTP router with composable middleware |
| `eclipse/paho.mqtt.golang` | MQTT client with auto-reconnect |
| `caarlos0/env/v11` | Struct-based env config parsing |
| `rs/zerolog` | Zero-allocation structured JSON logging |

## Data Model: Two-Level System/Site Hierarchy

The core concept that permeates the entire codebase:

- **System** = logical radio network. P25 identified by `(sysid, wacn)`. Conventional identified by `(instance_id, sys_name)`. Talkgroups and units belong at the system level.
- **Site** = recording point within a system. One TR `sys_name` per instance. Multiple sites can monitor the same P25 network from different locations.

```
System 1 (P25 sysid=348, wacn=BEE00)
  ├── Site 1 "butco"  (nac=340, instance=tr-1)
  ├── Site 2 "warco"  (nac=34D, instance=tr-2)
  ├── Talkgroups (shared across all sites)
  └── Units (shared across all sites)
```

Conventional systems are 1:1 with sites.

### Identity Resolution (MQTT Ingest)

- **System**: match `(sysid, wacn)` for P25/smartnet; `(instance_id, sys_name)` for conventional
- **Site**: match `(system_id, instance_id, sys_name)` — never use `sys_num` (positional, unstable)
- Two TR instances monitoring the same P25 network auto-merge into one system with separate sites

### ID Formats (API)

- Talkgroup: `{system_id}:{tgid}` (composite) or plain `{tgid}` (409 Conflict if ambiguous)
- Unit: `{system_id}:{unit_id}` (composite) or plain `{unit_id}` (409 if ambiguous)
- Call: plain integer `call_id` (opaque auto-increment)

## Database Design Principles

- **Store everything** — even fields that seem irrelevant now. `metadata_json` JSONB catch-all on calls and unit_events captures unmapped MQTT fields.
- **Denormalize for reads** — `calls` carries `system_name`, `site_short_name`, `tg_alpha_tag`, etc. copied at write time. Avoids JOINs on the hottest query paths.
- **Monthly partitioning** on high-volume tables: `calls`, `call_frequencies`, `call_transmissions`, `unit_events`, `trunking_messages`. Weekly for `mqtt_raw_messages`.
- **Dual-write transmission/frequency data** — `calls.src_list` and `calls.freq_list` JSONB columns for API reads (no JOINs). `call_transmissions` and `call_frequencies` relational tables for ad-hoc SQL queries. `calls.unit_ids` is a denormalized `int[]` with GIN index for fast unit filtering.
- **Call groups** deduplicate recordings: `(system_id, tgid, start_time)` groups duplicate recordings from multiple sites.
- **State tables** (`recorder_snapshots`, `decode_rates`) are append-only with decimation (1/min after 1 week, 1/hour after 1 month). Latest state = `ORDER BY time DESC LIMIT 1`.
- **Audio on filesystem**, not in DB. `calls.audio_file_path` stores relative path.

### Retention Policy

| Category | Tables | Retention |
|----------|--------|-----------|
| Permanent | calls, call_frequencies, call_transmissions, unit_events, transcriptions, talkgroups, units, trunking_messages | Forever (partitioned) |
| Decimated state | recorder_snapshots, decode_rates | Full 1 week → 1/min 1 month → 1/hour |
| Crash recovery | call_active_checkpoints | 7 days |
| Raw archive | mqtt_raw_messages | 7 days |
| Logs | console_messages, plugin_statuses | 30 days |
| Audit | system_merge_log, instance_configs | Forever (low volume) |

## Schema Validation

```bash
psql -f schema.sql  # creates all tables, indexes, triggers, initial partitions
```

The schema creates initial partitions (current month + 3 months ahead). The `create_monthly_partition()` and `create_weekly_partition()` functions handle ongoing partition creation.

## Building & Running

```bash
# Build (inject version at build time)
go build -ldflags "-X main.version=0.1.0" -o tr-engine.exe ./cmd/tr-engine

# Run — auto-loads .env from current directory
./tr-engine.exe

# Override settings via CLI flags
./tr-engine.exe --listen :9090 --log-level debug

# Use a different .env file
./tr-engine.exe --env-file /path/to/production.env

# Print version
./tr-engine.exe --version

# Test health
curl http://localhost:8080/api/v1/health
```

### Configuration

Configuration is loaded in priority order: **CLI flags > environment variables > .env file > defaults**.

The `.env` file is auto-loaded from the current directory on startup (silent if missing). See `sample.env` for all available fields with descriptions.

**CLI flags:**

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--listen` | `HTTP_ADDR` | `:8080` | HTTP listen address |
| `--log-level` | `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `--database-url` | `DATABASE_URL` | _(required)_ | PostgreSQL connection URL |
| `--mqtt-url` | `MQTT_BROKER_URL` | _(required)_ | MQTT broker URL |
| `--audio-dir` | `AUDIO_DIR` | `./audio` | Audio file directory |
| `--env-file` | — | `.env` | Path to .env file |
| `--version` | — | — | Print version and exit |

Additional env-only settings: `MQTT_TOPICS` (comma-separated MQTT topic filters, default `#`; match your TR plugin's `topic`/`unit_topic`/`message_topic` prefixes with `/#` wildcards to limit subscriptions), `MQTT_CLIENT_ID`, `MQTT_USERNAME`, `MQTT_PASSWORD`, `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT`, `AUTH_TOKEN`, `RAW_STORE` (bool, default `true` — master switch to disable all raw MQTT archival), `RAW_INCLUDE_TOPICS` (comma-separated allowlist of handler names for raw archival; supports `_unknown` for unrecognized topics; takes priority over `RAW_EXCLUDE_TOPICS`), `RAW_EXCLUDE_TOPICS` (comma-separated denylist of handler names to exclude from raw archival).

## Development Environment

A live environment is available for testing:

- **PostgreSQL**: Deployed instance with real data from ingest testing. Connection details in `.env`.
- **MQTT broker**: Live production server connected to a real trunk-recorder instance. Credentials in `.env`.
- **Config**: Copy `sample.env` to `.env` and fill in credentials. The `.env` file is gitignored and auto-loaded on startup.

## Implementation Status

**Completed:**
- `openapi.yaml` — full REST API spec with SSE event stream endpoint (32 endpoints)
- `schema.sql` — full PostgreSQL 18 DDL (20 tables, partitioning, triggers, helpers)
- MQTT ingestion pipeline — message routing, identity resolution, batch writes, all handler types (calls, units, recorders, rates, systems, config, audio, status, trunking messages, console logs)
- REST API — all 32 endpoints implemented across 12 handler files (systems, talkgroups, units, calls, call_groups, stats, recorders, events/SSE, unit-events, affiliations, admin, query)
- Database layer — complete CRUD and query builders for all tables
- SSE event bus — real-time pub/sub with ring buffer replay, `Last-Event-ID` support, and event publishing wired into all ingest handlers (call_start, call_end, unit_event, recorder_update, rate_update, trunking_message, console)
- Health endpoint — shows database, MQTT, and trunk-recorder instance status (connected/disconnected with last_seen timestamps)
- Dev tools — `cmd/mqtt-dump` (MQTT traffic inspector), `cmd/dbcheck` (DB analysis)

**Not yet done:**
- Test coverage for new unit-events and affiliations endpoints

## Real-Time Event Streaming (SSE)

`GET /api/v1/events/stream` pushes filtered events to clients over SSE.

- Filter params (all optional, AND-ed): `systems`, `sites`, `tgids`, `units`, `types`, `emergency_only`
- 8 event types: `call_start`, `call_update`, `call_end`, `unit_event`, `recorder_update`, `rate_update`, `trunking_message`, `console`
- `Last-Event-ID` header for gapless reconnect (60s server-side buffer)
- 15s keepalive comments
- Server sends `X-Accel-Buffering: no` header for nginx compatibility
- To change filters: disconnect and reconnect with new query params

### SSE Filtering Details

All filters are AND-ed. Events carry `SystemID`, `SiteID`, `Tgid`, and `UnitID` metadata for server-side filtering. Events with a zero value for a field (e.g., `recorder_update` has `SystemID=0` since recorders are per-instance, not per-system) pass through that filter dimension.

**Compound type syntax:** The `types` param supports `base:subtype` to filter event subtypes. Currently only `unit_event` has subtypes (on, off, call, end, join, location, ackresp, data). Examples:
- `types=unit_event` — all unit events (any subtype)
- `types=unit_event:call` — only unit call events
- `types=unit_event:call,unit_event:end,call_start` — mix compound and plain

Implementation: `EventData` struct in `eventbus.go` carries `Type`, `SubType`, `SystemID`, `SiteID`, `Tgid`, `UnitID`. All ingest handlers publish via `p.PublishEvent(EventData{...})`. The `matchesFilter()` function in `eventbus.go` handles all filter logic including compound type parsing via `strings.Cut`.

### MQTT Topic → Handler Mapping

| MQTT Topic | Handler | SSE Event | DB Table | Volume |
|-----------|---------|-----------|----------|--------|
| `trdash/feeds/call_start` | `handleCallStart` | `call_start` | `calls` | Low |
| `trdash/feeds/call_end` | `handleCallEnd` | `call_end` | `calls` | Low |
| `trdash/units/{sys_name}/{event}` | `handleUnitEvent` | `unit_event` | `unit_events` | Medium |
| `trdash/feeds/recorders` | `handleRecorders` | `recorder_update` | `recorder_snapshots` | Medium |
| `trdash/feeds/rates` | `handleRates` | `rate_update` | `decode_rates` | Low |
| `trdash/messages/{sys_name}/message` | `handleTrunkingMessage` | `trunking_message` | `trunking_messages` | Very high (batched) |
| `trdash/feeds/trunk_recorder/console` | `handleConsoleLog` | `console` | `console_messages` | Low-medium |
| `trdash/feeds/trunk_recorder/status` | `handleStatus` | _(none)_ | `plugin_statuses` | Very low |

Trunking messages use a `Batcher` for CopyFrom batch inserts (same as raw messages and recorder snapshots). Console logs use simple single-row INSERT. The status handler caches TR instance status in-memory for the `/api/v1/health` endpoint rather than publishing SSE events.

## Known trunk-recorder Issues (Potential Upstream Bug Reports)

### unit_event:end lags call_end by 3-4 seconds
`unit_event:end` arrives 3-4s after `call_end` for the same call. `call_end` fires from the recorder when voice frames stop (immediate), but `unit_event:end` fires from the control channel parser when it sees the deaffiliation message (delayed by the P25 trunking update cycle). TR should be able to detect unit transmission end from the recorder side (voice frames going null) rather than waiting for the control channel. The P25 channel stays allocated during hang time, but the actual voice traffic stops immediately. Event Horizon works around this with a 6s coalesce window.

### call ID shifts between call_start and call_end
The trunk-recorder call ID format `{sys_num}_{tgid}_{start_time}` embeds `start_time`, which can shift by 1-2 seconds between `call_start` and `call_end` messages. This causes the call_end handler to fail exact-match lookup. tr-engine works around this with fuzzy matching by `(tgid, start_time ± 5s)` in the active calls map.
