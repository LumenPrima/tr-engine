package ingest

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/transcribe"
)

// stubProvider is a minimal STT provider for routing tests.
type stubProvider struct{ name string }

func (s stubProvider) Transcribe(context.Context, string, transcribe.TranscribeOpts) (*transcribe.Response, error) {
	return nil, nil
}
func (s stubProvider) Name() string  { return s.name }
func (s stubProvider) Model() string { return s.name + "-model" }

func newPoolWithProvider(name string) *transcribe.WorkerPool {
	return transcribe.NewWorkerPool(transcribe.WorkerPoolOptions{
		Provider:    stubProvider{name: name},
		Workers:     1,
		QueueSize:   10,
		MinDuration: 1,
		MaxDuration: 300,
		Log:         zerolog.Nop(),
	})
}

func TestRouteTranscriber(t *testing.T) {
	imbe := newPoolWithProvider("imbe")
	whisper := newPoolWithProvider("whisper")
	eleven := newPoolWithProvider("elevenlabs")

	tests := []struct {
		name      string
		primary   *transcribe.WorkerPool
		fallback  *transcribe.WorkerPool
		audioPath string
		want      *transcribe.WorkerPool
	}{
		{"whisper primary always primary", whisper, nil, "calls/a.m4a", whisper},
		{"whisper primary ignores dvcf path", whisper, nil, "calls/a.dvcf", whisper},
		{"imbe only dvcf", imbe, nil, "calls/a.dvcf", imbe},
		{"imbe only uppercase dvcf", imbe, nil, "calls/a.DVCF", imbe},
		{"imbe only skips m4a", imbe, nil, "calls/a.m4a", nil},
		{"imbe only skips empty", imbe, nil, "", nil},
		{"imbe+fallback dvcf to imbe", imbe, whisper, "calls/a.dvcf", imbe},
		{"imbe+fallback m4a to fallback", imbe, whisper, "calls/a.m4a", whisper},
		{"imbe+fallback empty to fallback", imbe, whisper, "", whisper},
		{"imbe+elevenlabs m4a", imbe, eleven, "x.flac", eleven},
		{"nil primary", nil, whisper, "a.m4a", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeTranscriber(tt.primary, tt.fallback, tt.audioPath)
			if got != tt.want {
				var gotName, wantName string
				if got != nil {
					gotName = got.ProviderName()
				} else {
					gotName = "<nil>"
				}
				if tt.want != nil {
					wantName = tt.want.ProviderName()
				} else {
					wantName = "<nil>"
				}
				t.Fatalf("routeTranscriber(...) = %s, want %s", gotName, wantName)
			}
		})
	}
}

func TestIsDvcfPath(t *testing.T) {
	if !isDvcfPath("foo.dvcf") {
		t.Error("expected .dvcf true")
	}
	if !isDvcfPath("foo.DVCF") {
		t.Error("expected .DVCF true")
	}
	if isDvcfPath("foo.m4a") {
		t.Error("expected .m4a false")
	}
	if isDvcfPath("") {
		t.Error("expected empty false")
	}
}

func TestBackfillShouldSkipForProvider_DualModeAware(t *testing.T) {
	// Pure skip helper remains imbe-only without fallback knowledge.
	if !backfillShouldSkipForProvider("imbe", "a.m4a") {
		t.Error("imbe should skip m4a")
	}
	if backfillShouldSkipForProvider("imbe", "a.dvcf") {
		t.Error("imbe should accept dvcf")
	}
}
