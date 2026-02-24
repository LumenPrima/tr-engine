package ingest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/snarg/tr-engine/internal/api"
)

// ── EventBus Publish/Subscribe ────────────────────────────────────────

func TestEventBusPublishSubscribe(t *testing.T) {
	t.Run("subscriber_receives_published_event", func(t *testing.T) {
		eb := NewEventBus(64)
		ch, cancel := eb.Subscribe(api.EventFilter{})
		defer cancel()

		eb.Publish(EventData{
			Type:     "call_start",
			SystemID: 1,
			Tgid:     100,
			Payload:  map[string]string{"msg": "hello"},
		})

		select {
		case evt := <-ch:
			if evt.Type != "call_start" {
				t.Errorf("Type = %q, want call_start", evt.Type)
			}
			if evt.SystemID != 1 {
				t.Errorf("SystemID = %d, want 1", evt.SystemID)
			}
			if evt.Tgid != 100 {
				t.Errorf("Tgid = %d, want 100", evt.Tgid)
			}
			if evt.ID == "" {
				t.Error("expected non-empty event ID")
			}
			// Verify data is valid JSON
			var payload map[string]string
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				t.Fatalf("Data is not valid JSON: %v", err)
			}
			if payload["msg"] != "hello" {
				t.Errorf("payload msg = %q, want hello", payload["msg"])
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("filtered_subscriber_misses_non_matching", func(t *testing.T) {
		eb := NewEventBus(64)
		ch, cancel := eb.Subscribe(api.EventFilter{Types: []string{"call_end"}})
		defer cancel()

		eb.Publish(EventData{Type: "call_start", Payload: "x"})

		select {
		case evt := <-ch:
			t.Fatalf("should not receive event, got %+v", evt)
		case <-time.After(50 * time.Millisecond):
			// expected
		}
	})

	t.Run("cancel_stops_delivery", func(t *testing.T) {
		eb := NewEventBus(64)
		ch, cancel := eb.Subscribe(api.EventFilter{})
		cancel()

		eb.Publish(EventData{Type: "call_start", Payload: "x"})

		select {
		case _, ok := <-ch:
			if ok {
				t.Fatal("should not receive event after cancel")
			}
		case <-time.After(50 * time.Millisecond):
			// expected — channel not closed, just removed from map
		}
	})

	t.Run("multiple_subscribers", func(t *testing.T) {
		eb := NewEventBus(64)
		ch1, cancel1 := eb.Subscribe(api.EventFilter{})
		defer cancel1()
		ch2, cancel2 := eb.Subscribe(api.EventFilter{})
		defer cancel2()

		eb.Publish(EventData{Type: "call_start", Payload: "x"})

		for i, ch := range []<-chan api.SSEEvent{ch1, ch2} {
			select {
			case evt := <-ch:
				if evt.Type != "call_start" {
					t.Errorf("subscriber %d: Type = %q, want call_start", i, evt.Type)
				}
			case <-time.After(time.Second):
				t.Fatalf("subscriber %d: timed out", i)
			}
		}
	})
}

// ── EventBus ReplaySince ─────────────────────────────────────────────

func TestEventBusReplaySince(t *testing.T) {
	t.Run("replay_all_when_empty_lastID", func(t *testing.T) {
		eb := NewEventBus(64)
		eb.Publish(EventData{Type: "call_start", Payload: "a"})
		eb.Publish(EventData{Type: "call_end", Payload: "b"})

		events := eb.ReplaySince("", api.EventFilter{})
		if len(events) != 2 {
			t.Fatalf("got %d events, want 2", len(events))
		}
	})

	t.Run("replay_after_specific_id", func(t *testing.T) {
		eb := NewEventBus(64)
		eb.Publish(EventData{Type: "call_start", Payload: "a"})

		// Grab the first event's ID from the ring
		allEvents := eb.ReplaySince("", api.EventFilter{})
		if len(allEvents) != 1 {
			t.Fatalf("expected 1 event, got %d", len(allEvents))
		}
		firstID := allEvents[0].ID

		eb.Publish(EventData{Type: "call_end", Payload: "b"})

		events := eb.ReplaySince(firstID, api.EventFilter{})
		if len(events) != 1 {
			t.Fatalf("got %d events, want 1 (after first)", len(events))
		}
		if events[0].Type != "call_end" {
			t.Errorf("Type = %q, want call_end", events[0].Type)
		}
	})

	t.Run("replay_with_filter", func(t *testing.T) {
		eb := NewEventBus(64)
		eb.Publish(EventData{Type: "call_start", SystemID: 1, Payload: "a"})
		eb.Publish(EventData{Type: "call_start", SystemID: 2, Payload: "b"})

		events := eb.ReplaySince("", api.EventFilter{Systems: []int{2}})
		if len(events) != 1 {
			t.Fatalf("got %d events, want 1 (filtered)", len(events))
		}
		if events[0].SystemID != 2 {
			t.Errorf("SystemID = %d, want 2", events[0].SystemID)
		}
	})

	t.Run("unknown_lastID_replays_all", func(t *testing.T) {
		eb := NewEventBus(64)
		eb.Publish(EventData{Type: "call_start", Payload: "a"})

		// When lastEventID is not found (overwritten by ring wrap), all available
		// events are returned so the client doesn't silently miss everything.
		events := eb.ReplaySince("nonexistent-id", api.EventFilter{})
		if len(events) != 1 {
			t.Fatalf("got %d events, want 1 (fallback replay all)", len(events))
		}
	})
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name   string
		event  api.SSEEvent
		filter api.EventFilter
		want   bool
	}{
		// Empty filter matches everything
		{
			name:   "empty_filter_matches_all",
			event:  api.SSEEvent{Type: "call_start", SystemID: 1, Tgid: 100},
			filter: api.EventFilter{},
			want:   true,
		},

		// Type matching
		{
			name:   "type_match",
			event:  api.SSEEvent{Type: "call_start"},
			filter: api.EventFilter{Types: []string{"call_start"}},
			want:   true,
		},
		{
			name:   "type_no_match",
			event:  api.SSEEvent{Type: "call_start"},
			filter: api.EventFilter{Types: []string{"call_end"}},
			want:   false,
		},
		{
			name:   "type_multiple_one_matches",
			event:  api.SSEEvent{Type: "call_end"},
			filter: api.EventFilter{Types: []string{"call_start", "call_end"}},
			want:   true,
		},

		// Compound type syntax
		{
			name:   "compound_type_exact_match",
			event:  api.SSEEvent{Type: "unit_event", SubType: "call"},
			filter: api.EventFilter{Types: []string{"unit_event:call"}},
			want:   true,
		},
		{
			name:   "compound_type_wrong_subtype",
			event:  api.SSEEvent{Type: "unit_event", SubType: "on"},
			filter: api.EventFilter{Types: []string{"unit_event:call"}},
			want:   false,
		},
		{
			name:   "plain_type_matches_any_subtype",
			event:  api.SSEEvent{Type: "unit_event", SubType: "call"},
			filter: api.EventFilter{Types: []string{"unit_event"}},
			want:   true,
		},
		{
			name:   "mixed_compound_and_plain",
			event:  api.SSEEvent{Type: "call_start"},
			filter: api.EventFilter{Types: []string{"unit_event:call", "call_start"}},
			want:   true,
		},

		// System filter
		{
			name:   "system_match",
			event:  api.SSEEvent{Type: "call_start", SystemID: 1},
			filter: api.EventFilter{Systems: []int{1, 2}},
			want:   true,
		},
		{
			name:   "system_no_match",
			event:  api.SSEEvent{Type: "call_start", SystemID: 3},
			filter: api.EventFilter{Systems: []int{1, 2}},
			want:   false,
		},
		{
			name:   "system_zero_passes_through",
			event:  api.SSEEvent{Type: "recorder_update", SystemID: 0},
			filter: api.EventFilter{Systems: []int{1}},
			want:   true,
		},

		// Site filter
		{
			name:   "site_match",
			event:  api.SSEEvent{Type: "call_start", SiteID: 5},
			filter: api.EventFilter{Sites: []int{5}},
			want:   true,
		},
		{
			name:   "site_zero_passes_through",
			event:  api.SSEEvent{Type: "rate_update", SiteID: 0},
			filter: api.EventFilter{Sites: []int{5}},
			want:   true,
		},

		// Tgid filter
		{
			name:   "tgid_match",
			event:  api.SSEEvent{Type: "call_start", Tgid: 100},
			filter: api.EventFilter{Tgids: []int{100, 200}},
			want:   true,
		},
		{
			name:   "tgid_no_match",
			event:  api.SSEEvent{Type: "call_start", Tgid: 300},
			filter: api.EventFilter{Tgids: []int{100, 200}},
			want:   false,
		},

		// Unit filter
		{
			name:   "unit_match",
			event:  api.SSEEvent{Type: "unit_event", UnitID: 42},
			filter: api.EventFilter{Units: []int{42}},
			want:   true,
		},
		{
			name:   "unit_zero_passes_through",
			event:  api.SSEEvent{Type: "call_start", UnitID: 0},
			filter: api.EventFilter{Units: []int{42}},
			want:   true,
		},

		// Multi-dimension AND logic
		{
			name:   "multi_all_pass",
			event:  api.SSEEvent{Type: "call_start", SystemID: 1, Tgid: 100},
			filter: api.EventFilter{Types: []string{"call_start"}, Systems: []int{1}, Tgids: []int{100}},
			want:   true,
		},
		{
			name:   "multi_one_fails",
			event:  api.SSEEvent{Type: "call_start", SystemID: 1, Tgid: 300},
			filter: api.EventFilter{Types: []string{"call_start"}, Systems: []int{1}, Tgids: []int{100}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.event, tt.filter)
			if got != tt.want {
				t.Errorf("matchesFilter(%+v, %+v) = %v, want %v", tt.event, tt.filter, got, tt.want)
			}
		})
	}
}
