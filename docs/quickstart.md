# Quick Start -- tr-engine in 5 Minutes

tr-engine is a backend service that ingests data from [trunk-recorder](https://github.com/robotastic/trunk-recorder) instances via MQTT, file watching, or HTTP upload, stores it in PostgreSQL, and serves it through a REST API with real-time SSE streaming. This guide uses Docker Compose to get you from zero to seeing radio data in the UI with no manual configuration.

> **Other installation methods:**
> - **[Docker Compose (full guide)](./docker.md)** -- detailed Docker setup with all configuration options
> - **[Docker with existing MQTT](./docker-external-mqtt.md)** -- connect to a broker you already run
> - **[Build from source](./getting-started.md)** -- compile from source, bring your own PostgreSQL and MQTT
> - **[Binary releases](./binary-releases.md)** -- download a pre-built binary

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/)
- A running trunk-recorder instance (or audio files to watch)

## 1. Download and start

```bash
mkdir tr-engine && cd tr-engine
curl -sO https://raw.githubusercontent.com/trunk-reporter/tr-engine/master/docker-compose.yml
docker compose up -d
```

That's it. One file, one command. On first run:

- **PostgreSQL** starts and tr-engine auto-applies the database schema
- **Mosquitto** starts on port **1883** (anonymous access)
- **tr-engine** connects to both and starts listening on port **8080**

With no auth variables set, tr-engine starts in open mode. See the [Docker Compose guide](./docker.md#securing-a-public-facing-instance) before exposing it outside your LAN.

Verify it's running:

```bash
curl http://localhost:8080/api/v1/health
```

## 2. Get data in

tr-engine needs at least one ingest source. Choose the one that fits your setup:

### a) MQTT (richest data)

If trunk-recorder has the [MQTT Status plugin](https://github.com/TrunkRecorder/trunk-recorder-mqtt-status) installed, point it at the Mosquitto broker running on your Docker host:

```json
{
  "plugins": [
    {
      "name": "MQTT Status",
      "library": "libmqtt_status_plugin.so",
      "broker": "tcp://localhost:1883",
      "topic": "trengine/feeds",
      "unit_topic": "trengine/units",
      "console_logs": true,
      "instanceId": "my-site"
    }
  ]
}
```

Replace `localhost` with your Docker host IP if trunk-recorder runs on a different machine. The topic prefix is yours to choose -- tr-engine routes messages based on the trailing segments (`call_start`, `on`, `message`, etc.), not the prefix. If you change the prefix, set `MQTT_TOPICS` in your `.env` to match with a `/#` wildcard (default is `#`, which receives all topics).

MQTT provides the richest data: real-time `call_start`/`call_end` events, unit activity, recorder state, decode rates, and console logs.

### b) File watch (simplest)

If you don't have the MQTT plugin, tr-engine can watch trunk-recorder's audio output directory for new recordings. Add to `.env`:

```env
WATCH_DIR=/tr-audio
```

And bind-mount the audio directory in `docker-compose.yml`:

```yaml
    volumes:
      - /path/to/trunk-recorder/audio:/tr-audio:ro
```

Then restart: `docker compose up -d`

> **Note:** File watch mode only produces `call_end` events. For `call_start`, unit events, and recorder state, add MQTT.

### c) TR auto-discovery (easiest)

If trunk-recorder runs on the same machine and its directory is accessible, point tr-engine at it and let it figure out the rest. Add to `.env`:

```env
TR_DIR=/tr-config
```

And bind-mount the directory in `docker-compose.yml`:

```yaml
    volumes:
      - /path/to/trunk-recorder:/tr-config:ro
```

Then restart: `docker compose up -d`

This auto-discovers:

- `captureDir` from `config.json` -- enables file watch and audio serving automatically
- System names from `config.json` system entries
- Talkgroup and unit tag CSVs

All three ingest modes can run simultaneously.

## 3. Use the UI

Open http://localhost:8080

The index page auto-discovers all available dashboards. Key pages to explore first:

| Page | What it shows |
|------|---------------|
| **OmniTrunker** | Real-time system overview -- active calls, recorders, decode rates |
| **Event Horizon** | Logarithmic timeline where events drift from "now" into the past |
| **Call History** | Searchable call log with inline audio playback and transmission timeline |
| **Live Events** | Raw SSE event stream with type filtering |

Systems, sites, talkgroups, and units auto-populate as data flows in. There is no manual configuration. Open the **OmniTrunker** page to see your radio system appear in real time, or browse **Call History** to replay recordings with their metadata.

Use the nav dropdown in the sticky header to switch between pages, or press **Ctrl+Shift+T** to cycle through the 11 built-in themes.

## 4. What's next

- **Live audio streaming** -- hear radio traffic in real time through the browser. See the [Docker Compose guide](./docker.md#live-audio-streaming)
- **Transcription** -- automatic speech-to-text for call recordings. See the [Docker Compose guide](./docker.md#transcription-stt)
- **Production security** -- enable JWT auth and API keys before exposing to the internet. See [Docker Compose guide](./docker.md#securing-a-public-facing-instance) and [Auth Migration Guide](./migrating-auth.md)
- **Upgrading** -- `docker compose pull && docker compose up -d` (database persists across updates)
- **API reference** -- see `openapi.yaml` in the repository root, or open the built-in Swagger UI at `/docs.html`
