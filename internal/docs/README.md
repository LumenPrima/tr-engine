# tr-engine

Backend service for aggregating [trunk-recorder](https://github.com/robotastic/trunk-recorder) data via MQTT. Works with the [trunk-recorder MQTT plugin](https://github.com/TrunkRecorder/tr-plugin-mqtt).

**Designed as a backend for frontend developers** - build your own UI using any platform/language with the REST and WebSocket APIs. Included web dashboards are functional demos, not feature-complete applications.

## Highlights

- **Easy Mode** - Zero-config deployment. Just point at trunk-recorder's audio/logs directories. No MQTT required.
- **Self-contained deployment** - Single binary with embedded PostgreSQL and MQTT broker. No external dependencies required.
- **Watch Mode** - File watcher + log tailing for MQTT-free operation with real-time updates.
- **Data ingestion** - Receives calls, audio, unit events, recorder status via MQTT or file watching.
- **Speech-to-text transcription** - Whisper API support with word-level timestamps for synchronized playback.
- **Cross-site deduplication** - Links duplicate P25 call recordings from multi-site systems.
- **REST API** - Historical queries for calls, talkgroups, units, statistics with server-side search/sort.
- **WebSocket API** - Real-time event streaming with subscription filtering.
- **Authentication** - Optional API key and session-based auth with admin key management.
- **Interactive docs** - Swagger UI at `/swagger/`, WebSocket tester at `/websocket`.
- **Demo web UI** - Dashboard with call history, unit tracking, audio playback, transcription search.
- **Prometheus metrics** - System health and decode rate monitoring at `/metrics`.

## Quick Start

### Option 1: Easy Mode (Simplest - No MQTT Required)

Zero configuration. Just point at trunk-recorder's audio and log directories:

```bash
docker run -d -p 8080:8080 \
  -v ./data:/data \
  -v /path/to/trunk-recorder/audio:/audio:ro \
  -v /path/to/trunk-recorder/logs:/logs:ro \
  ghcr.io/lumenprima/tr-engine:0.3.1-beta1 --easy
```

That's it. No MQTT setup, no config file, no database setup. Open `http://localhost:8080` to browse recordings.

**What works:**
- Automatic import of all historical recordings
- Real-time call tracking via log tailing
- Decode rate monitoring
- Embedded database for zero dependencies

**Limitations vs MQTT mode:**
- No active unit tracking (unit on/off/affiliate events require MQTT)
- No real-time recorder status
- Transmission metadata parsed from JSON sidecars (less detailed than MQTT)

### Option 2: Self-Contained with MQTT

For full real-time features including unit tracking and recorder status:

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

### Option 3: Docker with MQTT

Run the pre-built image (self-contained mode with embedded PostgreSQL and MQTT):

```bash
# Create directories first (required - Docker creates them as root otherwise)
mkdir -p data config

# First run generates config.yaml, then exits
docker run --rm \
  -v ./data:/data \
  -v ./config:/app/config \
  ghcr.io/lumenprima/tr-engine:0.3.1-beta1

# Second run starts the service
docker run -d \
  -p 8080:8080 -p 1883:1883 \
  -v ./data:/data \
  -v ./config:/app/config \
  ghcr.io/lumenprima/tr-engine:0.3.1-beta1
```

**Note:** Beta releases require the specific version tag (e.g., `:0.3.1-beta1`). The `:latest` tag is only applied to stable releases.

### Option 4: Docker Compose (TimescaleDB)

Runs tr-engine with external PostgreSQL (TimescaleDB) and Mosquitto MQTT broker. Use this for high-volume deployments where time-series optimizations and long-term retention matter. Data stored in local `./data/` folder.

```bash
docker-compose up -d
```

### Configure trunk-recorder (Options 2-4 only)

Install the [MQTT status plugin](https://github.com/TrunkRecorder/tr-plugin-mqtt) and add to your trunk-recorder config:

```json
"plugins": [
    {
        "name": "mqtt_status",
        "library": "libmqtt_status_plugin.so",
        "broker": "tcp://<tr-engine-host>:1883",
        "topic": "feeds/main",
        "unit_topic": "units/main",
        "clientid": "tr-publish",
        "username": "",
        "password": "",
        "mqtt_audio": true,
        "console_logs": false,
        "mqtt_audio_type": "m4a"
    }
]
```

- `topic` and `unit_topic` are arbitrary - just ensure they match tr-engine's config (`mqtt.topics.status` and `mqtt.topics.units` patterns)
- `mqtt_audio_type` can be `m4a`, `wav`, or `both`
- **Note:** MQTT authentication is not currently implemented. Run on a trusted network or use a firewall.
- **Note:** If running multiple tr-engine instances against the same MQTT broker, each must have a unique `mqtt.client_id` in config.

### Open the dashboard

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

Single binary or Docker image with embedded PostgreSQL and MQTT broker. Zero external dependencies. Data stored in `./data/`. Suitable for most deployments including multi-site systems with moderate call volume.

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

### External Services (Optional)

External PostgreSQL and MQTT are **optional** but recommended for high-volume deployments (multiple busy systems, long-term retention). Benefits include TimescaleDB for time-series optimization, easier backups, and horizontal scaling.

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
| `storage.mode` | Storage mode: `copy`, `external`, or `watch` | `copy` |
| `storage.audio_path` | Audio file directory | `./data/audio` |
| `storage.log_path` | TR log directory (for watch mode) | |

**Storage Modes:**
- `copy` - Audio received via MQTT, copied to audio_path
- `external` - References TR's audio files directly (no copy)
- `watch` - File watcher mode, no MQTT required. Uses `--easy` flag.

### Authentication (Optional)

| Setting | Description | Default |
|---------|-------------|---------|
| `auth.enabled` | Enable authentication | `false` |
| `auth.dashboard.username` | Dashboard login username | |
| `auth.dashboard.password` | Dashboard login password | |
| `auth.api_keys` | List of root API keys | `[]` |

### Transcription (Optional)

| Setting | Description | Default |
|---------|-------------|---------|
| `transcription.enabled` | Enable transcription | `false` |
| `transcription.provider` | Provider (`openai`) | `openai` |
| `transcription.openai.api_key` | OpenAI API key | |
| `transcription.openai.base_url` | API base URL (for compatible APIs) | |
| `transcription.openai.model` | Model name | `whisper-1` |
| `transcription.openai.prompt` | Initial prompt for context | |
| `transcription.min_duration` | Min call duration to transcribe | `2.0` |
| `transcription.concurrency` | Concurrent transcription jobs | `2` |

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
| `OPENAI_API_KEY` | transcription.openai.api_key |

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
| `GET /api/v1/calls/{id}/transcription` | Get call transcription |
| `POST /api/v1/calls/{id}/transcribe` | Queue call for transcription |
| `GET /api/v1/transcriptions/search` | Full-text search transcriptions |
| `GET /api/v1/transcriptions/recent` | Recent transcriptions |
| `GET /api/v1/call-groups` | Deduplicated call groups |
| `GET /api/v1/stats` | System statistics |
| `GET /api/v1/recorders` | Recorder status |
| `GET /api/v1/admin/api-keys` | List API keys (admin) |
| `POST /api/v1/admin/api-keys` | Create API key (admin) |
| `DELETE /api/v1/admin/api-keys/{id}` | Revoke API key (admin) |

### WebSocket

Connect to `ws://<host>:8080/api/ws` and subscribe:

```json
{"action": "subscribe", "channels": ["calls", "units", "rates", "recorders"]}
```

**Events:** `call_start`, `call_end`, `audio_available`, `unit_event`, `rate_update`, `recorder_update`

---

## Building from Source

```bash
# Build (stripped binary, ~10MB)
go build -ldflags="-w -s" -o tr-engine

# Run tests (193 tests)
go test ./...

# Cross-compile
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o tr-engine-linux
GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o tr-engine.exe
```

---

## Roadmap

### Completed (v0.3.x)
- [x] Authentication/authorization (API keys, session auth, admin API)
- [x] Speech-to-text transcription (Whisper API with word timestamps)
- [x] Comprehensive test suite (193 tests across all packages)
- [x] Natural composite keys for talkgroups/units (sysid scoping)
- [x] Full-text search for transcriptions

### In Progress
- [ ] Performance optimization (batch inserts, query tuning)

### Future Features
- [ ] Embedded local transcription (download whisper.cpp + model on demand, no external API needed)
- [ ] Talkgroup replay/timeline view
- [ ] MQTT publish capability (publish state back to MQTT)
- [ ] Historical unit data import from external sources

### Future Ideas
- [ ] Visual talkgroup/unit activity heatmaps
- [ ] Location recognition and mapping from transmission content
- [ ] Context-aware "interest meter" - auto-alert on interesting topics/events
- [ ] Alerts on new units/talkgroups appearing
- [ ] Talkgroup context inference from associated unit patterns

---

## Disclaimer

This project was vibe-coded with AI assistance (Claude). While functional and tested against real trunk-recorder deployments, the code has not been formally audited. Use at your own risk. The binaries may or may not:
- Crash your PC
- Befriend your pets
- Develop aspirations of world domination

Bug reports welcome. Pull requests even more so.

## License

MIT License - see [LICENSE](LICENSE) for details.
