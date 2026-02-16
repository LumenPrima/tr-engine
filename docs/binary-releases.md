# Getting Started — Binary Release

Download a pre-built binary and get running in minutes. You still need PostgreSQL, an MQTT broker, and a running trunk-recorder instance.

> **Other installation methods:**
> - **[Build from source](./getting-started.md)** — compile everything yourself from scratch
> - **[Docker Compose](./docker.md)** — single `docker compose up` with everything pre-configured
> - **[Docker with existing MQTT](./docker-external-mqtt.md)** — Docker Compose connecting to a broker you already run

## Prerequisites

- A running **trunk-recorder** instance with the [MQTT Status plugin](https://github.com/TrunkRecorder/trunk-recorder-mqtt-status) configured
- An **MQTT broker** (e.g., Mosquitto) that trunk-recorder is publishing to
- **PostgreSQL 17+** (18 recommended)

If you don't have these yet, see the [build from source guide](./getting-started.md) for setup instructions on each component.

## 1. Download

Grab the latest release for your platform from the [releases page](https://github.com/yourusername/tr-engine/releases):

| Platform | File |
|----------|------|
| Linux (amd64) | `tr-engine-linux-amd64.tar.gz` |
| Linux (arm64) | `tr-engine-linux-arm64.tar.gz` |
| Windows (amd64) | `tr-engine-windows-amd64.zip` |
| macOS (amd64) | `tr-engine-darwin-amd64.tar.gz` |
| macOS (arm64) | `tr-engine-darwin-arm64.tar.gz` |

```bash
# Example: Linux amd64
curl -LO https://github.com/yourusername/tr-engine/releases/latest/download/tr-engine-linux-amd64.tar.gz
tar xzf tr-engine-linux-amd64.tar.gz
chmod +x tr-engine
```

The archive contains:
- `tr-engine` (or `tr-engine.exe` on Windows) — the server binary
- `schema.sql` — database schema
- `sample.env` — configuration template

## 2. Set up the database

Create a database and user, then load the schema:

```bash
sudo -u postgres psql -c "CREATE USER trengine WITH PASSWORD 'your_password_here';"
sudo -u postgres psql -c "CREATE DATABASE trengine OWNER trengine;"
psql -U trengine -d trengine -f schema.sql
```

This creates all tables, indexes, partitions, and triggers. Takes a few seconds.

## 3. Configure

```bash
cp sample.env .env
```

Edit `.env` — you only need to set two values:

```env
DATABASE_URL=postgres://trengine:your_password_here@localhost:5432/trengine?sslmode=disable
MQTT_BROKER_URL=tcp://localhost:1883
```

Make sure `MQTT_TOPICS` matches your trunk-recorder plugin's topic prefix. If your TR config uses `"topic": "trengine/feeds"`, then:

```env
MQTT_TOPICS=trengine/#
```

See `sample.env` for all available options (HTTP port, auth token, log level, audio directory, raw archival settings).

## 4. Run

```bash
./tr-engine
```

You should see:

```
{"level":"info","version":"...","message":"tr-engine starting"}
{"level":"info","component":"database","message":"database connected"}
{"level":"info","component":"mqtt","message":"mqtt connected, subscribing"}
{"level":"info","listen":":8080","message":"tr-engine ready"}
```

## 5. Verify

```bash
# Health check
curl http://localhost:8080/api/v1/health

# List discovered systems (populated once TR sends data)
curl http://localhost:8080/api/v1/systems

# Watch live events
curl -N http://localhost:8080/api/v1/events/stream
```

Open `http://localhost:8080/irc-radio-live.html` in a browser for the live web UI.

## Running as a service

### systemd (Linux)

Create `/etc/systemd/system/tr-engine.service`:

```ini
[Unit]
Description=tr-engine
After=network.target postgresql.service mosquitto.service

[Service]
Type=simple
User=trengine
WorkingDirectory=/opt/tr-engine
ExecStart=/opt/tr-engine/tr-engine
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tr-engine
sudo journalctl -u tr-engine -f
```

### Windows

Use [NSSM](https://nssm.cc/) or run as a scheduled task:

```powershell
nssm install tr-engine C:\tr-engine\tr-engine.exe
nssm set tr-engine AppDirectory C:\tr-engine
nssm start tr-engine
```

## Upgrading

1. Stop tr-engine
2. Replace the binary with the new version
3. Check the release notes for any schema migrations
4. If a migration is needed: `psql -U trengine -d trengine -f migrations/xxx.sql`
5. Start tr-engine

The schema is designed to be additive — new versions add tables/columns but don't break existing data. `schema.sql` is always safe to re-run on a fresh database.
