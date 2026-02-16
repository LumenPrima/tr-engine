# Getting Started — Docker Compose

Run tr-engine with a single command. Docker Compose handles PostgreSQL, the MQTT broker, and tr-engine — you just need trunk-recorder pointed at the broker.

> **Other installation methods:**
> - **[Build from source](./getting-started.md)** — compile everything yourself from scratch
> - **[Binary release](./binary-releases.md)** — download a pre-built binary, just add PostgreSQL and MQTT

## Prerequisites

- Docker and Docker Compose
- A running trunk-recorder instance with the [MQTT Status plugin](https://github.com/TrunkRecorder/trunk-recorder-mqtt-status)

## 1. Start

```bash
git clone https://github.com/LumenPrima/tr-engine.git
cd tr-engine
docker compose up -d
```

That's it. On first run:
- PostgreSQL starts and auto-loads `schema.sql`
- Mosquitto starts on port **1883**
- tr-engine connects to both and starts listening

Verify it's running:

```bash
curl http://localhost:8080/api/v1/health
```

## 2. Point trunk-recorder at the broker

In your trunk-recorder `config.json`, set the MQTT plugin's broker to your Docker host:

```json
{
  "plugins": [
    {
      "name": "MQTT Status",
      "library": "libmqtt_status_plugin.so",
      "broker": "tcp://YOUR_DOCKER_HOST:1883",
      "topic": "trdash/feeds",
      "unit_topic": "trdash/units",
      "message_topic": "trdash/messages",
      "console_logs": true
    }
  ]
}
```

Replace `YOUR_DOCKER_HOST` with the IP or hostname of the machine running Docker. If trunk-recorder runs on the same machine, use `localhost`.

Once trunk-recorder connects, systems and talkgroups will auto-populate within seconds.

## 3. Access

- **Web UI:** http://localhost:8080/irc-radio-live.html
- **API:** http://localhost:8080/api/v1/health
- **API docs:** http://localhost:8080/docs.html

## Data

Two named volumes persist across restarts and upgrades:

| Volume | Contents | Path in container |
|--------|----------|-------------------|
| `tr-engine-db` | PostgreSQL data | `/var/lib/postgresql/data` |
| `tr-engine-audio` | Call audio files | `/data/audio` |

To back up the database:

```bash
docker compose exec postgres pg_dump -U trengine trengine > backup.sql
```

## Configuration

Override tr-engine settings by editing the `environment` section in `docker-compose.yml`:

```yaml
environment:
  DATABASE_URL: postgres://trengine:trengine@postgres:5432/trengine?sslmode=disable
  MQTT_BROKER_URL: tcp://mosquitto:1883
  MQTT_TOPICS: "trdash/#"
  AUDIO_DIR: /data/audio
  LOG_LEVEL: debug        # add any env var from sample.env
  AUTH_TOKEN: my-secret   # enable API authentication
```

Then restart: `docker compose up -d`

### Custom web UI files

The web UI is embedded in the binary, but you can override it by mounting a local directory:

```yaml
volumes:
  - ./web:/opt/tr-engine/web
```

When a `web/` directory exists on disk, tr-engine serves from it instead of the embedded files. Changes take effect on the next browser request — no restart needed. This is useful for iterating on the UI without rebuilding the Docker image.

To pull the latest web UI files from GitHub without rebuilding:

**Linux/Mac:**
```bash
mkdir -p web && cd web && curl -s https://api.github.com/repos/LumenPrima/tr-engine/contents/web | python3 -c "import json,sys,urllib.request; [urllib.request.urlretrieve(f['download_url'],f['name']) for f in json.load(sys.stdin) if f['type']=='file']"
```

**Windows (PowerShell):**
```powershell
mkdir -Force web; (irm https://api.github.com/repos/LumenPrima/tr-engine/contents/web) | ? type -eq file | % { iwr $_.download_url -Out "web/$($_.name)" }
```

Run from the directory containing your `docker-compose.yml`. Changes take effect on the next browser refresh — no restart needed.

## Upgrading

```bash
cd tr-engine
git pull
docker compose up -d --build
```

The database volume persists — your data is safe. If a release includes schema migrations, they'll be noted in the release notes.

## Logs

```bash
# All services
docker compose logs -f

# Just tr-engine
docker compose logs -f tr-engine
```

## Stopping

```bash
# Stop (data preserved)
docker compose down

# Stop and delete all data (fresh start)
docker compose down -v
```
