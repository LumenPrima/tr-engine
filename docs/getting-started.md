# Getting Started — Build from Source

This guide walks through setting up tr-engine from scratch on bare metal: installing each component yourself, building from source, and wiring everything together.

> **Other installation methods:**
> - **[Docker Compose](./docker.md)** — single `docker compose up` with everything pre-configured
> - **[Docker with existing MQTT](./docker-external-mqtt.md)** — Docker Compose connecting to a broker you already run
> - **[Binary releases](./binary-releases.md)** — download a pre-built binary, just add PostgreSQL and MQTT

## Architecture

```
trunk-recorder  ──MQTT──>  broker  ──MQTT──>  tr-engine  ──REST/SSE──>  clients
   (radio)                (mosquitto)          (Go server)              (web UI)
                                                   │
                                                   v
                                               PostgreSQL
```

trunk-recorder captures radio traffic from SDR hardware and publishes events over MQTT. tr-engine subscribes to that MQTT feed, stores everything in PostgreSQL, and exposes it via a REST API with real-time SSE streaming.

## 1. MQTT Broker

tr-engine needs an MQTT broker between it and trunk-recorder. Mosquitto is the simplest choice.

### Install

```bash
# Debian/Ubuntu
sudo apt install mosquitto mosquitto-clients

# macOS
brew install mosquitto

# Docker
docker run -d --name mosquitto -p 1883:1883 eclipse-mosquitto
```

### Configure

For a local setup, the default config works fine (anonymous access on port 1883). For remote access, create `/etc/mosquitto/conf.d/listener.conf`:

```
listener 1883
allow_anonymous true
```

Restart with `sudo systemctl restart mosquitto`.

### Verify

```bash
# In one terminal, subscribe to all topics:
mosquitto_sub -t '#' -v

# In another, publish a test message:
mosquitto_pub -t 'test' -m 'hello'
```

## 2. PostgreSQL

tr-engine requires PostgreSQL 17 or later (tested on 18). It uses partitioned tables, JSONB, and GIN indexes.

### Install

```bash
# Debian/Ubuntu (via official PostgreSQL apt repo)
sudo apt install postgresql-18

# macOS
brew install postgresql@18

# Docker
docker run -d --name postgres -p 5432:5432 -e POSTGRES_PASSWORD=secret postgres:18
```

### Create database and user

```bash
sudo -u postgres psql
```

```sql
CREATE USER trengine WITH PASSWORD 'your_password_here';
CREATE DATABASE trengine OWNER trengine;
\q
```

### Load the schema

```bash
psql -U trengine -d trengine -f schema.sql
```

This creates all tables, indexes, triggers, partition functions, and initial partitions (current month + 3 months ahead). Partition maintenance runs automatically within tr-engine after that.

## 3. trunk-recorder

[trunk-recorder](https://github.com/robotastic/trunk-recorder) captures P25/SmartNet/conventional radio traffic using SDR hardware (RTL-SDR, HackRF, etc.). Full setup is covered in the [trunk-recorder docs](https://trunkrecorder.com/docs/Install). This section covers only the MQTT plugin configuration that tr-engine needs.

### Install the MQTT plugin

The MQTT Status plugin is a separate plugin, not built into trunk-recorder core.

```bash
# Install MQTT libraries
sudo apt install libpaho-mqtt-dev libpaho-mqttpp-dev

# Clone the plugin into your trunk-recorder source tree
cd trunk-recorder/user_plugins/
git clone https://github.com/TrunkRecorder/trunk-recorder-mqtt-status.git

# Rebuild trunk-recorder (from the build directory)
make install
```

A Docker image with the plugin pre-integrated is also available:
```bash
docker pull thegreatcodeholio/trunk-recorder-mqtt:latest
```

### Configure the plugin

Add the plugin to your trunk-recorder `config.json`:

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

| Field | Required | Description |
|-------|----------|-------------|
| `broker` | Yes | MQTT broker URL (`tcp://host:1883` or `ssl://host:8883`) |
| `topic` | Yes | Base topic for call/recorder/system events |
| `unit_topic` | No | Topic prefix for unit events (on/off/call/join). Recommended. |
| `message_topic` | No | Topic prefix for trunking messages. **Very high volume** — omit unless you specifically need trunking message data. |
| `console_logs` | No | Publish TR console output over MQTT (default: false) |
| `mqtt_audio` | No | Send audio as base64 in MQTT messages (default: false) |
| `instanceId` | No | Identifier for this TR instance (default: `trunk-recorder`) |
| `username` | No | MQTT broker credentials |
| `password` | No | MQTT broker credentials |

**The topic prefix is yours to choose.** The values above use `trengine/feeds` and `trengine/units`, but you can use any prefix — `myradio/feeds`, `robotastic/feeds`, etc. tr-engine routes messages based on the trailing segments (`call_start`, `on`, `message`, etc.), not the prefix. Just make sure `MQTT_TOPICS` in your tr-engine config matches with a `/#` wildcard (e.g. `MQTT_TOPICS=trengine/#`).

**Topic structure produced:**

With the config above, the plugin publishes to:
- `{topic}/call_start`, `{topic}/call_end`, `{topic}/recorders`, `{topic}/rates`, etc.
- `{unit_topic}/{sys_name}/on`, `{unit_topic}/{sys_name}/call`, `{unit_topic}/{sys_name}/join`, etc.

### Multiple trunk-recorder instances

If you have multiple TR instances monitoring the same P25 network from different sites, point them all at the same MQTT broker with the same topic prefix. tr-engine will auto-merge them into a single system with separate sites based on the P25 system ID (sysid/wacn).

## 4. tr-engine

### Build

```bash
git clone https://github.com/yourusername/tr-engine.git
cd tr-engine
bash build.sh
```

Or manually:
```bash
go build -o tr-engine ./cmd/tr-engine
```

### Configure

```bash
cp sample.env .env
```

Edit `.env` with your values:

```env
# Required
DATABASE_URL=postgres://trengine:your_password_here@localhost:5432/trengine?sslmode=disable
MQTT_BROKER_URL=tcp://localhost:1883

# Match your TR plugin's topic prefix + wildcard
MQTT_TOPICS=trengine/#

# Optional
HTTP_ADDR=:8080
LOG_LEVEL=info
AUDIO_DIR=./audio
```

`MQTT_TOPICS` must match the topic prefixes from your TR plugin config. If all your TR topics share a common root (e.g. `topic: "trengine/feeds"`, `unit_topic: "trengine/units"`), a single wildcard like `trengine/#` covers everything. If they differ, comma-separate them: `MQTT_TOPICS=prefix1/#,prefix2/#`.

### Run

```bash
./tr-engine
```

tr-engine auto-loads `.env` from the current directory. You can also use CLI flags:

```bash
./tr-engine --listen :9090 --log-level debug --database-url postgres://...
```

### Verify

```bash
# Health check — shows database, MQTT, and TR instance status
curl http://localhost:8080/api/v1/health

# List systems (populated after TR connects and sends data)
curl http://localhost:8080/api/v1/systems

# List talkgroups
curl http://localhost:8080/api/v1/talkgroups?limit=10

# Watch live events
curl -N http://localhost:8080/api/v1/events/stream
```

### Web UI

tr-engine serves static files from a `web/` directory in dev mode. Open `http://localhost:8080/irc-radio-live.html` for an IRC-style live radio monitor.

## What happens on first run

1. tr-engine connects to PostgreSQL and MQTT
2. It subscribes to the configured MQTT topics
3. When trunk-recorder publishes its first messages, tr-engine auto-discovers systems and sites from the MQTT data
4. Talkgroups, units, and calls populate as radio traffic flows
5. The SSE event stream (`/api/v1/events/stream`) begins pushing events to connected clients

There's no manual system/site/talkgroup configuration needed — everything is discovered from the MQTT feed. Talkgroup names come from trunk-recorder's CSV import (configured in TR's `config.json`).

## Troubleshooting

**No systems appearing:** Check that trunk-recorder is running, connected to the MQTT broker, and publishing messages. Use `mosquitto_sub -t '#' -v` to verify messages are flowing.

**MQTT connection failing:** Verify `MQTT_BROKER_URL` matches your broker's address and port. Check firewall rules if the broker is remote.

**Database errors on startup:** Ensure `schema.sql` was loaded successfully. Check that the database user has ownership of all tables.

**Audio playback not working:** tr-engine serves audio from `AUDIO_DIR`. If trunk-recorder writes audio to a different path, either symlink or set `AUDIO_DIR` to match.
