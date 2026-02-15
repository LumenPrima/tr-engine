package ingest

import (
	"testing"

	"github.com/snarg/tr-engine/internal/api"
)

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
