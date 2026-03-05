package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// buildSimplestreamPacket builds a simplestream sendJSON packet:
// [4-byte JSON length LE][JSON metadata][int16 PCM samples]
func buildSimplestreamPacket(meta map[string]any, pcmSamples []int16) []byte {
	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		panic(err)
	}
	// 4-byte JSON length (little-endian) + JSON + PCM samples (2 bytes each)
	buf := make([]byte, 4+len(jsonBytes)+len(pcmSamples)*2)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(jsonBytes)))
	copy(buf[4:4+len(jsonBytes)], jsonBytes)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(buf[4+len(jsonBytes)+i*2:], uint16(s))
	}
	return buf
}

func TestSimplestreamSourceReceivesChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Start(ctx, out)
	}()

	// Wait for Addr() to be non-empty (source has bound)
	for i := 0; i < 50; i++ {
		if src.Addr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if src.Addr() == "" {
		t.Fatal("source did not bind within 500ms")
	}

	// Build and send a valid sendJSON packet
	meta := map[string]any{
		"src":               305,
		"src_tag":           "Engine 5",
		"talkgroup":         1001,
		"talkgroup_tag":     "Fire Dispatch",
		"freq":              851250000.0,
		"short_name":        "butco",
		"audio_sample_rate": 8000,
	}
	pcm := []int16{100, -200, 300, -400}
	pkt := buildSimplestreamPacket(meta, pcm)

	addr, err := net.ResolveUDPAddr("udp", src.Addr())
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for chunk
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
		if len(chunk.Data) != len(pcm)*2 {
			t.Errorf("Data len = %d, want %d", len(chunk.Data), len(pcm)*2)
		}
		if chunk.TGAlphaTag != "Fire Dispatch" {
			t.Errorf("TGAlphaTag = %q, want %q", chunk.TGAlphaTag, "Fire Dispatch")
		}
		if chunk.Freq != 851250000.0 {
			t.Errorf("Freq = %f, want 851250000.0", chunk.Freq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk")
	}
}

func TestSimplestreamSourceMalformedPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Start(ctx, out)
	}()

	// Wait for bind
	for i := 0; i < 50; i++ {
		if src.Addr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if src.Addr() == "" {
		t.Fatal("source did not bind within 500ms")
	}

	// Send 2 bytes of garbage
	addr, err := net.ResolveUDPAddr("udp", src.Addr())
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{0xFF, 0xFE}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify no chunk is produced
	select {
	case chunk := <-out:
		t.Fatalf("unexpected chunk received: %+v", chunk)
	case <-time.After(200 * time.Millisecond):
		// Expected: no chunk produced
	}

	// Verify source is still alive by sending a valid packet
	meta := map[string]any{
		"talkgroup":         999,
		"short_name":        "test",
		"audio_sample_rate": 8000,
	}
	pkt := buildSimplestreamPacket(meta, []int16{1, 2})
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("write valid: %v", err)
	}

	select {
	case chunk := <-out:
		if chunk.TGID != 999 {
			t.Errorf("TGID = %d, want 999", chunk.TGID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("source appears to have crashed after malformed packet")
	}
}

func TestSimplestreamSourceSendTGIDMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := NewSimplestreamSource("127.0.0.1:0", 8000)
	out := make(chan AudioChunk, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Start(ctx, out)
	}()

	// Wait for bind
	for i := 0; i < 50; i++ {
		if src.Addr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if src.Addr() == "" {
		t.Fatal("source did not bind within 500ms")
	}

	// Build a sendTGID packet: [4-byte TGID LE][int16 PCM samples]
	pcm := []int16{500, -500, 1000, -1000, 1500}
	buf := make([]byte, 4+len(pcm)*2)
	binary.LittleEndian.PutUint32(buf[0:4], 2001) // TGID = 2001
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(buf[4+i*2:], uint16(s))
	}

	addr, err := net.ResolveUDPAddr("udp", src.Addr())
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case chunk := <-out:
		if chunk.TGID != 2001 {
			t.Errorf("TGID = %d, want 2001", chunk.TGID)
		}
		if chunk.Format != AudioFormatPCM {
			t.Errorf("Format = %v, want PCM", chunk.Format)
		}
		if chunk.SampleRate != 8000 {
			t.Errorf("SampleRate = %d, want 8000", chunk.SampleRate)
		}
		if len(chunk.Data) != len(pcm)*2 {
			t.Errorf("Data len = %d, want %d", len(chunk.Data), len(pcm)*2)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk")
	}
}
