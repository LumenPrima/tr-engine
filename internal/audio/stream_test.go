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
