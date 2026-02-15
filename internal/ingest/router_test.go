package ingest

import "testing"

func TestParseTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		want    *Route
		wantNil bool
	}{
		// Feed handlers
		{name: "status", topic: "trdash/feeds/trunk_recorder/status", want: &Route{Handler: "status"}},
		{name: "console", topic: "trdash/feeds/trunk_recorder/console", want: &Route{Handler: "console"}},
		{name: "systems", topic: "trdash/feeds/systems", want: &Route{Handler: "systems"}},
		{name: "system", topic: "trdash/feeds/system", want: &Route{Handler: "system"}},
		{name: "calls_active", topic: "trdash/feeds/calls_active", want: &Route{Handler: "calls_active"}},
		{name: "call_start", topic: "trdash/feeds/call_start", want: &Route{Handler: "call_start"}},
		{name: "call_end", topic: "trdash/feeds/call_end", want: &Route{Handler: "call_end"}},
		{name: "audio", topic: "trdash/feeds/audio", want: &Route{Handler: "audio"}},
		{name: "recorders", topic: "trdash/feeds/recorders", want: &Route{Handler: "recorders"}},
		{name: "recorder", topic: "trdash/feeds/recorder", want: &Route{Handler: "recorder"}},
		{name: "rates", topic: "trdash/feeds/rates", want: &Route{Handler: "rates"}},
		{name: "config", topic: "trdash/feeds/config", want: &Route{Handler: "config"}},

		// Trunking messages with SysName extraction
		{name: "trunking_butco", topic: "trdash/messages/butco/message", want: &Route{Handler: "trunking_message", SysName: "butco"}},
		{name: "trunking_warco", topic: "trdash/messages/warco/message", want: &Route{Handler: "trunking_message", SysName: "warco"}},

		// Unit events with SysName extraction
		{name: "unit_on", topic: "trdash/units/butco/on", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_call", topic: "trdash/units/warco/call", want: &Route{Handler: "unit_event", SysName: "warco"}},
		{name: "unit_location", topic: "trdash/units/butco/location", want: &Route{Handler: "unit_event", SysName: "butco"}},

		// Nil cases
		{name: "empty_string", topic: "", wantNil: true},
		{name: "unknown_feed", topic: "trdash/feeds/unknown_handler", wantNil: true},
		{name: "wrong_prefix", topic: "other/feeds/call_start", wantNil: true},
		{name: "wrong_segment1", topic: "trdash/bogus/call_start", wantNil: true},
		{name: "too_few_parts", topic: "trdash/feeds", wantNil: true},
		{name: "messages_wrong_suffix", topic: "trdash/messages/butco/wrong", wantNil: true},
		{name: "units_too_many_parts", topic: "trdash/units/butco/on/extra", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTopic(tt.topic)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ParseTopic(%q) = %+v, want nil", tt.topic, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseTopic(%q) = nil, want %+v", tt.topic, tt.want)
			}
			if got.Handler != tt.want.Handler {
				t.Errorf("Handler = %q, want %q", got.Handler, tt.want.Handler)
			}
			if got.SysName != tt.want.SysName {
				t.Errorf("SysName = %q, want %q", got.SysName, tt.want.SysName)
			}
		})
	}
}
