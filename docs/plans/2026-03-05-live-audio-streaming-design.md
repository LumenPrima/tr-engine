# Live Audio Streaming Design

**Date:** 2026-03-05
**Status:** Approved
**Branch:** `feature/live-audio-streaming` (dev branch, merge to main when stable)

## Overview

Add real-time audio streaming to tr-engine so browser clients can listen to radio calls as they happen, similar to SDRTrunk or Unitrunker. trunk-recorder's existing simplestream plugin delivers decoded PCM audio over UDP; tr-engine encodes it to Opus and relays it to browsers via WebSocket.

## Architecture

```
┌─────────────────┐     UDP PCM + JSON header      ┌──────────────────────────────┐
│  trunk-recorder  │ ─────────────────────────────► │  tr-engine                   │
│  (simplestream)  │    8kHz int16, ~20ms chunks    │                              │
└─────────────────┘                                 │  ┌────────────────────────┐  │
                                                    │  │ SimplestreamSource     │  │
                                                    │  │ (UDP listener)         │  │
┌─────────────────┐     MQTT audio chunks (future)  │  └────────┬───────────────┘  │
│  trunk-recorder  │ ─────────────────────────────► │           ↓                  │
│  (MQTT plugin)   │                                │  ┌────────────────────────┐  │
└─────────────────┘                                 │  │ AudioRouter            │  │
                                                    │  │  identity resolution   │  │
                                                    │  │  multi-site dedup      │  │
                                                    │  │  Opus encode (per TG)  │  │
                                                    │  └────────┬───────────────┘  │
                                                    │           ↓                  │
                                                    │  ┌────────────────────────┐  │
                                                    │  │ AudioBus (pub/sub)     │  │
                                                    │  │  subscriber channels   │  │
                                                    │  │  per-client filtering  │  │
                                                    │  └────────┬───────────────┘  │
                                                    │           ↓                  │
                                                    │  ┌────────────────────────┐  │
                                                    │  │ WebSocket handler      │  │
                                                    │  │ /api/v1/audio/live     │  │
                                                    │  └────────────────────────┘  │
                                                    └──────────────────────────────┘
                                                                ↓
                                                    ┌──────────────────────────┐
                                                    │  Browser                 │
                                                    │  Opus decode (WebCodecs  │
                                                    │    or WASM fallback)     │
                                                    │  Web Audio API worklet   │
                                                    │  Per-TG mixing + volume  │
                                                    │  Compressor/expander     │
                                                    └──────────────────────────┘
```

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Listening mode | Multi-talkgroup monitor | Dispatch console style — hear multiple TGs simultaneously with per-TG volume/mute |
| Mixing | Client-side (Web Audio API) | More flexible, less server CPU, per-TG volume sliders |
| Latency target | ~200ms | Practical target, comfortable with Opus + jitter buffer |
| TR integration | Simplestream (UDP) now, transport-agnostic interface for MQTT later | Zero TR code changes, proven plugin, design supports future alternatives |
| Codec | Opus (encode once per TG, fan out) | 6-12 kbps vs 128 kbps raw PCM, browser-native, error resilient |
| Transport | Single WebSocket (binary audio + JSON control) | SSE is text-only and unidirectional — bad fit for binary audio + subscription changes |
| Scale | 10-50 concurrent listeners | Opus encoding + efficient fan-out, no exotic infrastructure needed |

## Transport-Agnostic Ingest

The audio ingest layer is abstracted behind an interface so the UDP/simplestream source can be swapped or supplemented later (e.g., MQTT audio plugin).

```go
type AudioChunk struct {
    SystemName string
    SystemID   int
    TGID       int
    UnitID     int
    Freq       float64
    Format     AudioFormat   // PCM or Opus
    SampleRate int           // 8000 (P25) or 16000 (analog)
    Data       []byte        // raw PCM int16 LE or Opus packet
    Timestamp  time.Time
}

type AudioFormat int
const (
    AudioFormatPCM  AudioFormat = iota
    AudioFormatOpus
)

type AudioChunkSource interface {
    Start(ctx context.Context, out chan<- AudioChunk) error
}
```

If a future TR plugin encodes Opus at the origin, chunks arrive with `Format: AudioFormatOpus` and the router passes them through without re-encoding.

## SimplestreamSource (UDP Listener)

Listens for UDP packets from trunk-recorder's simplestream plugin configured with `sendJSON: true`.

**Packet format from simplestream:**

```
[4-byte JSON length (little-endian)][JSON metadata][int16 PCM samples]
```

**JSON metadata fields used:**

```json
{
  "src": 305,
  "src_tag": "Engine 5",
  "talkgroup": 1001,
  "talkgroup_tag": "Fire Dispatch",
  "freq": 851250000,
  "short_name": "butco",
  "audio_sample_rate": 8000
}
```

**Resilience:** UDP is connectionless. When tr-engine is down, packets are silently dropped by the OS with zero impact on trunk-recorder. When tr-engine comes back up, audio resumes immediately with no handshake or reconnection. Each packet is self-contained.

## AudioRouter

Receives `AudioChunk`s from any source, handles identity resolution, multi-site deduplication, and Opus encoding.

### Identity Resolution

Maps `short_name` from simplestream to `system_id` using the same identity data the MQTT ingest pipeline maintains. Required for multi-site dedup and for tagging WebSocket frames with `system_id`.

### Multi-Site Deduplication

When multiple TR instances monitor the same P25 system from different sites (e.g., butco/warco), both send audio for the same talkgroup simultaneously. The router deduplicates:

```
AudioChunk arrives {shortName: "butco", tgid: 1001}
  → resolve shortName → site → system_id
  → lookup active stream key: (system_id, tgid)
  → not found → create encoder, claim stream for this site
  → found, same site → feed to existing encoder
  → found, different site → drop chunk (another site owns this stream)
```

First site to send audio for a `(system_id, tgid)` pair claims the stream. The claim releases after `STREAM_IDLE_TIMEOUT` (30s default).

```go
type activeStream struct {
    systemID  int
    tgid      int
    siteID    int
    shortName string
    encoder   *OpusEncoder
    lastChunk time.Time
}

// Key: "system_id:tgid"
activeStreams map[string]*activeStream
```

### Opus Encoding

One encoder per active talkgroup. Encoders are created on first audio chunk and destroyed after `STREAM_IDLE_TIMEOUT` with no audio.

- PCM chunks → encode to Opus → emit `AudioFrame` to AudioBus
- Opus chunks (future, pre-encoded at origin) → pass through to AudioBus
- Encoder bitrate: `STREAM_OPUS_BITRATE` (default 16kbps, good for voice)
- Frame size: 20ms (matches P25 IMBE frame rate)
- Per-TG sample rate from metadata (8kHz P25, 16kHz analog — each encoder configured independently)

## AudioBus (Pub/Sub)

Fan-out mechanism for encoded audio frames. Same pattern as the existing SSE `EventBus` but specialized for binary audio.

```go
type AudioFrame struct {
    SystemID  int
    TGID      int
    UnitID    int
    Seq       uint16    // per-tgid sequence number
    Timestamp uint32    // ms since bus start
    OpusData  []byte
}

type AudioBus struct {
    subscribers map[int]*AudioSubscriber
    mu          sync.RWMutex
}

type AudioSubscriber struct {
    ch     chan AudioFrame   // buffered (128 frames)
    filter AudioFilter
    mu     sync.Mutex
}

func (ab *AudioBus) Subscribe(filter AudioFilter) (<-chan AudioFrame, func())
```

Fan-out: read lock, iterate subscribers, check filter, non-blocking send. Dropped frames (slow consumer) produce a brief audio gap — acceptable for audio unlike metadata events.

## WebSocket Protocol

Single WebSocket endpoint: `GET /api/v1/audio/live?token=xxx`

### Client → Server (text frames, JSON)

```json
{"type": "subscribe", "tgids": [1001, 1002, 1005], "systems": [1]}

{"type": "subscribe", "tgids": [1001], "systems": [1]}

{"type": "unsubscribe"}
```

Subscription replaces the previous filter. Unsubscribe pauses all audio.

### Server → Client (text frames, JSON — call metadata)

```json
{"type": "call_start", "system_id": 1, "tgid": 1001, "unit_id": 305,
 "tg_alpha_tag": "Fire Dispatch", "freq": 851250000, "encrypted": false}

{"type": "call_end", "system_id": 1, "tgid": 1001, "duration": 4.2}

{"type": "keepalive", "active_streams": 3}
```

Keepalive every 15s (matches SSE interval).

### Server → Client (binary frames — audio)

```
Byte 0-1:   system_id   (uint16 BE)
Byte 2-5:   tgid        (uint32 BE)
Byte 6-9:   timestamp   (uint32 BE, ms since connection)
Byte 10-11: seq         (uint16 BE, per-tgid sequence)
Byte 12+:   opus_data   (variable, typically 20-80 bytes per 20ms frame)
```

12-byte header + Opus payload. Client uses `system_id + tgid` to route to the correct AudioWorkletNode. `seq` enables gap detection for error concealment.

### WebSocket Handler Internals

```
client connects → upgrade to WebSocket → subscribe to AudioBus (empty filter)
  → spawn reader goroutine: parse JSON control, update filter on AudioSubscriber
  → spawn writer goroutine: range over AudioFrame channel, write binary frames
  → on disconnect: cancel subscription, clean up
```

## Client-Side Architecture

### Audio Chain

```
Per active talkgroup:
  OpusDecoder → Float32 PCM → AudioWorklet (jitter buffer) → GainNode (per-TG volume)
                                                                  ↓
                                                     DynamicsCompressorNode (per-TG, optional)
                                                                  ↓
                                                              master bus
                                                                  ↓
                                                     DynamicsCompressorNode (master, default on)
                                                                  ↓
                                                           GainNode (master volume)
                                                                  ↓
                                                         AudioContext.destination
```

### Opus Decoding

Primary: `WebCodecs AudioDecoder` (Chrome 94+, Edge 94+, Safari 16.4+).
Fallback: `opus-decoder` WASM library (~300KB) for Firefox and older browsers.
Detection: `typeof AudioDecoder !== 'undefined'`.

### Jitter Buffer

Implemented in the `AudioWorklet` processor as a ring buffer.

- Target depth: 80ms (4 Opus frames at 20ms each)
- Underrun: output silence (small click, masked by compressor)
- Overflow (>150ms): skip ahead to 80ms (prevents latency drift)
- Budget: 20ms encode + 5ms UDP + 5ms WebSocket + 80ms buffer + 5ms decode = ~115ms typical

### Compressor/Expander

**Master compressor** (always on by default, toggleable): Normalizes levels across all talkgroups.

```js
master.threshold = -24;   // dBFS
master.knee = 12;         // soft knee
master.ratio = 4;         // 4:1
master.attack = 0.003;    // 3ms
master.release = 0.25;    // 250ms
```

**Per-TG compressor** (optional, off by default): Normalizes levels between units on the same TG.

```js
perTG.threshold = -20;
perTG.knee = 10;
perTG.ratio = 3;
perTG.attack = 0.003;
perTG.release = 0.15;
```

All settings persisted in localStorage.

### Reconnection

Auto-reconnect with exponential backoff: 1s, 2s, 4s, max 30s. Re-sends last subscription on reconnect.

## Configuration

Feature is completely inert when `STREAM_LISTEN` is not configured. No UDP socket, no encoder allocation. WebSocket endpoint returns 404.

### New Environment Variables

```env
STREAM_LISTEN=:9123              # UDP listen address for simplestream input
STREAM_SAMPLE_RATE=8000          # Default sample rate (8000 P25, 16000 analog)
STREAM_OPUS_BITRATE=16000        # Opus encoder bitrate in bps
STREAM_OPUS_FRAME_MS=20          # Opus frame size in ms
STREAM_MAX_CLIENTS=50            # Max concurrent WebSocket listeners
STREAM_IDLE_TIMEOUT=30s          # Tear down per-TG encoder after no audio
```

### New CLI Flag

```
--stream-listen :9123
```

### Simplestream Config (trunk-recorder side)

```json
{
  "name": "simplestream",
  "library": "libsimplestream.so",
  "streams": [{
    "address": "127.0.0.1",
    "port": 9123,
    "talkgroup": 0,
    "sendJSON": true,
    "shortName": ""
  }]
}
```

`talkgroup: 0` = all talkgroups. `sendJSON: true` = JSON metadata header on each packet. `shortName: ""` = all systems.

### Docker Compose

```yaml
tr-engine:
  ports:
    - "${STREAM_PORT:-9123}:9123/udp"
  environment:
    - STREAM_LISTEN=:9123
```

### Health Endpoint Addition

`GET /api/v1/health` includes:

```json
{
  "audio_stream": {
    "enabled": true,
    "listen": ":9123",
    "active_encoders": 3,
    "connected_clients": 7,
    "last_chunk_received": "2026-03-05T14:32:01Z"
  }
}
```

## Error Handling

### UDP Input

| Scenario | Handling |
|----------|----------|
| Malformed JSON header | Log warning, drop chunk |
| Unknown short_name | Drop chunk, log at debug level |
| No chunks arriving | Health endpoint shows stale `last_chunk_received` |
| Burst of audio | OS socket buffer handles short bursts; dropped packets = audio glitches |
| tr-engine restart | Packets silently dropped during downtime, resume immediately |

### Opus Encoder

| Scenario | Handling |
|----------|----------|
| Allocation fails | Log error, drop audio for that TG, others continue |
| Sample rate mismatch | Create encoder with actual rate from metadata |
| Idle TG | Destroy encoder after `STREAM_IDLE_TIMEOUT` |
| Mixed analog/P25 | Per-TG encoder at incoming sample rate |

### WebSocket

| Scenario | Handling |
|----------|----------|
| Streaming disabled | HTTP 404 before upgrade |
| Max clients exceeded | HTTP 503 |
| Slow client | Non-blocking send, drop frames. After 500 consecutive drops (~10s), close connection |
| Invalid JSON from client | Ignore, keep connection alive |
| Subscribe to nonexistent TG | Accept silently, no audio flows until TG becomes active |
| Encrypted call | `call_start` metadata includes `encrypted: true`, UI can mute/indicate |

### Graceful Shutdown

Extends existing SIGINT/SIGTERM handler:

1. Stop accepting new WebSocket connections
2. Send close frame (1001 Going Away) to all clients
3. Close UDP listener
4. Drain encoder goroutines
5. Continue with existing shutdown sequence

## New Go Packages

| Package | Source |
|---------|--------|
| `gorilla/websocket` | Already in module graph (indirect dep of Paho MQTT), promote to direct |
| `pion/opus` | Pure-Go Opus encoder, no cgo |

## File Layout

```
internal/audio/
  chunk.go           # AudioChunk, AudioFormat, AudioChunkSource interface
  simplestream.go    # SimplestreamSource (UDP listener)
  router.go          # AudioRouter (identity resolution, dedup, encoding)
  encoder.go         # Opus encoder wrapper
  bus.go             # AudioBus pub/sub fan-out

internal/api/
  audio_stream.go    # WebSocket handler for /api/v1/audio/live

web/
  audio-engine.js    # AudioEngine class (WebSocket, Opus decode, Web Audio)
  audio-worklet.js   # AudioWorklet processor (jitter buffer)
```

## Future Considerations

- **Unified TR plugin:** Replace both MQTT and simplestream with a single custom TR plugin that sends everything over one TCP connection. Deferred — requires building and maintaining a C++ plugin. The `AudioChunkSource` interface supports this without changes to the rest of the stack.
- **Pre-encoded Opus at origin:** Future MQTT or unified plugin could encode Opus in TR, reducing tr-engine CPU. `AudioFormat` tag on `AudioChunk` handles this — Opus chunks pass through the router without re-encoding.
- **Site preference:** Allow configuring preferred site per system for multi-site dedup, instead of first-to-arrive wins.
- **Signal-quality selection:** Choose the best site based on signal metrics. Requires extending simplestream or using a custom plugin that includes signal data.

## API Specification

The WebSocket endpoint and health extension should be documented in `openapi.yaml` when implementation begins.
