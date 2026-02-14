# tr-engine

Backend service that ingests MQTT messages from [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances and serves them via a REST API. Handles radio system monitoring data: calls, talkgroups, units, transcriptions, and recorder state.

Target scale: full Ohio MARCS statewide P25 system (~180+ sites, ~15K talkgroups, ~10K-20K MQTT msgs/sec peak) on a single moderate server.

## Tech Stack

- **Go** — chosen for multi-core utilization at high message rates
- **PostgreSQL 17+** — partitioned tables, JSONB, denormalized for read performance
- **MQTT** — ingests from trunk-recorder instances
- **REST API** — under `/api/v1`, defined in `openapi.yaml`
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

# Run
DATABASE_URL="postgres://user:pass@localhost:5432/trengine?sslmode=disable" \
MQTT_BROKER_URL="tcp://localhost:1883" \
LOG_LEVEL="debug" \
./tr-engine.exe

# Verify
curl http://localhost:8080/api/v1/health
```

## Configuration

All configuration is via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | | PostgreSQL connection string |
| `MQTT_BROKER_URL` | Yes | | MQTT broker URL (e.g., `tcp://localhost:1883`) |
| `HTTP_ADDR` | No | `:8080` | HTTP listen address |
| `AUTH_TOKEN` | No | | Bearer token for API auth (disabled if empty) |
| `LOG_LEVEL` | No | `info` | Log level (`debug`, `info`, `warn`, `error`) |

## Data Model

Two-level system/site hierarchy:

```
System 1 (P25 sysid=348, wacn=BEE00)
  ├── Site 1 "butco"  (nac=340, instance=tr-1)
  ├── Site 2 "warco"  (nac=34D, instance=tr-2)
  ├── Talkgroups (shared across all sites)
  └── Units (shared across all sites)
```

- **System** = logical radio network (P25 or conventional)
- **Site** = recording point within a system
- Multiple trunk-recorder instances monitoring the same P25 network auto-merge into one system with separate sites

## Project Structure

```
cmd/tr-engine/main.go       Entry point, startup/shutdown orchestration
internal/
  config/config.go           Env-based configuration
  database/database.go       PostgreSQL connection pool
  mqttclient/client.go       MQTT client with auto-reconnect
  api/
    server.go                Chi router + HTTP server
    middleware.go            RequestID, logging, recovery, auth
    health.go                GET /api/v1/health
openapi.yaml                 API specification (source of truth)
schema.sql                   PostgreSQL DDL
```

## Status

**Done:** API spec, database schema, service scaffolding (config, DB pool, MQTT client, HTTP server, middleware, health endpoint, graceful shutdown)

**Next:** MQTT ingestion pipeline — message parsing, system/site identity resolution, database writes, raw message archival
