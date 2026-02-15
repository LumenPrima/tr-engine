package ingest

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestParseHandlerSet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]bool
	}{
		{name: "empty", input: "", want: map[string]bool{}},
		{name: "single", input: "audio", want: map[string]bool{"audio": true}},
		{name: "multiple", input: "audio,status,console", want: map[string]bool{"audio": true, "status": true, "console": true}},
		{name: "whitespace_trimmed", input: " audio , status ", want: map[string]bool{"audio": true, "status": true}},
		{name: "trailing_comma", input: "audio,status,", want: map[string]bool{"audio": true, "status": true}},
		{name: "leading_comma", input: ",audio", want: map[string]bool{"audio": true}},
		{name: "only_commas", input: ",,,", want: map[string]bool{}},
		{name: "spaces_only_entry", input: "audio, ,status", want: map[string]bool{"audio": true, "status": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHandlerSet(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseHandlerSet(%q) has %d entries, want %d\ngot:  %v\nwant: %v",
					tt.input, len(got), len(tt.want), got, tt.want)
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("parseHandlerSet(%q) missing key %q", tt.input, k)
				}
			}
		})
	}
}

func TestStripAudioBase64(t *testing.T) {
	// Helper to build a payload with optional audio fields inside "call"
	makePayload := func(callFields map[string]string, extraTopLevel map[string]string) []byte {
		call := make(map[string]any)
		for k, v := range callFields {
			call[k] = v
		}
		obj := map[string]any{"call": call}
		for k, v := range extraTopLevel {
			obj[k] = v
		}
		b, _ := json.Marshal(obj)
		return b
	}

	t.Run("strips_both_fields", func(t *testing.T) {
		payload := makePayload(map[string]string{
			"audio_m4a_base64": "AAAA",
			"audio_wav_base64": "BBBB",
			"freq":             "851000000",
		}, nil)
		result := stripAudioBase64(payload)
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(result, &obj); err != nil {
			t.Fatal(err)
		}
		var call map[string]json.RawMessage
		if err := json.Unmarshal(obj["call"], &call); err != nil {
			t.Fatal(err)
		}
		if _, ok := call["audio_m4a_base64"]; ok {
			t.Error("audio_m4a_base64 should be stripped")
		}
		if _, ok := call["audio_wav_base64"]; ok {
			t.Error("audio_wav_base64 should be stripped")
		}
		if _, ok := call["freq"]; !ok {
			t.Error("freq should be preserved")
		}
	})

	t.Run("strips_m4a_only", func(t *testing.T) {
		payload := makePayload(map[string]string{
			"audio_m4a_base64": "AAAA",
			"freq":             "851000000",
		}, nil)
		result := stripAudioBase64(payload)
		var obj map[string]json.RawMessage
		json.Unmarshal(result, &obj)
		var call map[string]json.RawMessage
		json.Unmarshal(obj["call"], &call)
		if _, ok := call["audio_m4a_base64"]; ok {
			t.Error("audio_m4a_base64 should be stripped")
		}
	})

	t.Run("strips_wav_only", func(t *testing.T) {
		payload := makePayload(map[string]string{
			"audio_wav_base64": "BBBB",
		}, nil)
		result := stripAudioBase64(payload)
		var obj map[string]json.RawMessage
		json.Unmarshal(result, &obj)
		var call map[string]json.RawMessage
		json.Unmarshal(obj["call"], &call)
		if _, ok := call["audio_wav_base64"]; ok {
			t.Error("audio_wav_base64 should be stripped")
		}
	})

	t.Run("no_audio_fields", func(t *testing.T) {
		payload := makePayload(map[string]string{"freq": "851000000"}, nil)
		result := stripAudioBase64(payload)
		// Should still be valid JSON with call.freq preserved
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(result, &obj); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no_call_key", func(t *testing.T) {
		payload := []byte(`{"instance_id":"tr-1","other":"data"}`)
		result := stripAudioBase64(payload)
		if !bytes.Equal(result, payload) {
			t.Errorf("expected original payload returned unchanged")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		payload := []byte(`{not valid json`)
		result := stripAudioBase64(payload)
		if !bytes.Equal(result, payload) {
			t.Errorf("expected original payload returned unchanged")
		}
	})

	t.Run("call_is_not_object", func(t *testing.T) {
		payload := []byte(`{"call":"just a string"}`)
		result := stripAudioBase64(payload)
		if !bytes.Equal(result, payload) {
			t.Errorf("expected original payload returned unchanged")
		}
	})
}

func TestActiveCallMapFindByTgidAndTime(t *testing.T) {
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	tolerance := 5 * time.Second

	t.Run("exact_match", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("1_100_1000", activeCallEntry{Tgid: 100, StartTime: base, CallID: 1})
		key, entry, ok := m.FindByTgidAndTime(100, base, tolerance)
		if !ok {
			t.Fatal("expected match")
		}
		if key != "1_100_1000" || entry.CallID != 1 {
			t.Errorf("got key=%q callID=%d", key, entry.CallID)
		}
	})

	t.Run("within_tolerance", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("1_100_1000", activeCallEntry{Tgid: 100, StartTime: base, CallID: 1})
		_, _, ok := m.FindByTgidAndTime(100, base.Add(3*time.Second), tolerance)
		if !ok {
			t.Fatal("expected match within tolerance")
		}
	})

	t.Run("picks_closest", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("far", activeCallEntry{Tgid: 100, StartTime: base.Add(-4 * time.Second), CallID: 1})
		m.Set("close", activeCallEntry{Tgid: 100, StartTime: base.Add(-1 * time.Second), CallID: 2})
		_, entry, ok := m.FindByTgidAndTime(100, base, tolerance)
		if !ok {
			t.Fatal("expected match")
		}
		if entry.CallID != 2 {
			t.Errorf("expected closest (CallID=2), got CallID=%d", entry.CallID)
		}
	})

	t.Run("negative_time_diff", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key", activeCallEntry{Tgid: 100, StartTime: base.Add(2 * time.Second), CallID: 1})
		_, _, ok := m.FindByTgidAndTime(100, base, tolerance)
		if !ok {
			t.Fatal("expected match with negative diff")
		}
	})

	t.Run("outside_tolerance", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key", activeCallEntry{Tgid: 100, StartTime: base.Add(10 * time.Second), CallID: 1})
		_, _, ok := m.FindByTgidAndTime(100, base, tolerance)
		if ok {
			t.Fatal("expected no match outside tolerance")
		}
	})

	t.Run("wrong_tgid", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key", activeCallEntry{Tgid: 200, StartTime: base, CallID: 1})
		_, _, ok := m.FindByTgidAndTime(100, base, tolerance)
		if ok {
			t.Fatal("expected no match for wrong tgid")
		}
	})

	t.Run("different_tgid_ignored", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("wrong_tg", activeCallEntry{Tgid: 200, StartTime: base, CallID: 1})
		m.Set("right_tg", activeCallEntry{Tgid: 100, StartTime: base.Add(3 * time.Second), CallID: 2})
		_, entry, ok := m.FindByTgidAndTime(100, base, tolerance)
		if !ok {
			t.Fatal("expected match")
		}
		if entry.CallID != 2 {
			t.Errorf("expected CallID=2, got %d", entry.CallID)
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		m := newActiveCallMap()
		_, _, ok := m.FindByTgidAndTime(100, base, tolerance)
		if ok {
			t.Fatal("expected no match in empty map")
		}
	})
}
