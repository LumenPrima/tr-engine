package audio

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
)

func TestPCMPassthroughEncode(t *testing.T) {
	enc := NewPCMPassthroughEncoder(8000)

	input := []byte{0x01, 0x02, 0x03, 0x04}
	out, format, err := enc.Encode(input)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if format != AudioFormatPCM {
		t.Errorf("format = %v, want PCM", format)
	}
	if !bytes.Equal(out, input) {
		t.Errorf("output = %v, want %v", out, input)
	}
}

func TestPCMPassthroughSampleRate(t *testing.T) {
	for _, rate := range []int{8000, 16000, 48000} {
		enc := NewPCMPassthroughEncoder(rate)
		if enc.SampleRate() != rate {
			t.Errorf("SampleRate() = %d, want %d", enc.SampleRate(), rate)
		}
	}
}

func TestPCMPassthroughCloseIsNoop(t *testing.T) {
	enc := NewPCMPassthroughEncoder(8000)
	enc.Close() // should not panic
}

func TestPCMPassthroughEmptyData(t *testing.T) {
	enc := NewPCMPassthroughEncoder(8000)

	out, format, err := enc.Encode([]byte{})
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if format != AudioFormatPCM {
		t.Errorf("format = %v, want PCM", format)
	}
	if len(out) != 0 {
		t.Errorf("output length = %d, want 0", len(out))
	}
}

func TestPCMPassthroughNilData(t *testing.T) {
	enc := NewPCMPassthroughEncoder(8000)

	out, format, err := enc.Encode(nil)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if format != AudioFormatPCM {
		t.Errorf("format = %v, want PCM", format)
	}
	if out != nil {
		t.Errorf("output = %v, want nil", out)
	}
}

func TestNewEncoderZeroBitrate(t *testing.T) {
	log := zerolog.Nop()
	enc := NewEncoder(8000, 0, log)

	// Should return a PCM passthrough encoder
	out, format, err := enc.Encode([]byte{0xAA, 0xBB})
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if format != AudioFormatPCM {
		t.Errorf("format = %v, want PCM", format)
	}
	if !bytes.Equal(out, []byte{0xAA, 0xBB}) {
		t.Errorf("output = %v, want [0xAA, 0xBB]", out)
	}
	if enc.SampleRate() != 8000 {
		t.Errorf("SampleRate() = %d, want 8000", enc.SampleRate())
	}
}

func TestNewEncoderPositiveBitrateFallback(t *testing.T) {
	// Opus requested but not available — should fall back to PCM passthrough
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.WarnLevel)

	enc := NewEncoder(16000, 24000, log)

	out, format, err := enc.Encode([]byte{0x01})
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if format != AudioFormatPCM {
		t.Errorf("format = %v, want PCM (fallback)", format)
	}
	if !bytes.Equal(out, []byte{0x01}) {
		t.Errorf("output = %v, want [0x01]", out)
	}

	// Verify a warning was logged
	logOutput := buf.String()
	if logOutput == "" {
		t.Error("expected warning log about Opus not available, got empty")
	}
}

func TestPCMPassthroughDataIntegrity(t *testing.T) {
	enc := NewPCMPassthroughEncoder(8000)

	// Simulate a realistic 20ms PCM frame (160 samples * 2 bytes = 320 bytes)
	input := make([]byte, 320)
	for i := range input {
		input[i] = byte(i % 256)
	}

	out, _, err := enc.Encode(input)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if !bytes.Equal(out, input) {
		t.Error("output does not match input for realistic PCM frame")
	}
}
