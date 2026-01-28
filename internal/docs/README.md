# tr-engine

Backend service for aggregating [trunk-recorder](https://github.com/robotastic/trunk-recorder) data via MQTT. Works with [taclane's trunk-recorder MQTT plugin](https://github.com/taclane/trunk-recorder-mqtt-status).

**Designed as a backend for frontend developers** - build your own UI using any platform/language with the REST and WebSocket APIs. Included web dashboards are functional demos, not feature-complete applications.

## Highlights

- **Self-contained deployment** - Single binary with embedded PostgreSQL and MQTT broker. No external dependencies.
- **Data ingestion** - Receives calls, audio, unit events, recorder status via MQTT. Stores audio with per-unit transmission metadata.
- **Cross-site deduplication** - Links duplicate P25 call recordings from multi-site systems.
- **REST API** - Historical queries for calls, talkgroups, units, statistics.
- **WebSocket API** - Real-time event streaming with subscription filtering.
- **Interactive docs** - Swagger UI at `/swagger/`, WebSocket tester at `/websocket`.
- **Demo web UI** - Dashboard with call history, unit tracking, audio playback. Recorder status monitor.
- **Prometheus metrics** - System health and decode rate monitoring at `/metrics`.

## Quick Start

### Option 1: Docker Compose (Easiest)

```bash
cd docker
docker-compose up -d
```

### Option 2: Self-Contained Binary

**1. Build or download the binary:**
```bash
go build -o tr-engine
```

**2. Generate config (first run):**
```bash
./tr-engine
```
Creates `config.yaml` with embedded mode defaults, then exits.

**3. Start the service:**
```bash
./tr-engine
```
Starts embedded PostgreSQL (port 5432), MQTT broker (port 1883), and HTTP server (port 8080).

**4. Configure trunk-recorder:**

Install the [MQTT status plugin](https://github.com/taclane/trunk-recorder-mqtt-status) and point it to tr-engine:

```json
{
  "mqttBrokerAddress": "tcp://<tr-engine-host>:1883"
}
```

**5. Open the dashboard:**

Navigate to `http://<tr-engine-host>:8080/`

| URL | Description |
|-----|-------------|
| `/` | Landing page |
| `/dashboard` | Real-time dashboard |
| `/recorders` | Recorder status monitor |
| `/swagger/` | REST API docs |
| `/websocket` | WebSocket API docs + live tester |
| `/metrics` | Prometheus metrics |

---

## Deployment Options

### Self-Contained (Default)

Single binary, zero dependencies. Data stored in `./data/`.

```yaml
database:
  embedded: true
  embedded_data_path: "./data/postgres"

mqtt:
  embedded: true
  embedded_port: 1883

storage:
  audio_path: "./data/audio"
```

### External Services (Production)

For high-volume deployments with TimescaleDB.

```yaml
database:
  host: "localhost"
  port: 5432
  name: "tr_engine"
  user: "tr_engine"
  password: "your-password"

mqtt:
  broker: "tcp://mosquitto:1883"

storage:
  audio_path: "/var/lib/tr-engine/audio"
```

---

## Configuration Reference

### Database

| Setting | Description | Default |
|---------|-------------|---------|
| `database.embedded` | Use embedded PostgreSQL | `false` |
| `database.embedded_data_path` | Embedded data directory | `./data/postgres` |
| `database.host` | PostgreSQL host | `localhost` |
| `database.port` | PostgreSQL port | `5432` |
| `database.name` | Database name | `tr_engine` |
| `database.user` | Database user | `tr_engine` |
| `database.password` | Database password | |

### MQTT

| Setting | Description | Default |
|---------|-------------|---------|
| `mqtt.embedded` | Use embedded MQTT broker | `false` |
| `mqtt.embedded_port` | Embedded broker port | `1883` |
| `mqtt.broker` | External broker URL | `tcp://localhost:1883` |
| `mqtt.topics.status` | Status topic pattern | `feeds/#` |
| `mqtt.topics.units` | Units topic pattern | `units/#` |

### Server

| Setting | Description | Default |
|---------|-------------|---------|
| `server.host` | Listen address | `0.0.0.0` |
| `server.port` | HTTP port | `8080` |

### Storage

| Setting | Description | Default |
|---------|-------------|---------|
| `storage.audio_path` | Audio file directory | `./data/audio` |

### Environment Variables

| Variable | Config Path |
|----------|-------------|
| `DB_HOST` | database.host |
| `DB_PORT` | database.port |
| `DB_NAME` | database.name |
| `DB_USER` | database.user |
| `DB_PASSWORD` | database.password |
| `MQTT_BROKER` | mqtt.broker |
| `AUDIO_PATH` | storage.audio_path |

---

## API Overview

Full interactive documentation at `/swagger/` and `/websocket`.

### REST Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/systems` | List systems |
| `GET /api/v1/talkgroups` | List/search talkgroups |
| `GET /api/v1/units` | List/search units |
| `GET /api/v1/units/active` | Currently active units |
| `GET /api/v1/calls` | List calls (with audio) |
| `GET /api/v1/calls/recent` | Recent completed calls |
| `GET /api/v1/calls/{id}` | Get call details |
| `GET /api/v1/calls/{id}/audio` | Stream audio file |
| `GET /api/v1/calls/{id}/transmissions` | Unit transmissions in call |
| `GET /api/v1/call-groups` | Deduplicated call groups |
| `GET /api/v1/stats` | System statistics |
| `GET /api/v1/recorders` | Recorder status |

### WebSocket

Connect to `ws://<host>:8080/api/ws` and subscribe:

```json
{"action": "subscribe", "channels": ["calls", "units", "rates", "recorders"]}
```

**Events:** `call_start`, `call_end`, `audio_available`, `unit_event`, `rate_update`, `recorder_update`

---

## Building from Source

```bash
# Build
go build -o tr-engine

# Run tests
go test ./...

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o tr-engine-linux
GOOS=windows GOARCH=amd64 go build -o tr-engine.exe
```

---

## License

This project is part of the trunk-recorder ecosystem.
