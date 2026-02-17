# Getting Started — Docker with Existing MQTT Broker

Run tr-engine via Docker Compose, connecting to an MQTT broker you already have running (e.g., the one your trunk-recorder is already publishing to). You only need two files — no repo clone required.

> **Other installation methods:**
> - **[Docker Compose (all-in-one)](./docker.md)** — includes its own MQTT broker
> - **[Build from source](./getting-started.md)** — compile everything yourself
> - **[Binary releases](./binary-releases.md)** — download a pre-built binary

## Prerequisites

- Docker and Docker Compose
- A running **MQTT broker** that trunk-recorder is already publishing to
- The broker's address, port, and credentials (if any)

## 1. Create a project directory and grab the files

```bash
mkdir tr-engine && cd tr-engine
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/schema.sql
curl -sO https://raw.githubusercontent.com/LumenPrima/tr-engine/master/docker-compose.yml
```

> **Why download `schema.sql` separately?** The schema is embedded inside the tr-engine image, but PostgreSQL runs in a separate container and needs the file mounted into its init directory to set up tables on first boot. Docker Compose can't share files between containers, so it's mounted from the host.

## 2. Edit `docker-compose.yml`

Open `docker-compose.yml` and change the tr-engine `environment` section to point at your broker. Remove the `mosquitto` service and the `depends_on` reference to it — you don't need a bundled broker.

Here's what the file should look like after editing:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: trengine
      POSTGRES_PASSWORD: trengine
      POSTGRES_DB: trengine
    volumes:
      - ./pgdata:/var/lib/postgresql/data
      - ./schema.sql:/docker-entrypoint-initdb.d/01-schema.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U trengine"]
      interval: 5s
      timeout: 3s
      retries: 5

  tr-engine:
    image: ghcr.io/lumenprima/tr-engine:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://trengine:trengine@postgres:5432/trengine?sslmode=disable
      MQTT_BROKER_URL: tcp://192.168.1.50:1883
      MQTT_USERNAME:
      MQTT_PASSWORD:
      MQTT_TOPICS: "trengine/#"
      AUDIO_DIR: /data/audio
      LOG_LEVEL: info
    volumes:
      - ./audio:/data/audio
      # Uncomment to live-edit web UI without rebuilding:
      # - ./web:/opt/tr-engine/web
    depends_on:
      postgres:
        condition: service_healthy
```

### What to change

| Variable | What to set |
|----------|-------------|
| `MQTT_BROKER_URL` | Your broker's address — e.g. `tcp://192.168.1.50:1883` |
| `MQTT_USERNAME` | Broker credentials (leave empty for anonymous) |
| `MQTT_PASSWORD` | Broker credentials (leave empty for anonymous) |
| `MQTT_TOPICS` | Must match your TR plugin's topic prefix with `/#`. If your TR plugin uses `topic: "myradio/feeds"`, set this to `myradio/#` |

### Broker on the Docker host

If the MQTT broker runs on the same machine as Docker, use `host.docker.internal` and add `extra_hosts` to the tr-engine service:

```yaml
  tr-engine:
    image: ghcr.io/lumenprima/tr-engine:latest
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      MQTT_BROKER_URL: tcp://host.docker.internal:1883
      # ... rest of env vars
```

On macOS and Windows, `host.docker.internal` works without `extra_hosts`. On Linux, the `extra_hosts` line is required.

## 3. Start

```bash
docker compose up -d
```

On first run, PostgreSQL initializes from `schema.sql` (takes a few seconds). tr-engine waits for that to finish before starting.

## 4. Verify

```bash
# Check logs — look for "mqtt connected" and "subscribing"
docker compose logs tr-engine --tail 30

# Health check — database and mqtt should both show "connected"
curl http://localhost:8080/api/v1/health

# Watch live events (Ctrl-C to stop)
curl -N http://localhost:8080/api/v1/events/stream
```

Open http://localhost:8080 for the web UI. Systems and talkgroups auto-populate as trunk-recorder sends data — no manual configuration needed.

## Data

All data is stored in bind-mounted directories next to your `docker-compose.yml`:

| Directory | Contents |
|-----------|----------|
| `./pgdata` | PostgreSQL data |
| `./audio` | Call audio files |

To back up the database:

```bash
docker compose exec postgres pg_dump -U trengine trengine > backup.sql
```

## Other settings

Add any variable from [sample.env](https://github.com/LumenPrima/tr-engine/blob/master/sample.env) to the `environment` section in `docker-compose.yml`:

```yaml
      AUTH_TOKEN: my-secret              # enable API authentication
      LOG_LEVEL: debug                   # more verbose logging
      RAW_STORE: "false"                 # disable raw MQTT archival (saves disk)
      RAW_EXCLUDE_TOPICS: trunking_message  # exclude high-volume raw archival
```

Then restart: `docker compose up -d`

## Custom web UI

Mount a local `web/` directory to override the embedded UI files without rebuilding:

```yaml
    volumes:
      - ./audio:/data/audio
      - ./web:/opt/tr-engine/web
```

Changes take effect on the next browser request — no restart needed. See [Updating Web Files](./docker.md#custom-web-ui-files) for how to pull the latest UI files from GitHub.

## Upgrading

```bash
docker compose pull && docker compose up -d
```

Database and audio files persist in the bind-mounted directories. Check the release notes for any schema migrations.

## Troubleshooting

**MQTT not connecting:** Check that the broker address is reachable from inside the container. Run `docker compose logs tr-engine` and look for connection errors. If the broker is on `localhost`, use `host.docker.internal` instead (see above).

**No data appearing:** Verify trunk-recorder is publishing with `mosquitto_sub -h your-broker -t '#' -v`. Check that `MQTT_TOPICS` matches the TR plugin's topic prefix.
