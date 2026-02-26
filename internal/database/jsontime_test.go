package database

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSrcFreqTimestamps_FreqList(t *testing.T) {
	input := json.RawMessage(`[{"freq":154875000,"time":1713207802,"pos":0.0,"len":3.24,"error_count":50,"spike_count":3}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	timeVal, ok := entries[0]["time"].(string)
	if !ok {
		t.Fatalf("time should be string, got %T: %v", entries[0]["time"], entries[0]["time"])
	}
	if timeVal != "2024-04-15T19:03:22Z" {
		t.Errorf("time = %q, want %q", timeVal, "2024-04-15T19:03:22Z")
	}
	// Other fields preserved
	if entries[0]["freq"].(float64) != 154875000 {
		t.Error("freq not preserved")
	}
}

func TestNormalizeSrcFreqTimestamps_SrcList(t *testing.T) {
	input := json.RawMessage(`[{"src":104,"tag":"09 7COM3","time":1713207802,"pos":0.0,"duration":3.5,"emergency":0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatal(err)
	}
	timeVal := entries[0]["time"].(string)
	if timeVal != "2024-04-15T19:03:22Z" {
		t.Errorf("time = %q, want %q", timeVal, "2024-04-15T19:03:22Z")
	}
}

func TestNormalizeSrcFreqTimestamps_NilAndEmpty(t *testing.T) {
	if out := NormalizeSrcFreqTimestamps(nil); out != nil {
		t.Errorf("nil input should return nil, got %s", out)
	}
	if out := NormalizeSrcFreqTimestamps(json.RawMessage(`null`)); string(out) != "null" {
		t.Errorf("null input should return null, got %s", out)
	}
	if out := NormalizeSrcFreqTimestamps(json.RawMessage(`[]`)); string(out) != "[]" {
		t.Errorf("empty array should return [], got %s", out)
	}
}

func TestNormalizeSrcFreqTimestamps_ZeroTime(t *testing.T) {
	input := json.RawMessage(`[{"freq":154875000,"time":0,"pos":0.0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	// time=0 should be omitted (or null) since it's meaningless
	if _, exists := entries[0]["time"]; exists {
		t.Errorf("time=0 should be omitted, got %v", entries[0]["time"])
	}
}

func TestNormalizeSrcFreqTimestamps_AlreadyString(t *testing.T) {
	// If time is already a string (future-proof), pass through unchanged
	input := json.RawMessage(`[{"freq":154875000,"time":"2024-04-15T19:03:22Z","pos":0.0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	if entries[0]["time"] != "2024-04-15T19:03:22Z" {
		t.Errorf("string time should pass through, got %v", entries[0]["time"])
	}
}

func TestNormalizeSrcFreqTimestamps_PreservesOtherFields(t *testing.T) {
	input := json.RawMessage(`[{"src":104,"tag":"09 7COM3","time":1713207802,"pos":0.5,"duration":3.5,"emergency":0,"signal_system":""}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	e := entries[0]
	if e["src"].(float64) != 104 {
		t.Error("src not preserved")
	}
	if e["tag"] != "09 7COM3" {
		t.Error("tag not preserved")
	}
	if e["pos"].(float64) != 0.5 {
		t.Error("pos not preserved")
	}
	if e["duration"].(float64) != 3.5 {
		t.Error("duration not preserved")
	}
	if e["emergency"].(float64) != 0 {
		t.Error("emergency not preserved")
	}
}
