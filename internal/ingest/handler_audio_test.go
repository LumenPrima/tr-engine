package ingest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// ── buildSrcFreqJSON ─────────────────────────────────────────────────

func TestBuildSrcFreqJSON_EmptyInputs(t *testing.T) {
	result := buildSrcFreqJSON(nil, nil, 0)
	if result.SrcListJSON != nil {
		t.Errorf("SrcListJSON = %s, want nil", result.SrcListJSON)
	}
	if result.FreqListJSON != nil {
		t.Errorf("FreqListJSON = %s, want nil", result.FreqListJSON)
	}
	if len(result.UnitIDs) != 0 {
		t.Errorf("UnitIDs = %v, want empty", result.UnitIDs)
	}
}

func TestBuildSrcFreqJSON_FreqListOnly(t *testing.T) {
	freqs := []FreqItem{
		{Freq: 851000000, Time: 1000, Pos: 0.0, Len: 1.5, ErrorCount: 2, SpikeCount: 1},
		{Freq: 852000000, Time: 1001, Pos: 1.5, Len: 2.0, ErrorCount: 0, SpikeCount: 0},
		{Freq: 853000000, Time: 1002, Pos: 3.5, Len: 0.5, ErrorCount: 1, SpikeCount: 3},
	}
	result := buildSrcFreqJSON(nil, freqs, 0)

	if result.SrcListJSON != nil {
		t.Errorf("SrcListJSON should be nil, got %s", result.SrcListJSON)
	}
	if result.FreqListJSON == nil {
		t.Fatal("FreqListJSON should not be nil")
	}

	var entries []struct {
		Freq       int64   `json:"freq"`
		Time       int64   `json:"time"`
		Pos        float64 `json:"pos"`
		Len        float64 `json:"len"`
		ErrorCount int     `json:"error_count"`
		SpikeCount int     `json:"spike_count"`
	}
	if err := json.Unmarshal(result.FreqListJSON, &entries); err != nil {
		t.Fatalf("unmarshal FreqListJSON: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Verify freq is int64 (truncated from float)
	if entries[0].Freq != 851000000 {
		t.Errorf("entries[0].Freq = %d, want 851000000", entries[0].Freq)
	}
	if entries[1].Pos != 1.5 {
		t.Errorf("entries[1].Pos = %f, want 1.5", entries[1].Pos)
	}
	if entries[2].SpikeCount != 3 {
		t.Errorf("entries[2].SpikeCount = %d, want 3", entries[2].SpikeCount)
	}
}

func TestBuildSrcFreqJSON_SrcListDurations(t *testing.T) {
	srcs := []SrcItem{
		{Src: 100, Time: 1000, Pos: 0.0},
		{Src: 200, Time: 1002, Pos: 2.0},
	}

	t.Run("duration_from_next_entry", func(t *testing.T) {
		result := buildSrcFreqJSON(srcs, nil, 5)
		var entries []struct {
			Src      int     `json:"src"`
			Duration float64 `json:"duration"`
		}
		if err := json.Unmarshal(result.SrcListJSON, &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		// First entry: duration = next.Pos - curr.Pos = 2.0 - 0.0 = 2.0
		if entries[0].Duration != 2.0 {
			t.Errorf("entries[0].Duration = %f, want 2.0", entries[0].Duration)
		}
		// Last entry: duration = callLength - pos = 5 - 2.0 = 3.0
		if entries[1].Duration != 3.0 {
			t.Errorf("entries[1].Duration = %f, want 3.0", entries[1].Duration)
		}
	})

	t.Run("last_entry_callLength_zero", func(t *testing.T) {
		single := []SrcItem{{Src: 100, Time: 1000, Pos: 1.0}}
		result := buildSrcFreqJSON(single, nil, 0)
		var entries []struct {
			Duration float64 `json:"duration"`
		}
		json.Unmarshal(result.SrcListJSON, &entries)
		if entries[0].Duration != 0 {
			t.Errorf("duration = %f, want 0 (callLength=0)", entries[0].Duration)
		}
	})
}

func TestBuildSrcFreqJSON_UnitIDs(t *testing.T) {
	srcs := []SrcItem{
		{Src: 100, Time: 1000, Pos: 0.0},
		{Src: 200, Time: 1001, Pos: 1.0},
		{Src: 100, Time: 1002, Pos: 2.0}, // duplicate
	}
	result := buildSrcFreqJSON(srcs, nil, 3)

	// Should have 2 unique unit IDs (100 and 200)
	if len(result.UnitIDs) != 2 {
		t.Fatalf("got %d unitIDs, want 2", len(result.UnitIDs))
	}
	idSet := make(map[int32]bool)
	for _, id := range result.UnitIDs {
		idSet[id] = true
	}
	if !idSet[100] || !idSet[200] {
		t.Errorf("UnitIDs = %v, want to contain 100 and 200", result.UnitIDs)
	}
}

func TestBuildSrcFreqJSON_Mixed(t *testing.T) {
	srcs := []SrcItem{{Src: 100, Time: 1000, Pos: 0.0}}
	freqs := []FreqItem{{Freq: 851000000, Time: 1000, Pos: 0.0, Len: 3.0}}
	result := buildSrcFreqJSON(srcs, freqs, 3)

	if result.SrcListJSON == nil {
		t.Error("SrcListJSON should not be nil")
	}
	if result.FreqListJSON == nil {
		t.Error("FreqListJSON should not be nil")
	}
	if len(result.UnitIDs) != 1 {
		t.Errorf("got %d unitIDs, want 1", len(result.UnitIDs))
	}
}

// ── buildAudioFilename ───────────────────────────────────────────────

func TestBuildAudioFilename(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	unixStr := fmt.Sprintf("%d", ts.Unix())

	tests := []struct {
		name      string
		filename  string
		audioType string
		want      string
	}{
		{"provided_filename", "call_123.m4a", "m4a", "call_123.m4a"},
		{"empty_filename_m4a", "", "m4a", unixStr + ".m4a"},
		{"empty_filename_wav", "", "wav", unixStr + ".wav"},
		{"empty_filename_dot_wav", "", ".wav", unixStr + ".wav"},
		{"empty_filename_empty_type", "", "", unixStr + ".wav"},
		{"empty_filename_mp3", "", "mp3", unixStr + ".mp3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAudioFilename(tt.filename, tt.audioType, ts)
			if got != tt.want {
				t.Errorf("buildAudioFilename(%q, %q) = %q, want %q", tt.filename, tt.audioType, got, tt.want)
			}
		})
	}
}

// ── buildAudioRelPath ────────────────────────────────────────────────

func TestBuildAudioRelPath(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	got := buildAudioRelPath("butco", ts, "1750325400.m4a")
	// filepath.Join uses OS-specific separator, so use filepath.Join for expected value
	want := filepath.Join("butco", "2025-06-15", "1750325400.m4a")
	if got != want {
		t.Errorf("buildAudioRelPath = %q, want %q", got, want)
	}
}
