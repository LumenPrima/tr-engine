package ingest

import "testing"

func TestBackfillShouldSkipForProvider(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		audioPath string
		want      bool
	}{
		{"whisper accepts m4a", "whisper", "calls/2026/05/call.m4a", false},
		{"whisper accepts empty path", "whisper", "", false},
		{"imbe accepts dvcf", "imbe", "calls/2026/05/call.dvcf", false},
		{"imbe accepts uppercase dvcf", "imbe", "calls/2026/05/call.DVCF", false},
		{"imbe skips m4a", "imbe", "calls/2026/05/call.m4a", true},
		{"imbe skips empty path", "imbe", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backfillShouldSkipForProvider(tt.provider, tt.audioPath)
			if got != tt.want {
				t.Fatalf("backfillShouldSkipForProvider(%q, %q) = %v, want %v", tt.provider, tt.audioPath, got, tt.want)
			}
		})
	}
}
