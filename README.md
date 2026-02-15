# tr-engine

Backend service that ingests MQTT messages from [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances and serves them via a REST API. Handles radio system monitoring data: calls, talkgroups, units, transcriptions, and recorder state.

## Tech Stack

- **Go** — chosen for multi-core utilization at high message rates
- **PostgreSQL 17+** — partitioned tables, JSONB, denormalized for read performance
- **MQTT** — ingests from trunk-recorder instances
- **REST API** — 32 endpoints under `/api/v1`, defined in `openapi.yaml`
- **SSE** — real-time event streaming with server-side filtering

## Prerequisites

- Go 1.23+
- PostgreSQL 17+
- MQTT broker (e.g., Mosquitto)

## Quick Start

```bash
# Set up the database
psql -f schema.sql

# Build
go build -ldflags "-X main.version=0.1.0" -o tr-engine.exe ./cmd/tr-engine

# Configure — copy sample.env to .env and fill in credentials
cp sample.env .env
# edit .env with your DATABASE_URL, MQTT_BROKER_URL, etc.

# Run — .env is auto-loaded
./tr-engine.exe

# Verify
curl http://localhost:8080/api/v1/health
```

## Configuration

Configuration is loaded in priority order: **CLI flags > environment variables > .env file > defaults**.

The `.env` file is auto-loaded from the current directory on startup (silent if missing). See `sample.env` for all fields.

### CLI Flags

```
--listen        HTTP listen address (overrides HTTP_ADDR)
--log-level     Log level: debug, info, warn, error (overrides LOG_LEVEL)
--database-url  PostgreSQL connection URL (overrides DATABASE_URL)
--mqtt-url      MQTT broker URL (overrides MQTT_BROKER_URL)
--audio-dir     Audio file directory (overrides AUDIO_DIR)
--env-file      Path to .env file (default: .env)
--version       Print version and exit
```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | | PostgreSQL connection string |
| `MQTT_BROKER_URL` | Yes | | MQTT broker URL (e.g., `tcp://localhost:1883`) |
| `MQTT_CLIENT_ID` | No | `tr-engine` | MQTT client ID |
| `MQTT_TOPICS` | No | `#` | MQTT topic filter |
| `MQTT_USERNAME` | No | | MQTT credentials |
| `MQTT_PASSWORD` | No | | MQTT credentials |
| `HTTP_ADDR` | No | `:8080` | HTTP listen address |
| `HTTP_READ_TIMEOUT` | No | `5s` | HTTP read timeout |
| `HTTP_WRITE_TIMEOUT` | No | `30s` | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | No | `120s` | HTTP idle timeout |
| `AUTH_TOKEN` | No | | Bearer token for API auth (disabled if empty) |
| `AUDIO_DIR` | No | `./audio` | Audio file storage directory |
| `LOG_LEVEL` | No | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `RAW_STORE` | No | `true` | Master switch to disable all raw MQTT archival |
| `RAW_INCLUDE_TOPICS` | No | | Allowlist of handler names for raw archival (supports `_unknown`; overrides exclude) |
| `RAW_EXCLUDE_TOPICS` | No | | Denylist of handler names to exclude from raw archival |

## Data Model

Two-level system/site hierarchy:

```
System 1 (P25 sysid=348, wacn=BEE00)
  |- Site 1 "butco"  (nac=340, instance=tr-1)
  |- Site 2 "warco"  (nac=34D, instance=tr-2)
  |- Talkgroups (shared across all sites)
  +- Units (shared across all sites)
```

- **System** = logical radio network (P25 or conventional)
- **Site** = recording point within a system
- Multiple trunk-recorder instances monitoring the same P25 network auto-merge into one system with separate sites

## Real-Time Event Streaming

`GET /api/v1/events/stream` pushes filtered events over SSE.

- **Filter params** (all optional, AND-ed): `systems`, `sites`, `tgids`, `units`, `types`, `emergency_only`
- **8 event types**: `call_start`, `call_update`, `call_end`, `unit_event`, `recorder_update`, `rate_update`, `trunking_message`, `console`
- **Compound type syntax**: `types=unit_event:call` filters by subtype
- **Reconnect**: `Last-Event-ID` header for gapless recovery (60s server-side buffer)

## Project Structure

```
cmd/tr-engine/main.go           Entry point with CLI flag parsing
internal/
  config/config.go              .env + env var + CLI config loading
  database/                     PostgreSQL connection pool + query files
  mqttclient/client.go          MQTT client with auto-reconnect
  ingest/
    pipeline.go                 MQTT message dispatch + batchers
    router.go                   Topic-to-handler routing
    identity.go                 System/site identity resolution + caching
    eventbus.go                 SSE pub/sub with ring buffer replay
    handler_*.go                Per-topic message handlers
  api/
    server.go                   Chi router + HTTP server
    middleware.go               RequestID, logging, recovery, auth
    health.go                   Health endpoint with TR instance status
    events.go                   SSE event stream endpoint
    stats.go                    Stats, trunking messages, console logs
    query.go                    Ad-hoc read-only SQL query endpoint
    *.go                        Systems, talkgroups, units, calls, etc.
openapi.yaml                    API specification (source of truth)
schema.sql                      PostgreSQL DDL
sample.env                      Configuration template
```

## API Endpoints

See `openapi.yaml` for full specification. Key endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Service health + TR instance status |
| `GET /systems` | List radio systems |
| `GET /talkgroups` | List talkgroups (filterable) |
| `GET /units` | List radio units |
| `GET /calls` | List call recordings (paginated, filterable) |
| `GET /calls/active` | Currently in-progress calls |
| `GET /calls/{id}/audio` | Stream call audio |
| `GET /call-groups` | Deduplicated call groups |
| `GET /recorders` | Recorder hardware state |
| `GET /trunking-messages` | P25 control channel messages |
| `GET /console-messages` | Trunk-recorder console logs |
| `GET /events/stream` | Real-time SSE event stream |
| `GET /stats` | System statistics |
| `GET /stats/rates` | Decode rate history |
| `POST /query` | Ad-hoc read-only SQL queries |

## License

Private.
