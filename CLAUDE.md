# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

tr-engine is a backend service that ingests MQTT messages from one or more [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances and serves them via a REST API. It handles radio system monitoring data: calls, talkgroups, units, transcriptions, and recorder state.

Target scale: full Ohio MARCS statewide P25 system (~180+ sites, ~15K talkgroups, ~10K-20K MQTT msgs/sec peak) on a single moderate server. Current dev/test uses 2 counties (Butler/Warren, NACs 340/34D).

## Technology Stack

- **Language**: Go
- **Database**: PostgreSQL 17+
- **MQTT**: ingests from trunk-recorder instances
- **Real-time push**: Server-Sent Events (SSE) at `GET /api/v1/events/stream` with server-side filtering (systems, sites, tgids, units, event types). Clients reconnect with `Last-Event-ID` for gapless recovery on filter changes.
- **API**: REST under `/api/v1`, defined in `openapi.yaml`

Go was chosen over Node.js for multi-core utilization and headroom at high message rates (Node single-thread saturates at ~60-70% capacity at MARCS scale; Go sits at ~5-10%).

## Key Files

- `openapi.yaml` — Complete REST API specification (OpenAPI 3.0.3), including SSE event stream endpoint. This is the **source of truth** for API contracts.
- `schema.sql` — PostgreSQL 17 DDL. All tables, indexes, triggers, partitioning, and helper functions. Run with `psql -f schema.sql`.
- `cmd/tr-engine/main.go` — Entry point. Startup order: config → logger → database → MQTT → HTTP server. Graceful shutdown via SIGINT/SIGTERM with 10s timeout. Version injected via `-ldflags`.
- `internal/config/config.go` — Env-based config (`DATABASE_URL`, `MQTT_BROKER_URL`, `HTTP_ADDR`, `AUTH_TOKEN`, `LOG_LEVEL`, timeouts). Uses `caarlos0/env/v11`.
- `internal/database/database.go` — pgxpool wrapper. 20 max / 4 min conns, 2s health-check ping.
- `internal/mqttclient/client.go` — Paho MQTT client. Auto-reconnect (5s), QoS 0, `atomic.Bool` connection tracking. Stub message handler logs topic + payload size.
- `internal/api/server.go` — Chi router + HTTP server lifecycle. Health route outside auth group; future routes inside auth group.
- `internal/api/middleware.go` — RequestID, structured request Logger (zerolog/hlog), Recoverer (JSON 500), BearerAuth (passthrough when `AUTH_TOKEN` empty).
- `internal/api/health.go` — `GET /api/v1/health`. Checks DB (ping) and MQTT (connected). Returns `healthy`/`degraded`/`unhealthy` per OpenAPI `HealthResponse` schema.

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
- **Call groups** deduplicate recordings: `(system_id, tgid, start_time)` groups duplicate recordings from multiple sites.
- **State tables** (`recorder_snapshots`, `decode_rates`) are append-only with decimation (1/min after 24h, 1/hour after 7d). Latest state = `ORDER BY time DESC LIMIT 1`.
- **Audio on filesystem**, not in DB. `calls.audio_file_path` stores relative path.

### Retention Policy

| Category | Tables | Retention |
|----------|--------|-----------|
| Permanent | calls, unit_events, call_frequencies, call_transmissions, transcriptions, talkgroups, units, trunking_messages | Forever (partitioned) |
| Decimated state | recorder_snapshots, decode_rates | Full 24h → 1/min 7d → 1/hour |
| Crash recovery | call_active_checkpoints | 24 hours |
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

# Run (requires PostgreSQL + MQTT broker)
DATABASE_URL="postgres://user:pass@localhost:5432/trengine?sslmode=disable" \
MQTT_BROKER_URL="tcp://localhost:1883" \
LOG_LEVEL="debug" \
./tr-engine.exe

# Test health
curl http://localhost:8080/api/v1/health
```

## Implementation Status

**Completed:**
- `openapi.yaml` — full REST API spec with SSE event stream endpoint
- `schema.sql` — full PostgreSQL 17 DDL (20 tables, partitioning, triggers, helpers)
- Project scaffolding — Go module, config, DB pool, MQTT client, HTTP server with middleware, `GET /api/v1/health`, graceful shutdown

**Next: MQTT ingestion pipeline**
- Parse incoming trunk-recorder MQTT messages (calls, talkgroups, units, recorders, etc.)
- Identity resolution: system/site matching from MQTT fields
- Database writes: insert/upsert into calls, talkgroups, units, unit_events, recorder_snapshots, etc.
- Raw MQTT archival to `mqtt_raw_messages`

## Real-Time Event Streaming (SSE)

`GET /api/v1/events/stream` pushes filtered events to clients over SSE.

- Filter params (all optional, AND-ed): `systems`, `sites`, `tgids`, `units`, `types`, `emergency_only`
- 8 event types: `call_start`, `call_update`, `call_end`, `unit_event`, `recorder_update`, `rate_update`, `trunking_message`, `console`
- `Last-Event-ID` header for gapless reconnect (60s server-side buffer)
- 15s keepalive comments
- Server sends `X-Accel-Buffering: no` header for nginx compatibility
- To change filters: disconnect and reconnect with new query params
