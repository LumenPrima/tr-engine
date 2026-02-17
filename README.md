# tr-engine

Backend service that ingests MQTT messages from [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances and serves them via a REST API. Handles radio system monitoring data: calls, talkgroups, units, and recorder state.

Zero configuration for radio systems — tr-engine discovers systems, sites, talkgroups, and units automatically from the MQTT feed. Point it at a broker, give it a database, and it figures out the rest.

> **Note:** This is a ground-up rewrite of the original tr-engine, now archived at [LumenPrima/tr-engine-v0](https://github.com/LumenPrima/tr-engine-v0). The database schema is not compatible — there is no migration path from v0. If you're coming from v0, see the **[migration guide](docs/migrating-from-v0.md)**. If you're starting fresh, you're in the right place.

> **Warning:** This project is almost entirely AI-written (Claude Code pair programming). It works, but it may also eat your computer or your pets. Specifically:
> - **Do not expose directly to the internet.** There is no meaningful authentication and the security posture has not been audited. If you need external access, put it behind a reverse proxy (Caddy, nginx, etc.) with its own auth layer.
> - **Denial-of-service vectors exist.** The ad-hoc SQL query endpoint, SSE streaming, and unbounded list endpoints could all be abused. This is a monitoring tool for your LAN, not a public service.
> - **Installation instructions have not been thoroughly vetted** and may cause random fires. Test in a disposable environment first.

## Tech Stack

- **Go** — multi-core utilization at high message rates
- **PostgreSQL 17+** — partitioned tables, JSONB, denormalized for read performance
- **MQTT** — ingests from trunk-recorder via the [MQTT Status plugin](https://github.com/TrunkRecorder/trunk-recorder-mqtt-status)
- **REST API** — 36 endpoints under `/api/v1`, defined in `openapi.yaml`
- **SSE** — real-time event streaming with server-side filtering
- **Web UI** — built-in dashboards demonstrating API and SSE capabilities

## Getting Started

- **[Docker Compose](docs/docker.md)** — single `docker compose up` with PostgreSQL, MQTT, and tr-engine
- **[Docker with existing MQTT](docs/docker-external-mqtt.md)** — Docker Compose connecting to a broker you already run
- **[Build from source](docs/getting-started.md)** — roll your own: compile from source, bring your own PostgreSQL and MQTT
- **[Binary releases](docs/binary-releases.md)** — download a pre-built binary, just add PostgreSQL and MQTT

### Quick Start

```bash
mkdir tr-engine && cd tr-engine
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/docker-compose.yml
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/schema.sql
mkdir -p docker && curl -so docker/mosquitto.conf https://raw.githubusercontent.com/LumenPrima/tr-engine/master/docker/mosquitto.conf
docker compose up -d
curl http://localhost:8080/api/v1/health
```

## Configuration

Configuration is loaded in priority order: **CLI flags > environment variables > .env file > defaults**.

The `.env` file is auto-loaded from the current directory on startup. See `sample.env` for all available fields.

### CLI Flags

```
--listen        HTTP listen address (default :8080)
--log-level     debug, info, warn, error (default info)
--database-url  PostgreSQL connection URL
--mqtt-url      MQTT broker URL
--audio-dir     Audio file directory (default ./audio)
--env-file      Path to .env file (default .env)
--version       Print version and exit
```

### Key Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | | PostgreSQL connection string |
| `MQTT_BROKER_URL` | Yes | | MQTT broker URL (e.g., `tcp://localhost:1883`) |
| `MQTT_TOPICS` | No | `#` | MQTT topic filter (match your TR plugin prefix with `/#`) |
| `HTTP_ADDR` | No | `:8080` | HTTP listen address |
| `AUTH_TOKEN` | No | | Bearer token for API auth (disabled if empty) |
| `AUDIO_DIR` | No | `./audio` | Audio file storage directory |
| `TR_AUDIO_DIR` | No | | Serve audio from trunk-recorder's filesystem (see below) |
| `LOG_LEVEL` | No | `info` | Log level |

See `sample.env` for the full list including MQTT credentials, HTTP timeouts, and raw archival settings.

### Audio Modes

tr-engine supports two modes for call audio:

- **MQTT audio (default):** trunk-recorder sends base64-encoded audio in MQTT messages. tr-engine decodes and saves the files to `AUDIO_DIR`. Enable this by setting `mqtt_audio: true` in trunk-recorder's MQTT plugin config.

- **Filesystem audio (`TR_AUDIO_DIR`):** trunk-recorder saves audio files to its local filesystem. tr-engine serves them directly using the `call_filename` path stored at call_end. Set `TR_AUDIO_DIR` to the directory where trunk-recorder writes audio (its `audioBaseDir`). This avoids the overhead of base64 encoding/decoding over MQTT and eliminates duplicate files. When using this mode, you can disable `mqtt_audio` in trunk-recorder's plugin config — tr-engine still receives the audio metadata message for frequency and transmission data.

Both modes can coexist during a transition. Existing calls with MQTT-ingested audio continue to serve from `AUDIO_DIR`; new calls resolve from `TR_AUDIO_DIR`.

## How It Works

### Auto-Discovery

tr-engine builds its model of the radio world entirely from MQTT messages. When trunk-recorder publishes system info, call events, and unit activity, tr-engine:

1. **Identifies systems** by matching P25 `(sysid, wacn)` pairs or conventional `(instance_id, sys_name)`
2. **Discovers sites** within each system — multiple TR instances monitoring the same P25 network auto-merge into one system with separate sites
3. **Tracks talkgroups and units** as they appear in call and unit events

```
System "MARCS" (P25 sysid=348, wacn=BEE00)
  |- Site "butco"  (nac=340, instance=tr-1)
  |- Site "warco"  (nac=34D, instance=tr-2)
  |- Talkgroups (shared across all sites)
  +- Units (shared across all sites)
```

### Data Flow

```
trunk-recorder  ──MQTT──>  broker  ──MQTT──>  tr-engine  ──REST/SSE──>  clients
                                                  |
                                                  v
                                              PostgreSQL
```

MQTT messages are routed to specialized handlers (calls, units, recorders, rates, trunking messages, etc.) that write to PostgreSQL and publish events to the SSE bus.

## Real-Time Event Streaming

`GET /api/v1/events/stream` pushes filtered events over SSE.

- **Filter params** (all optional, AND-ed): `systems`, `sites`, `tgids`, `units`, `types`, `emergency_only`
- **8 event types**: `call_start`, `call_update`, `call_end`, `unit_event`, `recorder_update`, `rate_update`, `trunking_message`, `console`
- **Compound type syntax**: `types=unit_event:call` filters by subtype
- **Reconnect**: `Last-Event-ID` header for gapless recovery (60s server-side buffer)

## API Endpoints

All under `/api/v1`. See `openapi.yaml` for the full specification.

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Service health + TR instance status |
| `GET /systems` | List radio systems |
| `GET /talkgroups` | List talkgroups (filterable) |
| `GET /units` | List radio units |
| `GET /calls` | List call recordings (paginated, filterable) |
| `GET /calls/active` | Currently in-progress calls |
| `GET /calls/{id}/audio` | Stream call audio |
| `GET /unit-events` | Unit event queries (DB-backed) |
| `GET /unit-affiliations` | Live talkgroup affiliation state (in-memory) |
| `GET /call-groups` | Deduplicated call groups across sites |
| `GET /recorders` | Recorder hardware state |
| `GET /events/stream` | Real-time SSE event stream |
| `GET /stats` | System statistics |
| `POST /query` | Ad-hoc read-only SQL queries |

## Web UI

tr-engine ships with several built-in dashboards at `http://localhost:8080`. The index page auto-discovers all pages and links to them.

| Page | Description |
|------|-------------|
| **Event Horizon** | Logarithmic timeline — events drift from now into the past |
| **Live Events** | Real-time SSE event stream with type filtering |
| **Unit Tracker** | Live unit status grid with state colors and group filters |
| **IRC Radio Live** | IRC-style monitor — talkgroups as channels, units as nicks, audio playback |
| **Signal Flow** | Stream graph of talkgroup activity over time (D3.js) |
| **API Docs** | Interactive Swagger UI for the REST API |

Pages are plain HTML with no build step. Add new pages by dropping an `.html` file in `web/` with a `<meta name="card-title">` tag — see [CLAUDE.md](CLAUDE.md#web-frontend-page-registration) for the spec.

## Storage Estimates

Observed with 2 moderately busy counties and 1 trunk-recorder instance:

| Category | Estimated Annual Usage |
|----------|----------------------|
| Database (permanent tables) | ~22 GB/year |
| Database (state + logs overhead) | ~3 GB steady-state |
| Audio files (M4A) | ~140 GB/year |

High-volume tables (calls, unit_events, trunking_messages) are automatically partitioned by month. Partition maintenance runs on startup and every 24 hours.

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
    events.go                   SSE event stream endpoint
    *.go                        Handler files for each resource
web/irc-radio-live.html         IRC-style live radio monitor
openapi.yaml                    API specification (source of truth)
schema.sql                      PostgreSQL DDL
sample.env                      Configuration template
```

## License

MIT
