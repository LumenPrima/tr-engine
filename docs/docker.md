# Getting Started — Docker Compose

Run tr-engine with a single command. Docker Compose handles PostgreSQL, the MQTT broker, and tr-engine — you just need trunk-recorder pointed at the broker.

> **Other installation methods:**
> - **[Docker with existing MQTT](./docker-external-mqtt.md)** — connect to a broker you already run instead of bundling one
> - **[Build from source](./getting-started.md)** — compile everything yourself from scratch
> - **[Binary release](./binary-releases.md)** — download a pre-built binary, just add PostgreSQL and MQTT

## Prerequisites

- Docker and Docker Compose
- A running trunk-recorder instance with the [MQTT Status plugin](https://github.com/TrunkRecorder/trunk-recorder-mqtt-status)

## 1. Grab the files

```bash
mkdir tr-engine && cd tr-engine
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/docker-compose.yml
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/schema.sql
mkdir -p docker && curl -so docker/mosquitto.conf https://raw.githubusercontent.com/LumenPrima/tr-engine/master/docker/mosquitto.conf
```

Three files — that's all you need:
- `docker-compose.yml` — orchestrates PostgreSQL, Mosquitto, and tr-engine
- `schema.sql` — database schema, auto-loaded on first run
- `docker/mosquitto.conf` — minimal Mosquitto config (anonymous access on port 1883)

> **Why download `schema.sql` separately?** The schema is embedded inside the tr-engine image, but PostgreSQL runs in a separate container and needs the file mounted into its init directory to set up tables on first boot. Docker Compose can't share files between containers, so it's mounted from the host.

## 2. Start

```bash
docker compose up -d
```

On first run:
- PostgreSQL starts and auto-loads `schema.sql`
- Mosquitto starts on port **1883**
- tr-engine connects to both and starts listening

Verify it's running:

```bash
curl http://localhost:8080/api/v1/health
```

## 3. Point trunk-recorder at the broker

In your trunk-recorder `config.json`, set the MQTT plugin's broker to your Docker host:

```json
{
  "plugins": [
    {
      "name": "MQTT Status",
      "library": "libmqtt_status_plugin.so",
      "broker": "tcp://YOUR_DOCKER_HOST:1883",
      "topic": "trengine/feeds",
      "unit_topic": "trengine/units",
      "console_logs": true
    }
  ]
}
```

Replace `YOUR_DOCKER_HOST` with the IP or hostname of the machine running Docker. If trunk-recorder runs on the same machine, use `localhost`.

**The topic prefix is yours to choose.** tr-engine routes messages based on the trailing segments (e.g. `call_start`, `on`, `message`), not the prefix. Use any prefix you like — `trengine`, `myradio`, `robotastic` — as long as `MQTT_TOPICS` in `docker-compose.yml` matches with a `/#` wildcard. The default compose file uses `trengine/#` which matches the example above.

Once trunk-recorder connects, systems and talkgroups will auto-populate within seconds.

## 4. Access

- **Web UI:** http://localhost:8080
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
  MQTT_TOPICS: "trengine/#"
  AUDIO_DIR: /data/audio
  LOG_LEVEL: debug        # add any env var from sample.env
  AUTH_TOKEN: my-secret   # enable API authentication
```

Then restart: `docker compose up -d`

### Filesystem audio (TR_AUDIO_DIR)

Instead of receiving audio over MQTT as base64, tr-engine can serve audio files directly from trunk-recorder's filesystem. This avoids the encoding overhead and eliminates duplicate files.

To enable it, bind-mount trunk-recorder's audio directory into the tr-engine container and set `TR_AUDIO_DIR`:

```yaml
  tr-engine:
    environment:
      TR_AUDIO_DIR: /tr-audio
    volumes:
      - /path/to/trunk-recorder/audio:/tr-audio:ro
```

When `TR_AUDIO_DIR` is set, tr-engine skips saving audio from MQTT and instead resolves files using the `call_filename` path that trunk-recorder reports at call_end. In your TR plugin config, keep `mqtt_audio: true` but set `mqtt_audio_type: none` — this sends the call metadata (frequencies, transmissions, unit list) without the base64 audio payload, saving encoding CPU and MQTT bandwidth.

Both modes coexist during a transition — existing MQTT-ingested audio still serves from `AUDIO_DIR`.

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
docker compose pull && docker compose up -d
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
