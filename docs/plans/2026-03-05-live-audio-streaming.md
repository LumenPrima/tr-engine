# Live Audio Streaming Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stream real-time radio audio from trunk-recorder's simplestream plugin through tr-engine to browser clients via WebSocket.

**Architecture:** UDP listener receives PCM audio chunks from simplestream, an AudioRouter demuxes by talkgroup and handles multi-site dedup, an AudioBus fans out frames to WebSocket subscribers, and browser clients decode and mix per-talkgroup audio via Web Audio API. Phase 1 sends raw PCM; Phase 2 adds Opus encoding.

**Tech Stack:** Go (gorilla/websocket already in module graph), Web Audio API + AudioWorklet, jj11hh/opus for Phase 2 Opus encoding (WASM, no cgo).

**Design doc:** `docs/plans/2026-03-05-live-audio-streaming-design.md`

**Branch:** `feature/live-audio-streaming`

---

### Task 1: Create Feature Branch and Add Config Fields

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Create the feature branch**

```bash
git checkout -b feature/live-audio-streaming
```

**Step 2: Write the failing test**

In `internal/config/config_test.go`, add a test for the new stream config fields:

```go
func TestStreamConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("STREAM_LISTEN", ":9123")
	t.Setenv("STREAM_OPUS_BITRATE", "24000")
	t.Setenv("STREAM_MAX_CLIENTS", "25")
	t.Setenv("STREAM_IDLE_TIMEOUT", "45s")

	cfg, err := Load("nonexistent.env")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StreamListen != ":9123" {
		t.Errorf("StreamListen = %q, want %q", cfg.StreamListen, ":9123")
	}
	if cfg.StreamOpusBitrate != 24000 {
		t.Errorf("StreamOpusBitrate = %d, want 24000", cfg.StreamOpusBitrate)
	}
	if cfg.StreamMaxClients != 25 {
		t.Errorf("StreamMaxClients = %d, want 25", cfg.StreamMaxClients)
	}
	if cfg.StreamIdleTimeout != 45*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 45s", cfg.StreamIdleTimeout)
	}
}

func TestStreamConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := Load("nonexistent.env")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StreamListen != "" {
		t.Errorf("StreamListen should be empty by default, got %q", cfg.StreamListen)
	}
	if cfg.StreamSampleRate != 8000 {
		t.Errorf("StreamSampleRate = %d, want 8000", cfg.StreamSampleRate)
	}
	if cfg.StreamOpusBitrate != 16000 {
		t.Errorf("StreamOpusBitrate = %d, want 16000", cfg.StreamOpusBitrate)
	}
	if cfg.StreamMaxClients != 50 {
		t.Errorf("StreamMaxClients = %d, want 50", cfg.StreamMaxClients)
	}
	if cfg.StreamIdleTimeout != 30*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 30s", cfg.StreamIdleTimeout)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestStreamConfig -v`
Expected: FAIL — fields don't exist on Config struct.

**Step 4: Add config fields**

In `internal/config/config.go`, add to the `Config` struct after the watch/upload section:

```go
// Live audio streaming (simplestream UDP ingest → WebSocket relay)
StreamListen      string        `env:"STREAM_LISTEN"`                              // UDP listen address, e.g. ":9123". Feature disabled if empty.
StreamSampleRate  int           `env:"STREAM_SAMPLE_RATE" envDefault:"8000"`        // Default PCM sample rate (8000 P25, 16000 analog)
StreamOpusBitrate int           `env:"STREAM_OPUS_BITRATE" envDefault:"16000"`      // Opus encoder bitrate in bps
StreamMaxClients  int           `env:"STREAM_MAX_CLIENTS" envDefault:"50"`          // Max concurrent WebSocket listeners
StreamIdleTimeout time.Duration `env:"STREAM_IDLE_TIMEOUT" envDefault:"30s"`        // Tear down per-TG encoder after idle
```

Also add `StreamListen` to the `Overrides` struct and the CLI flag application block:

```go
// In Overrides struct:
StreamListen string

// In applyOverrides or Load, after other flag applications:
if o.StreamListen != "" {
    cfg.StreamListen = o.StreamListen
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestStreamConfig -v`
Expected: PASS

**Step 6: Add --stream-listen CLI flag in main.go**

In `cmd/tr-engine/main.go`, add the flag alongside the existing CLI flags:

```go
flag.StringVar(&overrides.StreamListen, "stream-listen", "", "UDP listen address for simplestream audio")
```

**Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/tr-engine/main.go
git commit -m "feat(stream): add live audio streaming config fields"
```

---

### Task 2: AudioChunk Types and AudioChunkSource Interface

**Files:**
- Create: `internal/audio/stream.go`
- Create: `internal/audio/stream_test.go`

**Step 1: Write the types and interface**

Create `internal/audio/stream.go`:

```go
package audio

import (
	"context"
	"time"
)

// AudioFormat identifies the encoding of audio data in a chunk.
type AudioFormat int

const (
	AudioFormatPCM  AudioFormat = iota // 16-bit signed little-endian PCM
	AudioFormatOpus                    // Opus-encoded packet
)

func (f AudioFormat) String() string {
	switch f {
	case AudioFormatPCM:
		return "pcm"
	case AudioFormatOpus:
		return "opus"
	default:
		return "unknown"
	}
}

// AudioChunk is a single audio frame from any ingest source.
type AudioChunk struct {
	ShortName  string      // TR sys_name (e.g. "butco")
	SystemID   int         // resolved system ID (0 if unresolved)
	SiteID     int         // resolved site ID (0 if unresolved)
	TGID       int         // talkgroup ID
	UnitID     int         // source unit ID
	Freq       float64     // frequency in Hz
	TGAlphaTag string      // talkgroup alpha tag (from simplestream JSON)
	Format     AudioFormat // PCM or Opus
	SampleRate int         // samples per second (8000, 16000, etc.)
	Data       []byte      // audio payload
	Timestamp  time.Time   // when the chunk was received
}

// AudioFrame is an encoded audio frame ready for WebSocket delivery.
type AudioFrame struct {
	SystemID  int
	TGID      int
	UnitID    int
	Seq       uint16 // per-tgid sequence number
	Timestamp uint32 // ms since bus start
	Format    AudioFormat
	Data      []byte // PCM or Opus payload
}

// AudioChunkSource produces audio chunks from a transport (UDP, MQTT, etc.).
type AudioChunkSource interface {
	Start(ctx context.Context, out chan<- AudioChunk) error
}
```

**Step 2: Write a basic test to verify types compile and work**

Create `internal/audio/stream_test.go`:

```go
package audio

import (
	"testing"
	"time"
)

func TestAudioFormatString(t *testing.T) {
	tests := []struct {
		format AudioFormat
		want   string
	}{
		{AudioFormatPCM, "pcm"},
		{AudioFormatOpus, "opus"},
		{AudioFormat(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.format.String(); got != tt.want {
			t.Errorf("AudioFormat(%d).String() = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestAudioChunkFields(t *testing.T) {
	chunk := AudioChunk{
		ShortName:  "butco",
		TGID:       1001,
		UnitID:     305,
		Format:     AudioFormatPCM,
		SampleRate: 8000,
		Data:       make([]byte, 320), // 20ms at 8kHz 16-bit mono
		Timestamp:  time.Now(),
	}
	if chunk.ShortName != "butco" {
		t.Error("unexpected ShortName")
	}
	if len(chunk.Data) != 320 {
		t.Errorf("Data len = %d, want 320", len(chunk.Data))
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/audio/ -run TestAudio -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/audio/stream.go internal/audio/stream_test.go
git commit -m "feat(stream): add AudioChunk types and AudioChunkSource interface"
```

---

### Task 3: SimplestreamSource (UDP Listener)

**Files:**
- Create: `internal/audio/simplestream.go`
- Create: `internal/audio/simplestream_test.go`

**Step 1: Write the failing test**

Create `internal/audio/simplestream_test.go`:

```go
package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// buildSimplestreamPacket creates a UDP packet matching simplestream's sendJSON format:
// [4-byte JSON length LE][JSON metadata][int16 PCM samples]
func buildSimplestreamPacket(meta map[string]any, pcmSamples []int16) []byte {
	jsonBytes, _ := json.Marshal(meta)
	jsonLen := uint32(len(jsonBytes))

	buf := make([]byte, 4+len(jsonBytes)+len(pcmSamples)*2)
	binary.LittleEndian.PutUint32(buf[0:4], jsonLen)
	copy(buf[4:4+len(jsonBytes)], jsonBytes)

	offset := 4 + len(jsonBytes)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(buf[offset+i*2:], uint16(s))
	}
	return buf
}

func TestSimplestreamSourceReceivesChunks(t *testing.T) {
	// Start source on a random port
	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- src.Start(ctx, out) }()

	// Wait for listener to be ready
	var addr string
	for i := 0; i < 50; i++ {
		if a := src.Addr(); a != "" {
			addr = a
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("source did not start")
	}

	// Send a simplestream packet
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	meta := map[string]any{
		"src":               305,
		"talkgroup":         1001,
		"talkgroup_tag":     "Fire Dispatch",
		"freq":              851250000.0,
		"short_name":        "butco",
		"audio_sample_rate": 8000,
	}
	samples := make([]int16, 160) // 20ms at 8kHz
	for i := range samples {
		samples[i] = int16(i * 100) // non-zero data
	}

	pkt := buildSimplestreamPacket(meta, samples)
	conn.Write(pkt)

	// Read the chunk
	select {
	case chunk := <-out:
		if chunk.ShortName != "butco" {
			t.Errorf("ShortName = %q, want %q", chunk.ShortName, "butco")
		}
		if chunk.TGID != 1001 {
			t.Errorf("TGID = %d, want 1001", chunk.TGID)
		}
		if chunk.UnitID != 305 {
			t.Errorf("UnitID = %d, want 305", chunk.UnitID)
		}
		if chunk.Format != AudioFormatPCM {
			t.Errorf("Format = %v, want PCM", chunk.Format)
		}
		if chunk.SampleRate != 8000 {
			t.Errorf("SampleRate = %d, want 8000", chunk.SampleRate)
		}
		if len(chunk.Data) != 320 { // 160 samples * 2 bytes
			t.Errorf("Data len = %d, want 320", len(chunk.Data))
		}
		if chunk.TGAlphaTag != "Fire Dispatch" {
			t.Errorf("TGAlphaTag = %q, want %q", chunk.TGAlphaTag, "Fire Dispatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk")
	}

	cancel()
}

func TestSimplestreamSourceMalformedPacket(t *testing.T) {
	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go src.Start(ctx, out)

	var addr string
	for i := 0; i < 50; i++ {
		if a := src.Addr(); a != "" {
			addr = a
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("source did not start")
	}

	conn, _ := net.Dial("udp", addr)
	defer conn.Close()

	// Send garbage
	conn.Write([]byte{0x01, 0x02})

	// Should not produce a chunk (and should not crash)
	select {
	case chunk := <-out:
		t.Errorf("unexpected chunk from malformed packet: %+v", chunk)
	case <-time.After(200 * time.Millisecond):
		// Expected — no chunk produced
	}

	cancel()
}

func TestSimplestreamSourceSendTGIDMode(t *testing.T) {
	// Test the simpler 4-byte TGID prefix mode (no JSON)
	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go src.Start(ctx, out)

	var addr string
	for i := 0; i < 50; i++ {
		if a := src.Addr(); a != "" {
			addr = a
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, _ := net.Dial("udp", addr)
	defer conn.Close()

	// 4-byte TGID + PCM samples (sendTGID mode, no JSON)
	buf := make([]byte, 4+320)
	binary.LittleEndian.PutUint32(buf[0:4], 2001)
	// leave PCM as zeros
	conn.Write(buf)

	select {
	case chunk := <-out:
		if chunk.TGID != 2001 {
			t.Errorf("TGID = %d, want 2001", chunk.TGID)
		}
		if chunk.Format != AudioFormatPCM {
			t.Error("expected PCM format")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk")
	}

	cancel()
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/audio/ -run TestSimplestream -v`
Expected: FAIL — `NewSimplestreamSource` not defined.

**Step 3: Implement SimplestreamSource**

Create `internal/audio/simplestream.go`:

```go
package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// simplestreamMeta matches the JSON header sent by simplestream's sendJSON mode.
type simplestreamMeta struct {
	Src            int     `json:"src"`
	SrcTag         string  `json:"src_tag"`
	Talkgroup      int     `json:"talkgroup"`
	TalkgroupTag   string  `json:"talkgroup_tag"`
	Freq           float64 `json:"freq"`
	ShortName      string  `json:"short_name"`
	AudioSampleRate int    `json:"audio_sample_rate"`
}

// SimplestreamSource receives UDP packets from trunk-recorder's simplestream plugin.
type SimplestreamSource struct {
	listenAddr     string
	defaultSampleRate int
	log            zerolog.Logger

	mu   sync.Mutex
	addr string // actual bound address (after :0 resolution)
}

func NewSimplestreamSource(listenAddr string, defaultSampleRate int) *SimplestreamSource {
	return &SimplestreamSource{
		listenAddr:        listenAddr,
		defaultSampleRate: defaultSampleRate,
		log:               log.With().Str("component", "simplestream").Logger(),
	}
}

// Addr returns the actual listen address after binding. Empty until Start is called.
func (s *SimplestreamSource) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Start listens for UDP packets and sends parsed AudioChunks to out.
// Blocks until ctx is cancelled. Returns nil on clean shutdown.
func (s *SimplestreamSource) Start(ctx context.Context, out chan<- AudioChunk) error {
	addr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("resolve UDP address %q: %w", s.listenAddr, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen UDP %q: %w", s.listenAddr, err)
	}
	defer conn.Close()

	s.mu.Lock()
	s.addr = conn.LocalAddr().String()
	s.mu.Unlock()

	s.log.Info().Str("addr", s.addr).Msg("simplestream UDP listener started")

	// Close conn when context is cancelled to unblock ReadFromUDP
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 65536) // max UDP packet
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			s.log.Warn().Err(err).Msg("UDP read error")
			continue
		}
		if n < 4 {
			continue // too small to be useful
		}

		chunk, ok := s.parsePacket(buf[:n])
		if !ok {
			continue
		}

		select {
		case out <- chunk:
		case <-ctx.Done():
			return nil
		}
	}
}

// parsePacket tries to parse a simplestream UDP packet.
// Supports two formats:
//   - sendJSON: [4-byte JSON length LE][JSON][PCM samples]
//   - sendTGID: [4-byte TGID LE][PCM samples]
//
// Heuristic: if the first 4 bytes interpreted as a length produce a valid
// JSON object within the packet, it's sendJSON mode. Otherwise sendTGID.
func (s *SimplestreamSource) parsePacket(data []byte) (AudioChunk, bool) {
	if len(data) < 4 {
		return AudioChunk{}, false
	}

	possibleLen := binary.LittleEndian.Uint32(data[0:4])

	// Try sendJSON mode: possibleLen is the JSON length
	if possibleLen > 0 && possibleLen < uint32(len(data)-4) {
		jsonData := data[4 : 4+possibleLen]
		var meta simplestreamMeta
		if err := json.Unmarshal(jsonData, &meta); err == nil && meta.Talkgroup > 0 {
			pcmData := make([]byte, len(data)-4-int(possibleLen))
			copy(pcmData, data[4+possibleLen:])

			sampleRate := meta.AudioSampleRate
			if sampleRate == 0 {
				sampleRate = s.defaultSampleRate
			}

			return AudioChunk{
				ShortName:  meta.ShortName,
				TGID:       meta.Talkgroup,
				UnitID:     meta.Src,
				Freq:       meta.Freq,
				TGAlphaTag: meta.TalkgroupTag,
				Format:     AudioFormatPCM,
				SampleRate: sampleRate,
				Data:       pcmData,
				Timestamp:  time.Now(),
			}, true
		}
	}

	// Fallback: sendTGID mode — first 4 bytes are the TGID
	tgid := int(binary.LittleEndian.Uint32(data[0:4]))
	if tgid <= 0 || tgid > 65535 {
		s.log.Debug().Int("tgid", tgid).Msg("dropping packet with invalid TGID")
		return AudioChunk{}, false
	}

	pcmData := make([]byte, len(data)-4)
	copy(pcmData, data[4:])

	return AudioChunk{
		TGID:       tgid,
		Format:     AudioFormatPCM,
		SampleRate: s.defaultSampleRate,
		Data:       pcmData,
		Timestamp:  time.Now(),
	}, true
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/audio/ -run TestSimplestream -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/audio/simplestream.go internal/audio/simplestream_test.go
git commit -m "feat(stream): add SimplestreamSource UDP listener"
```

---

### Task 4: AudioBus (Pub/Sub Fan-Out)

**Files:**
- Create: `internal/audio/bus.go`
- Create: `internal/audio/bus_test.go`

**Step 1: Write the failing test**

Create `internal/audio/bus_test.go`:

```go
package audio

import (
	"testing"
	"time"
)

func TestAudioBusPublishToSubscriber(t *testing.T) {
	bus := NewAudioBus()

	filter := AudioFilter{TGIDs: []int{1001}}
	ch, cancel := bus.Subscribe(filter)
	defer cancel()

	frame := AudioFrame{
		SystemID: 1,
		TGID:     1001,
		UnitID:   305,
		Seq:      1,
		Format:   AudioFormatPCM,
		Data:     []byte{0x01, 0x02},
	}
	bus.Publish(frame)

	select {
	case got := <-ch:
		if got.TGID != 1001 {
			t.Errorf("TGID = %d, want 1001", got.TGID)
		}
		if got.Seq != 1 {
			t.Errorf("Seq = %d, want 1", got.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAudioBusFilterByTGID(t *testing.T) {
	bus := NewAudioBus()

	filter := AudioFilter{TGIDs: []int{1001}}
	ch, cancel := bus.Subscribe(filter)
	defer cancel()

	// Publish for a different TGID — should not be received
	bus.Publish(AudioFrame{TGID: 2002, Data: []byte{0x01}})

	select {
	case <-ch:
		t.Error("received frame for non-subscribed TGID")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestAudioBusFilterBySystem(t *testing.T) {
	bus := NewAudioBus()

	filter := AudioFilter{SystemIDs: []int{1}, TGIDs: []int{1001}}
	ch, cancel := bus.Subscribe(filter)
	defer cancel()

	// Wrong system
	bus.Publish(AudioFrame{SystemID: 2, TGID: 1001, Data: []byte{0x01}})

	select {
	case <-ch:
		t.Error("received frame for non-subscribed system")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	// Right system
	bus.Publish(AudioFrame{SystemID: 1, TGID: 1001, Data: []byte{0x02}})

	select {
	case got := <-ch:
		if got.SystemID != 1 {
			t.Errorf("SystemID = %d, want 1", got.SystemID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAudioBusEmptyFilterReceivesAll(t *testing.T) {
	bus := NewAudioBus()

	ch, cancel := bus.Subscribe(AudioFilter{})
	defer cancel()

	bus.Publish(AudioFrame{SystemID: 1, TGID: 1001, Data: []byte{0x01}})
	bus.Publish(AudioFrame{SystemID: 2, TGID: 2002, Data: []byte{0x02}})

	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("timed out on frame %d", i)
		}
	}
}

func TestAudioBusCancelUnsubscribes(t *testing.T) {
	bus := NewAudioBus()

	ch, cancel := bus.Subscribe(AudioFilter{})
	cancel()

	bus.Publish(AudioFrame{TGID: 1001, Data: []byte{0x01}})

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		// Also acceptable — channel closed, no more data
	}
}

func TestAudioBusSlowSubscriberDropsFrames(t *testing.T) {
	bus := NewAudioBus()

	filter := AudioFilter{}
	ch, cancel := bus.Subscribe(filter)
	defer cancel()

	// Flood the channel beyond its buffer capacity (256)
	for i := 0; i < 300; i++ {
		bus.Publish(AudioFrame{TGID: 1001, Seq: uint16(i), Data: []byte{0x01}})
	}

	// Should have received up to buffer capacity, rest dropped
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count > 256 {
		t.Errorf("received %d frames, expected at most 256 (buffer size)", count)
	}
	if count == 0 {
		t.Error("received 0 frames, expected some")
	}
}

func TestAudioBusMultipleSubscribers(t *testing.T) {
	bus := NewAudioBus()

	ch1, cancel1 := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel1()
	ch2, cancel2 := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel2()

	bus.Publish(AudioFrame{TGID: 1001, Data: []byte{0x01}})

	for i, ch := range []<-chan AudioFrame{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestAudioBusUpdateFilter(t *testing.T) {
	bus := NewAudioBus()

	ch, cancel := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel()

	// Update filter to a different TGID
	bus.UpdateFilter(ch, AudioFilter{TGIDs: []int{2002}})

	// Old TGID should not be received
	bus.Publish(AudioFrame{TGID: 1001, Data: []byte{0x01}})
	select {
	case <-ch:
		t.Error("received frame for old TGID after filter update")
	case <-time.After(100 * time.Millisecond):
	}

	// New TGID should be received
	bus.Publish(AudioFrame{TGID: 2002, Data: []byte{0x02}})
	select {
	case got := <-ch:
		if got.TGID != 2002 {
			t.Errorf("TGID = %d, want 2002", got.TGID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/audio/ -run TestAudioBus -v`
Expected: FAIL — `NewAudioBus`, `AudioFilter` not defined.

**Step 3: Implement AudioBus**

Create `internal/audio/bus.go`:

```go
package audio

import (
	"sync"
	"sync/atomic"
)

const audioSubscriberBuffer = 256

// AudioFilter controls which frames a subscriber receives.
// Empty slices mean "receive all" for that dimension.
// Non-empty slices are OR-matched within the dimension; dimensions are AND-ed.
type AudioFilter struct {
	SystemIDs []int
	TGIDs     []int
}

type audioSubscriber struct {
	ch     chan AudioFrame
	filter AudioFilter
	mu     sync.RWMutex
}

// AudioBus distributes encoded audio frames to subscribers.
type AudioBus struct {
	mu          sync.RWMutex
	subscribers map[uint64]*audioSubscriber
	nextID      atomic.Uint64
	chToID      map[<-chan AudioFrame]uint64 // reverse lookup for UpdateFilter
}

func NewAudioBus() *AudioBus {
	return &AudioBus{
		subscribers: make(map[uint64]*audioSubscriber),
		chToID:      make(map[<-chan AudioFrame]uint64),
	}
}

// Subscribe returns a channel that receives matching audio frames and a cancel function.
func (ab *AudioBus) Subscribe(filter AudioFilter) (<-chan AudioFrame, func()) {
	id := ab.nextID.Add(1)
	ch := make(chan AudioFrame, audioSubscriberBuffer)
	sub := &audioSubscriber{ch: ch, filter: filter}

	ab.mu.Lock()
	ab.subscribers[id] = sub
	ab.chToID[(<-chan AudioFrame)(ch)] = id
	ab.mu.Unlock()

	cancel := func() {
		ab.mu.Lock()
		delete(ab.subscribers, id)
		delete(ab.chToID, (<-chan AudioFrame)(ch))
		ab.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// UpdateFilter changes the filter for an existing subscriber identified by its channel.
func (ab *AudioBus) UpdateFilter(ch <-chan AudioFrame, filter AudioFilter) {
	ab.mu.RLock()
	id, ok := ab.chToID[ch]
	if !ok {
		ab.mu.RUnlock()
		return
	}
	sub := ab.subscribers[id]
	ab.mu.RUnlock()

	sub.mu.Lock()
	sub.filter = filter
	sub.mu.Unlock()
}

// Publish sends a frame to all matching subscribers. Non-blocking — drops frames
// for slow subscribers rather than blocking the publisher.
func (ab *AudioBus) Publish(frame AudioFrame) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	for _, sub := range ab.subscribers {
		sub.mu.RLock()
		match := matchesAudioFilter(frame, sub.filter)
		sub.mu.RUnlock()

		if match {
			select {
			case sub.ch <- frame:
			default:
				// Slow subscriber — drop frame
			}
		}
	}
}

// SubscriberCount returns the current number of subscribers.
func (ab *AudioBus) SubscriberCount() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.subscribers)
}

func matchesAudioFilter(frame AudioFrame, f AudioFilter) bool {
	if len(f.SystemIDs) > 0 {
		found := false
		for _, id := range f.SystemIDs {
			if frame.SystemID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(f.TGIDs) > 0 {
		found := false
		for _, id := range f.TGIDs {
			if frame.TGID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
```

**Step 4: Run tests**

Run: `go test ./internal/audio/ -run TestAudioBus -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/audio/bus.go internal/audio/bus_test.go
git commit -m "feat(stream): add AudioBus pub/sub fan-out"
```

---

### Task 5: AudioRouter (Identity Resolution, Multi-Site Dedup)

**Files:**
- Create: `internal/audio/router.go`
- Create: `internal/audio/router_test.go`

The AudioRouter sits between the chunk source and the AudioBus. It resolves shortName → systemID, deduplicates multi-site streams, and (in Phase 2) encodes PCM to Opus.

**Step 1: Write the failing test**

Create `internal/audio/router_test.go`:

```go
package audio

import (
	"context"
	"testing"
	"time"
)

// mockIdentityLookup simulates the identity resolver's shortName → system/site mapping.
type mockIdentityLookup struct {
	systems map[string]identityResult // shortName → result
}

type identityResult struct {
	systemID int
	siteID   int
}

func (m *mockIdentityLookup) LookupByShortName(shortName string) (systemID, siteID int, ok bool) {
	r, found := m.systems[shortName]
	if !found {
		return 0, 0, false
	}
	return r.systemID, r.siteID, true
}

func TestRouterResolvesIdentity(t *testing.T) {
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{
		systems: map[string]identityResult{
			"butco": {systemID: 1, siteID: 10},
		},
	}

	router := NewAudioRouter(bus, lookup, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	router.Input() <- AudioChunk{
		ShortName:  "butco",
		TGID:       1001,
		UnitID:     305,
		Format:     AudioFormatPCM,
		SampleRate: 8000,
		Data:       make([]byte, 320),
		Timestamp:  time.Now(),
	}

	select {
	case frame := <-ch:
		if frame.SystemID != 1 {
			t.Errorf("SystemID = %d, want 1", frame.SystemID)
		}
		if frame.TGID != 1001 {
			t.Errorf("TGID = %d, want 1001", frame.TGID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRouterDropsUnknownSystem(t *testing.T) {
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{systems: map[string]identityResult{}}

	router := NewAudioRouter(bus, lookup, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	router.Input() <- AudioChunk{
		ShortName: "unknown",
		TGID:      1001,
		Data:      make([]byte, 320),
		Timestamp: time.Now(),
	}

	select {
	case <-ch:
		t.Error("should not receive frame for unknown system")
	case <-time.After(200 * time.Millisecond):
		// Expected
	}
}

func TestRouterDeduplicatesMultiSite(t *testing.T) {
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{
		systems: map[string]identityResult{
			"butco": {systemID: 1, siteID: 10},
			"warco": {systemID: 1, siteID: 20}, // same system, different site
		},
	}

	router := NewAudioRouter(bus, lookup, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	// butco sends first — claims the stream
	router.Input() <- AudioChunk{ShortName: "butco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first frame")
	}

	// warco sends for the same TG — should be dropped
	router.Input() <- AudioChunk{ShortName: "warco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}

	select {
	case <-ch:
		t.Error("should not receive duplicate frame from second site")
	case <-time.After(200 * time.Millisecond):
		// Expected — dedup dropped it
	}

	// butco sends again — should still work
	router.Input() <- AudioChunk{ShortName: "butco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second frame from claimed site")
	}
}

func TestRouterIdleStreamRelease(t *testing.T) {
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{
		systems: map[string]identityResult{
			"butco": {systemID: 1, siteID: 10},
			"warco": {systemID: 1, siteID: 20},
		},
	}

	// Very short idle timeout for testing
	router := NewAudioRouter(bus, lookup, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	// butco claims TG 1001
	router.Input() <- AudioChunk{ShortName: "butco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}
	<-ch

	// Wait for idle timeout
	time.Sleep(200 * time.Millisecond)

	// Now warco should be able to claim it
	router.Input() <- AudioChunk{ShortName: "warco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}

	select {
	case <-ch:
		// Good — warco claimed after idle release
	case <-time.After(2 * time.Second):
		t.Fatal("warco should have claimed stream after idle timeout")
	}
}

func TestRouterActiveStreams(t *testing.T) {
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{
		systems: map[string]identityResult{
			"butco": {systemID: 1, siteID: 10},
		},
	}

	router := NewAudioRouter(bus, lookup, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	if n := router.ActiveStreamCount(); n != 0 {
		t.Errorf("ActiveStreamCount = %d before any audio, want 0", n)
	}

	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	router.Input() <- AudioChunk{ShortName: "butco", TGID: 1001, Data: make([]byte, 320), Timestamp: time.Now()}
	<-ch

	if n := router.ActiveStreamCount(); n != 1 {
		t.Errorf("ActiveStreamCount = %d after one TG, want 1", n)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/audio/ -run TestRouter -v`
Expected: FAIL — `NewAudioRouter`, `IdentityLookup` not defined.

**Step 3: Implement AudioRouter**

Create `internal/audio/router.go`:

```go
package audio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// IdentityLookup resolves a TR short_name to system/site IDs.
type IdentityLookup interface {
	LookupByShortName(shortName string) (systemID, siteID int, ok bool)
}

type activeStream struct {
	systemID  int
	siteID    int
	shortName string
	lastChunk time.Time
	seq       uint16
}

// AudioRouter receives AudioChunks, resolves identity, deduplicates multi-site
// streams, and publishes AudioFrames to the AudioBus.
type AudioRouter struct {
	bus         *AudioBus
	identity    IdentityLookup
	idleTimeout time.Duration
	log         zerolog.Logger

	input chan AudioChunk

	mu            sync.RWMutex
	activeStreams  map[string]*activeStream // "systemID:tgid" → stream
}

func NewAudioRouter(bus *AudioBus, identity IdentityLookup, idleTimeout time.Duration) *AudioRouter {
	return &AudioRouter{
		bus:          bus,
		identity:     identity,
		idleTimeout:  idleTimeout,
		log:          log.With().Str("component", "audio-router").Logger(),
		input:        make(chan AudioChunk, 256),
		activeStreams: make(map[string]*activeStream),
	}
}

// Input returns the channel to send AudioChunks to.
func (r *AudioRouter) Input() chan<- AudioChunk {
	return r.input
}

// ActiveStreamCount returns the number of currently active TG streams.
func (r *AudioRouter) ActiveStreamCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.activeStreams)
}

// Run processes audio chunks until ctx is cancelled.
func (r *AudioRouter) Run(ctx context.Context) {
	cleanup := time.NewTicker(5 * time.Second)
	defer cleanup.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case chunk := <-r.input:
			r.processChunk(chunk)
		case <-cleanup.C:
			r.cleanupIdle()
		}
	}
}

func (r *AudioRouter) processChunk(chunk AudioChunk) {
	// Resolve identity
	systemID := chunk.SystemID
	siteID := chunk.SiteID

	if systemID == 0 && chunk.ShortName != "" {
		sid, siteid, ok := r.identity.LookupByShortName(chunk.ShortName)
		if !ok {
			r.log.Debug().Str("short_name", chunk.ShortName).Msg("unknown system, dropping audio chunk")
			return
		}
		systemID = sid
		siteID = siteid
	}

	if systemID == 0 || chunk.TGID == 0 {
		return
	}

	key := fmt.Sprintf("%d:%d", systemID, chunk.TGID)

	r.mu.Lock()
	stream, exists := r.activeStreams[key]
	if exists {
		// Stream already claimed — check if same site
		if stream.siteID != siteID {
			r.mu.Unlock()
			return // Different site, drop (dedup)
		}
		stream.lastChunk = chunk.Timestamp
		stream.seq++
	} else {
		// New stream — claim for this site
		stream = &activeStream{
			systemID:  systemID,
			siteID:    siteID,
			shortName: chunk.ShortName,
			lastChunk: chunk.Timestamp,
			seq:       0,
		}
		r.activeStreams[key] = stream
	}
	seq := stream.seq
	r.mu.Unlock()

	// Build frame and publish
	frame := AudioFrame{
		SystemID: systemID,
		TGID:     chunk.TGID,
		UnitID:   chunk.UnitID,
		Seq:      seq,
		Format:   chunk.Format,
		Data:     chunk.Data,
	}

	r.bus.Publish(frame)
}

func (r *AudioRouter) cleanupIdle() {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, stream := range r.activeStreams {
		if now.Sub(stream.lastChunk) > r.idleTimeout {
			r.log.Debug().
				Str("key", key).
				Str("short_name", stream.shortName).
				Msg("releasing idle audio stream")
			delete(r.activeStreams, key)
		}
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/audio/ -run TestRouter -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/audio/router.go internal/audio/router_test.go
git commit -m "feat(stream): add AudioRouter with identity resolution and multi-site dedup"
```

---

### Task 6: WebSocket Handler

**Files:**
- Create: `internal/api/audio_stream.go`
- Create: `internal/api/audio_stream_test.go`
- Modify: `internal/api/middleware.go` (add `/audio/live` to ResponseTimeout exclusion)

**Dependencies:** `gorilla/websocket` — already in module graph as indirect dep, promote to direct.

**Step 1: Promote gorilla/websocket to direct dependency**

```bash
go get github.com/gorilla/websocket@v1.5.3
```

**Step 2: Write the failing test**

Create `internal/api/audio_stream_test.go`:

```go
package api

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/snarg/tr-engine/internal/audio"
)

type mockAudioStreamer struct {
	bus *audio.AudioBus
}

func (m *mockAudioStreamer) SubscribeAudio(filter audio.AudioFilter) (<-chan audio.AudioFrame, func()) {
	return m.bus.Subscribe(filter)
}

func (m *mockAudioStreamer) UpdateAudioFilter(ch <-chan audio.AudioFrame, filter audio.AudioFilter) {
	m.bus.UpdateFilter(ch, filter)
}

func (m *mockAudioStreamer) AudioStreamEnabled() bool { return true }
func (m *mockAudioStreamer) AudioStreamStatus() *AudioStreamStatusData {
	return &AudioStreamStatusData{
		Enabled:          true,
		ActiveEncoders:   0,
		ConnectedClients: m.bus.SubscriberCount(),
	}
}

func TestAudioStreamWebSocketConnect(t *testing.T) {
	bus := audio.NewAudioBus()
	streamer := &mockAudioStreamer{bus: bus}
	handler := NewAudioStreamHandler(streamer, 50)

	srv := httptest.NewServer(http.HandlerFunc(handler.HandleStream))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] // http → ws
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer ws.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("status = %d, want 101", resp.StatusCode)
	}
}

func TestAudioStreamSubscribeAndReceive(t *testing.T) {
	bus := audio.NewAudioBus()
	streamer := &mockAudioStreamer{bus: bus}
	handler := NewAudioStreamHandler(streamer, 50)

	srv := httptest.NewServer(http.HandlerFunc(handler.HandleStream))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Subscribe to TG 1001
	sub := map[string]any{"type": "subscribe", "tgids": []int{1001}, "systems": []int{1}}
	ws.WriteJSON(sub)

	// Give the subscription a moment to be processed
	time.Sleep(50 * time.Millisecond)

	// Publish an audio frame
	bus.Publish(audio.AudioFrame{
		SystemID: 1,
		TGID:     1001,
		UnitID:   305,
		Seq:      1,
		Format:   audio.AudioFormatPCM,
		Data:     []byte{0x01, 0x02, 0x03, 0x04},
	})

	// Read binary frame
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Errorf("message type = %d, want binary (%d)", msgType, websocket.BinaryMessage)
	}

	// Parse header: system_id(2) + tgid(4) + timestamp(4) + seq(2) = 12 bytes + payload
	if len(data) < 12 {
		t.Fatalf("frame too short: %d bytes", len(data))
	}
	gotSystemID := binary.BigEndian.Uint16(data[0:2])
	gotTGID := binary.BigEndian.Uint32(data[2:6])
	gotSeq := binary.BigEndian.Uint16(data[10:12])

	if gotSystemID != 1 {
		t.Errorf("system_id = %d, want 1", gotSystemID)
	}
	if gotTGID != 1001 {
		t.Errorf("tgid = %d, want 1001", gotTGID)
	}
	if gotSeq != 1 {
		t.Errorf("seq = %d, want 1", gotSeq)
	}
}

func TestAudioStreamCallMetadata(t *testing.T) {
	bus := audio.NewAudioBus()
	streamer := &mockAudioStreamer{bus: bus}
	handler := NewAudioStreamHandler(streamer, 50)

	srv := httptest.NewServer(http.HandlerFunc(handler.HandleStream))
	defer srv.Close()

	ws, _, _ := websocket.DefaultDialer.Dial("ws"+srv.URL[4:], nil)
	defer ws.Close()

	// The handler should send a keepalive within 15s. Test we can read text frames.
	ws.SetReadDeadline(time.Now().Add(20 * time.Second))
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("expected text message for keepalive, got type %d", msgType)
	}

	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("invalid JSON keepalive: %v", err)
	}
	if msg["type"] != "keepalive" {
		t.Errorf("type = %v, want keepalive", msg["type"])
	}
}

func TestAudioStreamMaxClients(t *testing.T) {
	bus := audio.NewAudioBus()
	streamer := &mockAudioStreamer{bus: bus}
	handler := NewAudioStreamHandler(streamer, 2) // max 2 clients

	srv := httptest.NewServer(http.HandlerFunc(handler.HandleStream))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]

	// Connect 2 clients — should succeed
	ws1, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer ws1.Close()
	ws2, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer ws2.Close()

	time.Sleep(50 * time.Millisecond)

	// Third client should be rejected
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("third client should have been rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestAudioStream -v`
Expected: FAIL — types not defined.

**Step 4: Add AudioStreamer interface to live_data.go**

In `internal/api/live_data.go`, add:

```go
// AudioStreamer provides live audio streaming capabilities.
type AudioStreamer interface {
	SubscribeAudio(filter audio.AudioFilter) (<-chan audio.AudioFrame, func())
	UpdateAudioFilter(ch <-chan audio.AudioFrame, filter audio.AudioFilter)
	AudioStreamEnabled() bool
	AudioStreamStatus() *AudioStreamStatusData
}

type AudioStreamStatusData struct {
	Enabled            bool   `json:"enabled"`
	Listen             string `json:"listen,omitempty"`
	ActiveEncoders     int    `json:"active_encoders"`
	ConnectedClients   int    `json:"connected_clients"`
	LastChunkReceived  string `json:"last_chunk_received,omitempty"`
}
```

Add import for `"github.com/snarg/tr-engine/internal/audio"`.

**Step 5: Implement WebSocket handler**

Create `internal/api/audio_stream.go`:

```go
package api

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/snarg/tr-engine/internal/audio"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true }, // CORS handled by middleware
}

// AudioStreamHandler serves the /api/v1/audio/live WebSocket endpoint.
type AudioStreamHandler struct {
	streamer   AudioStreamer
	maxClients int
	clients    atomic.Int32
	log        zerolog.Logger
}

func NewAudioStreamHandler(streamer AudioStreamer, maxClients int) *AudioStreamHandler {
	return &AudioStreamHandler{
		streamer:   streamer,
		maxClients: maxClients,
		log:        log.With().Str("component", "audio-ws").Logger(),
	}
}

func (h *AudioStreamHandler) Routes(r chi.Router) {
	r.Get("/audio/live", h.HandleStream)
}

// subscribeMsg is a client → server subscription control message.
type subscribeMsg struct {
	Type    string `json:"type"`    // "subscribe" or "unsubscribe"
	TGIDs   []int  `json:"tgids"`
	Systems []int  `json:"systems"`
}

func (h *AudioStreamHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	if !h.streamer.AudioStreamEnabled() {
		WriteError(w, http.StatusNotFound, "live audio streaming is not enabled")
		return
	}

	if int(h.clients.Load()) >= h.maxClients {
		WriteError(w, http.StatusServiceUnavailable, "max audio stream clients reached")
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	h.clients.Add(1)
	defer h.clients.Add(-1)
	defer conn.Close()

	connStart := time.Now()

	// Subscribe with empty filter (no audio until client sends subscribe)
	ch, cancel := h.streamer.SubscribeAudio(audio.AudioFilter{})
	defer cancel()

	// Reader goroutine: processes client control messages
	controlCh := make(chan subscribeMsg, 4)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub subscribeMsg
			if err := json.Unmarshal(msg, &sub); err != nil {
				continue
			}
			select {
			case controlCh <- sub:
			default:
			}
		}
	}()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	frameBuf := make([]byte, 12+8192) // header + max opus/pcm frame

	for {
		select {
		case <-doneCh:
			// Client disconnected
			return

		case sub := <-controlCh:
			switch sub.Type {
			case "subscribe":
				filter := audio.AudioFilter{
					SystemIDs: sub.Systems,
					TGIDs:     sub.TGIDs,
				}
				h.streamer.UpdateAudioFilter(ch, filter)
			case "unsubscribe":
				h.streamer.UpdateAudioFilter(ch, audio.AudioFilter{
					TGIDs: []int{-1}, // match nothing
				})
			}

		case frame, ok := <-ch:
			if !ok {
				return
			}
			// Build binary frame: system_id(2) + tgid(4) + timestamp(4) + seq(2) + data
			binary.BigEndian.PutUint16(frameBuf[0:2], uint16(frame.SystemID))
			binary.BigEndian.PutUint32(frameBuf[2:6], uint32(frame.TGID))
			elapsed := uint32(time.Since(connStart).Milliseconds())
			binary.BigEndian.PutUint32(frameBuf[6:10], elapsed)
			binary.BigEndian.PutUint16(frameBuf[10:12], frame.Seq)
			copy(frameBuf[12:], frame.Data)

			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, frameBuf[:12+len(frame.Data)]); err != nil {
				return
			}

		case <-keepalive.C:
			msg, _ := json.Marshal(map[string]any{
				"type":           "keepalive",
				"active_streams": h.streamer.AudioStreamStatus().ActiveEncoders,
			})
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}
```

**Step 6: Update ResponseTimeout middleware exclusion**

In `internal/api/middleware.go`, modify the ResponseTimeout function's skip check (around line 151):

```go
// Before:
if strings.HasSuffix(r.URL.Path, "/events/stream") ||
    strings.HasSuffix(r.URL.Path, "/audio") {

// After:
if strings.HasSuffix(r.URL.Path, "/events/stream") ||
    strings.HasSuffix(r.URL.Path, "/audio") ||
    strings.HasSuffix(r.URL.Path, "/audio/live") {
```

**Step 7: Run tests**

Run: `go test ./internal/api/ -run TestAudioStream -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/api/audio_stream.go internal/api/audio_stream_test.go internal/api/live_data.go internal/api/middleware.go go.mod go.sum
git commit -m "feat(stream): add WebSocket handler for live audio streaming"
```

---

### Task 7: Pipeline Integration and Wiring

**Files:**
- Modify: `internal/ingest/pipeline.go` (add AudioBus + AudioRouter + implement AudioStreamer)
- Modify: `internal/api/server.go` (wire AudioStreamHandler)
- Modify: `cmd/tr-engine/main.go` (start SimplestreamSource, pass to pipeline)

**Step 1: Add IdentityLookup implementation to Pipeline**

The Pipeline's `IdentityResolver` already has a cache of `shortName → system/site`. Add a method that satisfies the `audio.IdentityLookup` interface.

In `internal/ingest/pipeline.go`, add:

```go
// LookupByShortName satisfies audio.IdentityLookup for the AudioRouter.
func (p *Pipeline) LookupByShortName(shortName string) (systemID, siteID int, ok bool) {
	p.identity.mu.RLock()
	defer p.identity.mu.RUnlock()

	for _, ri := range p.identity.cache {
		if ri.SystemName == shortName {
			return ri.SystemID, ri.SiteID, true
		}
	}
	return 0, 0, false
}
```

Note: Check whether `IdentityResolver.cache` and `mu` are exported or accessible from Pipeline. If `identity` is a `*IdentityResolver` with unexported fields, add a `LookupByShortName` method directly on `IdentityResolver` instead, and have Pipeline delegate to it:

```go
// In identity.go:
func (ir *IdentityResolver) LookupByShortName(shortName string) (systemID, siteID int, ok bool) {
    ir.mu.RLock()
    defer ir.mu.RUnlock()
    for _, ri := range ir.cache {
        if ri.SystemName == shortName {
            return ri.SystemID, ri.SiteID, true
        }
    }
    return 0, 0, false
}

// In pipeline.go:
func (p *Pipeline) LookupByShortName(shortName string) (systemID, siteID int, ok bool) {
    return p.identity.LookupByShortName(shortName)
}
```

**Step 2: Add AudioBus and AudioRouter to Pipeline**

In `internal/ingest/pipeline.go`, add fields to the `Pipeline` struct:

```go
audioBus    *audio.AudioBus
audioRouter *audio.AudioRouter
```

In `PipelineOptions`, add:

```go
StreamListen      string
StreamSampleRate  int
StreamIdleTimeout time.Duration
```

In `NewPipeline`, conditionally create the audio infrastructure:

```go
var audioBus *audio.AudioBus
var audioRouter *audio.AudioRouter
if opts.StreamListen != "" {
    audioBus = audio.NewAudioBus()
    audioRouter = audio.NewAudioRouter(audioBus, identity, opts.StreamIdleTimeout)
}
```

Assign to Pipeline fields.

**Step 3: Start SimplestreamSource and AudioRouter in Pipeline.Start**

In `Pipeline.Start()`, add after existing goroutine launches:

```go
if p.audioRouter != nil {
    go p.audioRouter.Run(ctx)
}
```

The SimplestreamSource is started in `main.go` (see step 6) and feeds into `p.audioRouter.Input()`.

**Step 4: Implement AudioStreamer interface on Pipeline**

In `pipeline.go`, add:

```go
func (p *Pipeline) SubscribeAudio(filter audio.AudioFilter) (<-chan audio.AudioFrame, func()) {
    if p.audioBus == nil {
        ch := make(chan audio.AudioFrame)
        close(ch)
        return ch, func() {}
    }
    return p.audioBus.Subscribe(filter)
}

func (p *Pipeline) UpdateAudioFilter(ch <-chan audio.AudioFrame, filter audio.AudioFilter) {
    if p.audioBus != nil {
        p.audioBus.UpdateFilter(ch, filter)
    }
}

func (p *Pipeline) AudioStreamEnabled() bool {
    return p.audioBus != nil
}

func (p *Pipeline) AudioStreamStatus() *api.AudioStreamStatusData {
    if p.audioBus == nil {
        return &api.AudioStreamStatusData{Enabled: false}
    }
    return &api.AudioStreamStatusData{
        Enabled:          true,
        ActiveEncoders:   p.audioRouter.ActiveStreamCount(),
        ConnectedClients: p.audioBus.SubscriberCount(),
    }
}

// AudioRouterInput returns the chunk input channel, or nil if streaming is disabled.
func (p *Pipeline) AudioRouterInput() chan<- audio.AudioChunk {
    if p.audioRouter == nil {
        return nil
    }
    return p.audioRouter.Input()
}
```

**Step 5: Add AudioStreamer to ServerOptions and wire handler**

In `internal/api/server.go`, add to `ServerOptions`:

```go
AudioStreamer AudioStreamer
```

In `NewServer`, inside the authenticated route group, add:

```go
if opts.AudioStreamer != nil {
    NewAudioStreamHandler(opts.AudioStreamer, opts.Config.StreamMaxClients).Routes(r)
}
```

**Step 6: Wire everything in main.go**

In `cmd/tr-engine/main.go`, after `pipeline.Start(ctx)` and before `api.NewServer(...)`:

```go
// Start live audio streaming if configured
if cfg.StreamListen != "" {
    src := audio.NewSimplestreamSource(cfg.StreamListen, cfg.StreamSampleRate)
    if input := pipeline.AudioRouterInput(); input != nil {
        go func() {
            if err := src.Start(ctx, input); err != nil {
                logger.Error().Err(err).Msg("simplestream source failed")
            }
        }()
        logger.Info().Str("addr", cfg.StreamListen).Msg("live audio streaming enabled")
    }
}
```

Pass `AudioStreamer` to `ServerOptions`:

```go
srv := api.NewServer(api.ServerOptions{
    // ... existing fields ...
    AudioStreamer: pipeline, // Pipeline implements AudioStreamer
})
```

Pass stream config to `PipelineOptions`:

```go
pipeline := ingest.NewPipeline(ingest.PipelineOptions{
    // ... existing fields ...
    StreamListen:      cfg.StreamListen,
    StreamSampleRate:  cfg.StreamSampleRate,
    StreamIdleTimeout: cfg.StreamIdleTimeout,
})
```

**Step 7: Add cleanup to Pipeline.Stop()**

In `Pipeline.Stop()`, before `p.cancel()`:

```go
// audioRouter stops when ctx is cancelled (via p.cancel())
// No explicit stop needed — it exits its Run loop on ctx.Done()
```

**Step 8: Build and verify compilation**

```bash
go build ./cmd/tr-engine/
```

Expected: compiles successfully.

**Step 9: Commit**

```bash
git add internal/ingest/pipeline.go internal/ingest/identity.go internal/api/server.go cmd/tr-engine/main.go
git commit -m "feat(stream): wire audio streaming into pipeline and HTTP server"
```

---

### Task 8: Health Endpoint Extension

**Files:**
- Modify: `internal/api/health.go` (add audio stream status)

**Step 1: Find existing health handler and add audio stream status**

The health handler already returns system status. Add an `audio_stream` field to the response when streaming is enabled.

In the health handler, add the `AudioStreamer` dependency and include its status in the response:

```go
// In the health response struct, add:
AudioStream *AudioStreamStatusData `json:"audio_stream,omitempty"`

// In the handler body, add:
if h.audioStreamer != nil && h.audioStreamer.AudioStreamEnabled() {
    resp.AudioStream = h.audioStreamer.AudioStreamStatus()
}
```

Wire the `AudioStreamer` into the health handler via `ServerOptions` (same pattern as other dependencies).

**Step 2: Build and verify**

```bash
go build ./cmd/tr-engine/
```

**Step 3: Commit**

```bash
git add internal/api/health.go internal/api/server.go
git commit -m "feat(stream): add audio stream status to health endpoint"
```

---

### Task 9: OpenAPI Spec Update

**Files:**
- Modify: `openapi.yaml`

Add the WebSocket endpoint documentation and update the health response schema.

**Step 1: Add WebSocket endpoint**

Under `paths`, add `/api/v1/audio/live` with:
- GET operation
- Description of WebSocket upgrade
- Query parameter: `token` (for auth)
- Document the binary frame format and JSON control messages in the description
- 101 (Switching Protocols), 404 (not enabled), 503 (max clients)

**Step 2: Update health response schema**

Add `audio_stream` object to the health response with `enabled`, `listen`, `active_encoders`, `connected_clients`, `last_chunk_received`.

**Step 3: Commit**

```bash
git add openapi.yaml
git commit -m "docs: add live audio streaming WebSocket endpoint to OpenAPI spec"
```

---

### Task 10: Client-Side Audio Engine (Phase 1 — PCM)

**Files:**
- Create: `web/audio-worklet.js`
- Create: `web/audio-engine.js`

These are standalone JS modules usable by any page.

**Step 1: Create AudioWorklet processor**

Create `web/audio-worklet.js` — runs on the audio thread:

```js
// AudioWorklet processor with jitter buffer for live radio audio.
// Receives PCM samples via port.postMessage, outputs at AudioContext sample rate.

class RadioAudioProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = new Float32Array(16384); // ~2s ring buffer at 8kHz
    this.writePos = 0;
    this.readPos = 0;
    this.buffered = 0;
    this.inputSampleRate = 8000;
    this.resampleRatio = 1;
    this.resamplePos = 0;
    this.active = true;

    this.port.onmessage = (e) => {
      if (e.data.type === 'audio') {
        this.enqueueSamples(e.data.samples, e.data.sampleRate);
      } else if (e.data.type === 'stop') {
        this.active = false;
      }
    };
  }

  enqueueSamples(int16Array, sampleRate) {
    if (sampleRate && sampleRate !== this.inputSampleRate) {
      this.inputSampleRate = sampleRate;
      this.resampleRatio = sampleRate / sampleRate; // updated in process()
    }

    for (let i = 0; i < int16Array.length; i++) {
      this.buffer[this.writePos] = int16Array[i] / 32768.0;
      this.writePos = (this.writePos + 1) % this.buffer.length;
      this.buffered = Math.min(this.buffered + 1, this.buffer.length);
    }

    // Overflow protection: if buffered > 150ms worth, skip ahead to 80ms
    const maxSamples = Math.floor(this.inputSampleRate * 0.15);
    const targetSamples = Math.floor(this.inputSampleRate * 0.08);
    if (this.buffered > maxSamples) {
      const skip = this.buffered - targetSamples;
      this.readPos = (this.readPos + skip) % this.buffer.length;
      this.buffered -= skip;
    }
  }

  process(inputs, outputs, parameters) {
    if (!this.active) return false;

    const output = outputs[0][0]; // mono
    const ratio = this.inputSampleRate / sampleRate; // sampleRate is AudioContext rate (48000)

    for (let i = 0; i < output.length; i++) {
      if (this.buffered > 0) {
        // Nearest-neighbor resampling (good enough for voice)
        this.resamplePos += ratio;
        while (this.resamplePos >= 1 && this.buffered > 0) {
          this.resamplePos -= 1;
          this.readPos = (this.readPos + 1) % this.buffer.length;
          this.buffered--;
        }
        output[i] = this.buffer[this.readPos];
      } else {
        output[i] = 0; // silence on underrun
      }
    }

    return true;
  }
}

registerProcessor('radio-audio-processor', RadioAudioProcessor);
```

**Step 2: Create AudioEngine module**

Create `web/audio-engine.js` — main thread coordinator:

```js
// AudioEngine: WebSocket → PCM decode → Web Audio API with per-TG mixing and compression.
// Usage:
//   const engine = new AudioEngine('/api/v1/audio/live');
//   await engine.start();
//   engine.subscribe({ tgids: [1001, 1002], systems: [1] });
//   engine.setVolume(1001, 0.8);
//   engine.setMasterVolume(0.9);
//   engine.stop();

class AudioEngine {
  constructor(wsPath, options = {}) {
    this.wsPath = wsPath;
    this.options = {
      jitterBufferMs: options.jitterBufferMs || 80,
      reconnectMaxMs: options.reconnectMaxMs || 30000,
      ...options,
    };
    this.ws = null;
    this.audioCtx = null;
    this.masterGain = null;
    this.masterCompressor = null;
    this.tgNodes = new Map(); // tgid → { worklet, gain, compressor, compressorEnabled }
    this.reconnectDelay = 1000;
    this.lastSubscription = null;
    this.listeners = { call_start: [], call_end: [], status: [], error: [] };
  }

  on(event, fn) { (this.listeners[event] || []).push(fn); }
  emit(event, data) { (this.listeners[event] || []).forEach(fn => fn(data)); }

  async start() {
    // Create AudioContext (must be triggered by user gesture)
    this.audioCtx = new AudioContext({ sampleRate: 48000 });
    await this.audioCtx.audioWorklet.addModule('audio-worklet.js');

    // Master compressor → master gain → destination
    this.masterCompressor = this.audioCtx.createDynamicsCompressor();
    this.masterCompressor.threshold.value = -24;
    this.masterCompressor.knee.value = 12;
    this.masterCompressor.ratio.value = 4;
    this.masterCompressor.attack.value = 0.003;
    this.masterCompressor.release.value = 0.25;

    this.masterGain = this.audioCtx.createGain();
    this.masterCompressor.connect(this.masterGain);
    this.masterGain.connect(this.audioCtx.destination);

    // Load persisted settings
    this._loadSettings();

    this._connect();
  }

  stop() {
    if (this.ws) { this.ws.close(1000); this.ws = null; }
    this.tgNodes.forEach((nodes, tgid) => this._removeTG(tgid));
    if (this.audioCtx) { this.audioCtx.close(); this.audioCtx = null; }
  }

  subscribe(filter) {
    this.lastSubscription = filter;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'subscribe', ...filter }));
    }
  }

  unsubscribe() {
    this.lastSubscription = null;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'unsubscribe' }));
    }
  }

  setVolume(tgid, value) {
    const nodes = this.tgNodes.get(tgid);
    if (nodes) nodes.gain.gain.value = value;
    this._saveSetting(`vol_${tgid}`, value);
  }

  setMasterVolume(value) {
    if (this.masterGain) this.masterGain.gain.value = value;
    this._saveSetting('master_vol', value);
  }

  setMasterCompressorEnabled(enabled) {
    // Reconnect chain: TG outputs → compressor or direct → masterGain
    // Implementation: bypass by setting ratio to 1
    if (this.masterCompressor) {
      this.masterCompressor.ratio.value = enabled ? 4 : 1;
    }
    this._saveSetting('master_comp', enabled);
  }

  setTGCompressorEnabled(tgid, enabled) {
    const nodes = this.tgNodes.get(tgid);
    if (!nodes) return;
    nodes.compressorEnabled = enabled;
    nodes.compressor.ratio.value = enabled ? 3 : 1;
    this._saveSetting(`comp_${tgid}`, enabled);
  }

  // --- Internal ---

  _connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const token = window._authToken || '';
    const url = `${protocol}//${location.host}${this.wsPath}?token=${token}`;

    this.ws = new WebSocket(url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = () => {
      this.reconnectDelay = 1000;
      this.emit('status', { connected: true });
      if (this.lastSubscription) {
        this.subscribe(this.lastSubscription);
      }
    };

    this.ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        this._handleTextMessage(JSON.parse(event.data));
      } else {
        this._handleBinaryFrame(event.data);
      }
    };

    this.ws.onclose = (event) => {
      this.emit('status', { connected: false, code: event.code });
      if (event.code !== 1000) { // not intentional close
        setTimeout(() => this._connect(), this.reconnectDelay);
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.options.reconnectMaxMs);
      }
    };

    this.ws.onerror = () => {
      this.emit('error', { message: 'WebSocket error' });
    };
  }

  _handleTextMessage(msg) {
    switch (msg.type) {
      case 'call_start':
        this.emit('call_start', msg);
        break;
      case 'call_end':
        this.emit('call_end', msg);
        break;
      case 'keepalive':
        this.emit('status', { connected: true, active_streams: msg.active_streams });
        break;
    }
  }

  _handleBinaryFrame(buffer) {
    const view = new DataView(buffer);
    if (buffer.byteLength < 12) return;

    const systemId = view.getUint16(0);
    const tgid = view.getUint32(2);
    // timestamp at offset 6 (4 bytes) — available for latency measurement
    // seq at offset 10 (2 bytes) — available for gap detection

    const pcmData = new Int16Array(buffer, 12);

    // Get or create per-TG audio nodes
    if (!this.tgNodes.has(tgid)) {
      this._createTG(tgid);
    }

    const nodes = this.tgNodes.get(tgid);
    nodes.worklet.port.postMessage({
      type: 'audio',
      samples: pcmData,
      sampleRate: 8000, // TODO: detect from metadata or config
    });
    nodes.lastActivity = Date.now();
  }

  _createTG(tgid) {
    const worklet = new AudioWorkletNode(this.audioCtx, 'radio-audio-processor', {
      outputChannelCount: [1],
    });

    const compressor = this.audioCtx.createDynamicsCompressor();
    compressor.threshold.value = -20;
    compressor.knee.value = 10;
    compressor.ratio.value = 1; // disabled by default
    compressor.attack.value = 0.003;
    compressor.release.value = 0.15;

    const gain = this.audioCtx.createGain();
    const savedVol = this._loadSetting(`vol_${tgid}`);
    if (savedVol !== null) gain.gain.value = savedVol;

    const savedComp = this._loadSetting(`comp_${tgid}`);
    if (savedComp) compressor.ratio.value = 3;

    worklet.connect(compressor);
    compressor.connect(gain);
    gain.connect(this.masterCompressor);

    this.tgNodes.set(tgid, {
      worklet, compressor, gain,
      compressorEnabled: !!savedComp,
      lastActivity: Date.now(),
    });
  }

  _removeTG(tgid) {
    const nodes = this.tgNodes.get(tgid);
    if (!nodes) return;
    nodes.worklet.port.postMessage({ type: 'stop' });
    nodes.worklet.disconnect();
    nodes.compressor.disconnect();
    nodes.gain.disconnect();
    this.tgNodes.delete(tgid);
  }

  _saveSetting(key, value) {
    try {
      const settings = JSON.parse(localStorage.getItem('audio-engine') || '{}');
      settings[key] = value;
      localStorage.setItem('audio-engine', JSON.stringify(settings));
    } catch (e) { /* ignore */ }
  }

  _loadSetting(key) {
    try {
      const settings = JSON.parse(localStorage.getItem('audio-engine') || '{}');
      return settings[key] ?? null;
    } catch (e) { return null; }
  }

  _loadSettings() {
    const masterVol = this._loadSetting('master_vol');
    if (masterVol !== null && this.masterGain) this.masterGain.gain.value = masterVol;
    const masterComp = this._loadSetting('master_comp');
    if (masterComp === false && this.masterCompressor) this.masterCompressor.ratio.value = 1;
  }
}

// Export for use by pages
window.AudioEngine = AudioEngine;
```

**Step 3: Commit**

```bash
git add web/audio-worklet.js web/audio-engine.js
git commit -m "feat(stream): add client-side audio engine with Web Audio API"
```

---

### Task 11: Integration Test — End-to-End Audio Flow

**Files:**
- Create: `internal/audio/integration_test.go`

**Step 1: Write an integration test that exercises the full server-side pipeline**

```go
package audio

import (
	"context"
	"testing"
	"time"
)

func TestEndToEndPCMFlow(t *testing.T) {
	// Simulates: SimplestreamSource → AudioRouter → AudioBus → subscriber
	bus := NewAudioBus()
	lookup := &mockIdentityLookup{
		systems: map[string]identityResult{
			"butco": {systemID: 1, siteID: 10},
		},
	}

	router := NewAudioRouter(bus, lookup, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	// Subscribe to all audio
	ch, cancelSub := bus.Subscribe(AudioFilter{})
	defer cancelSub()

	// Simulate 5 consecutive 20ms audio chunks (100ms of audio)
	for i := 0; i < 5; i++ {
		router.Input() <- AudioChunk{
			ShortName:  "butco",
			TGID:       1001,
			UnitID:     305,
			Format:     AudioFormatPCM,
			SampleRate: 8000,
			Data:       make([]byte, 320), // 160 samples * 2 bytes
			Timestamp:  time.Now(),
		}
	}

	// Should receive all 5 frames with incrementing sequence numbers
	for i := 0; i < 5; i++ {
		select {
		case frame := <-ch:
			if frame.TGID != 1001 {
				t.Errorf("frame %d: TGID = %d, want 1001", i, frame.TGID)
			}
			if int(frame.Seq) != i {
				t.Errorf("frame %d: Seq = %d, want %d", i, frame.Seq, i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for frame %d", i)
		}
	}

	// Active stream count should be 1
	if n := router.ActiveStreamCount(); n != 1 {
		t.Errorf("ActiveStreamCount = %d, want 1", n)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/audio/ -run TestEndToEnd -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/audio/integration_test.go
git commit -m "test(stream): add end-to-end integration test for audio pipeline"
```

---

### Task 12 (Phase 2): Opus Encoding

**Files:**
- Create: `internal/audio/encoder.go`
- Create: `internal/audio/encoder_test.go`
- Modify: `internal/audio/router.go` (integrate encoder)

**Dependency:** `go get github.com/jj11hh/opus`

This task adds Opus encoding to the AudioRouter. When a PCM chunk arrives, it gets encoded to Opus before publishing to the AudioBus. Opus chunks from future pre-encoding sources pass through unchanged.

**Step 1: Add dependency**

```bash
go get github.com/jj11hh/opus
```

**Step 2: Write failing test for encoder wrapper**

Create `internal/audio/encoder_test.go`:

```go
package audio

import (
	"testing"
)

func TestOpusEncoderEncode(t *testing.T) {
	enc, err := NewOpusEncoder(8000, 1, 16000) // 8kHz mono, 16kbps
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	// 20ms of silence at 8kHz = 160 samples = 320 bytes PCM
	pcm := make([]byte, 320)

	opus, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}

	if len(opus) == 0 {
		t.Error("Opus output is empty")
	}
	if len(opus) > 320 {
		t.Errorf("Opus output (%d bytes) larger than PCM input — compression failed?", len(opus))
	}
}
```

**Step 3: Implement encoder wrapper**

Create `internal/audio/encoder.go`:

```go
package audio

import (
	"fmt"

	"github.com/jj11hh/opus"
)

// OpusEncoder wraps the Opus encoder for voice audio.
type OpusEncoder struct {
	enc        *opus.Encoder
	sampleRate int
	channels   int
	frameSize  int // samples per frame (sampleRate * frameMs / 1000)
}

func NewOpusEncoder(sampleRate, channels, bitrate int) (*OpusEncoder, error) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}
	if err := enc.SetBitrate(bitrate); err != nil {
		return nil, fmt.Errorf("set bitrate: %w", err)
	}

	return &OpusEncoder{
		enc:        enc,
		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  sampleRate * 20 / 1000, // 20ms frames
	}, nil
}

// Encode takes PCM int16 LE data and returns an Opus packet.
func (e *OpusEncoder) Encode(pcmData []byte) ([]byte, error) {
	// Convert bytes to int16 slice
	nSamples := len(pcmData) / 2
	pcm := make([]int16, nSamples)
	for i := 0; i < nSamples; i++ {
		pcm[i] = int16(pcmData[i*2]) | int16(pcmData[i*2+1])<<8
	}

	buf := make([]byte, 1024) // max Opus packet
	n, err := e.enc.Encode(pcm, buf)
	if err != nil {
		return nil, fmt.Errorf("opus encode: %w", err)
	}
	return buf[:n], nil
}

func (e *OpusEncoder) Close() {
	// jj11hh/opus may not need explicit cleanup, but good practice
}
```

**Step 4: Integrate into AudioRouter**

In `internal/audio/router.go`, modify `processChunk` to encode PCM chunks:

```go
// In the AudioRouter struct, add:
opusBitrate int
encoders    map[string]*OpusEncoder // "systemID:tgid" → encoder

// In NewAudioRouter, add opusBitrate parameter and initialize encoders map.

// In processChunk, after dedup check and before bus.Publish:
if chunk.Format == AudioFormatPCM && r.opusBitrate > 0 {
    enc, ok := r.encoders[key]
    if !ok {
        var err error
        enc, err = NewOpusEncoder(chunk.SampleRate, 1, r.opusBitrate)
        if err != nil {
            r.log.Error().Err(err).Str("key", key).Msg("failed to create Opus encoder")
            // Fall through with PCM
        } else {
            r.encoders[key] = enc
        }
    }
    if enc != nil {
        opusData, err := enc.Encode(chunk.Data)
        if err != nil {
            r.log.Warn().Err(err).Str("key", key).Msg("Opus encode failed, sending PCM")
        } else {
            chunk.Data = opusData
            chunk.Format = AudioFormatOpus
        }
    }
}
```

Also clean up encoders in `cleanupIdle`.

**Step 5: Run all tests**

```bash
go test ./internal/audio/ -v
```

**Step 6: Commit**

```bash
git add internal/audio/encoder.go internal/audio/encoder_test.go internal/audio/router.go go.mod go.sum
git commit -m "feat(stream): add Opus encoding via jj11hh/opus (WASM, no cgo)"
```

---

### Task 13: Update Client for Opus Decoding

**Files:**
- Modify: `web/audio-engine.js`

Add Opus decoding support to the audio engine using WebCodecs `AudioDecoder` with WASM fallback.

**Step 1: Add WebCodecs Opus decoder to `_handleBinaryFrame`**

When the server sends Opus frames, detect the format and decode before feeding the worklet. For Phase 1 (PCM), this is a no-op — PCM goes directly to the worklet. For Opus, use `AudioDecoder` to decode to PCM first.

The format detection can be based on frame size heuristic (PCM frames are always `numSamples * 2` bytes, Opus frames are much smaller) or a server-sent config message.

**Step 2: Commit**

```bash
git add web/audio-engine.js
git commit -m "feat(stream): add Opus decoding support (WebCodecs + WASM fallback)"
```

---

## Execution Order Summary

| Task | Component | Dependencies |
|------|-----------|-------------|
| 1 | Config fields | None |
| 2 | AudioChunk types | None |
| 3 | SimplestreamSource | Task 2 |
| 4 | AudioBus | Task 2 |
| 5 | AudioRouter | Tasks 2, 4 |
| 6 | WebSocket handler | Tasks 2, 4 |
| 7 | Pipeline wiring | Tasks 3, 4, 5, 6 |
| 8 | Health endpoint | Task 7 |
| 9 | OpenAPI spec | Task 6 |
| 10 | Client JS (PCM) | None (parallel with server tasks) |
| 11 | Integration test | Tasks 4, 5 |
| 12 | Opus encoding (Phase 2) | Tasks 5, 7 |
| 13 | Client Opus decode (Phase 2) | Tasks 10, 12 |

Tasks 1-2 can run in parallel. Tasks 3-4 can run in parallel. Task 10 can run in parallel with all server tasks.
