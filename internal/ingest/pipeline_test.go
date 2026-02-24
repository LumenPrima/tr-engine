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

// ── activeCallMap CRUD ───────────────────────────────────────────────

func TestActiveCallMapCRUD(t *testing.T) {
	t.Run("set_and_get", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key1", activeCallEntry{CallID: 1, Tgid: 100})

		entry, ok := m.Get("key1")
		if !ok {
			t.Fatal("expected key1 to exist")
		}
		if entry.CallID != 1 || entry.Tgid != 100 {
			t.Errorf("got CallID=%d Tgid=%d, want 1, 100", entry.CallID, entry.Tgid)
		}
	})

	t.Run("get_missing_returns_false", func(t *testing.T) {
		m := newActiveCallMap()
		_, ok := m.Get("nonexistent")
		if ok {
			t.Error("expected ok=false for missing key")
		}
	})

	t.Run("delete_removes_entry", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key1", activeCallEntry{CallID: 1})
		m.Delete("key1")

		_, ok := m.Get("key1")
		if ok {
			t.Error("expected key1 to be deleted")
		}
	})

	t.Run("delete_nonexistent_is_noop", func(t *testing.T) {
		m := newActiveCallMap()
		m.Delete("nonexistent") // should not panic
	})

	t.Run("len_tracks_count", func(t *testing.T) {
		m := newActiveCallMap()
		if m.Len() != 0 {
			t.Errorf("Len = %d, want 0", m.Len())
		}
		m.Set("a", activeCallEntry{CallID: 1})
		m.Set("b", activeCallEntry{CallID: 2})
		if m.Len() != 2 {
			t.Errorf("Len = %d, want 2", m.Len())
		}
		m.Delete("a")
		if m.Len() != 1 {
			t.Errorf("Len = %d, want 1", m.Len())
		}
	})

	t.Run("set_overwrites_existing", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("key1", activeCallEntry{CallID: 1})
		m.Set("key1", activeCallEntry{CallID: 2})

		entry, _ := m.Get("key1")
		if entry.CallID != 2 {
			t.Errorf("CallID = %d, want 2 (overwritten)", entry.CallID)
		}
		if m.Len() != 1 {
			t.Errorf("Len = %d, want 1", m.Len())
		}
	})

	t.Run("all_returns_snapshot", func(t *testing.T) {
		m := newActiveCallMap()
		m.Set("a", activeCallEntry{CallID: 1, Tgid: 100})
		m.Set("b", activeCallEntry{CallID: 2, Tgid: 200})

		snapshot := m.All()
		if len(snapshot) != 2 {
			t.Fatalf("All returned %d entries, want 2", len(snapshot))
		}
		if snapshot["a"].CallID != 1 {
			t.Errorf("a.CallID = %d, want 1", snapshot["a"].CallID)
		}
		if snapshot["b"].CallID != 2 {
			t.Errorf("b.CallID = %d, want 2", snapshot["b"].CallID)
		}

		// Verify it's a copy — mutating snapshot doesn't affect original
		delete(snapshot, "a")
		if m.Len() != 2 {
			t.Error("deleting from snapshot should not affect original map")
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

// ── beginningOfMonth ─────────────────────────────────────────────────

func TestBeginningOfMonth(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  time.Time
	}{
		{
			"mid_month",
			time.Date(2025, 6, 15, 14, 30, 45, 123, time.UTC),
			time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"first_of_month",
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"last_day_december",
			time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"leap_year_feb_29",
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"non_leap_year_feb",
			time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"preserves_location",
			time.Date(2025, 3, 15, 10, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			time.Date(2025, 3, 1, 0, 0, 0, 0, time.FixedZone("EST", -5*3600)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := beginningOfMonth(tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("beginningOfMonth(%v) = %v, want %v", tt.input, got, tt.want)
			}
			if got.Location().String() != tt.want.Location().String() {
				t.Errorf("location = %v, want %v", got.Location(), tt.want.Location())
			}
		})
	}
}

// ── unitDedupKey struct equality ─────────────────────────────────────

func TestUnitDedupKeyEquality(t *testing.T) {
	a := unitDedupKey{SystemID: 1, UnitID: 100, EventType: "call", Tgid: 200}
	b := unitDedupKey{SystemID: 1, UnitID: 100, EventType: "call", Tgid: 200}

	if a != b {
		t.Error("identical keys should be equal")
	}

	c := unitDedupKey{SystemID: 1, UnitID: 100, EventType: "end", Tgid: 200}
	if a == c {
		t.Error("different EventType should not be equal")
	}

	d := unitDedupKey{SystemID: 2, UnitID: 100, EventType: "call", Tgid: 200}
	if a == d {
		t.Error("different SystemID should not be equal")
	}
}
