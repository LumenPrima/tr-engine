package ingest

import "testing"

func TestParseTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		want    *Route
		wantNil bool
	}{
		// Feed handlers — any prefix works
		{name: "status", topic: "trengine/feeds/trunk_recorder/status", want: &Route{Handler: "status"}},
		{name: "console", topic: "trengine/feeds/trunk_recorder/console", want: &Route{Handler: "console"}},
		{name: "systems", topic: "trengine/feeds/systems", want: &Route{Handler: "systems"}},
		{name: "system", topic: "trengine/feeds/system", want: &Route{Handler: "system"}},
		{name: "calls_active", topic: "trengine/feeds/calls_active", want: &Route{Handler: "calls_active"}},
		{name: "call_start", topic: "trengine/feeds/call_start", want: &Route{Handler: "call_start"}},
		{name: "call_end", topic: "trengine/feeds/call_end", want: &Route{Handler: "call_end"}},
		{name: "audio", topic: "trengine/feeds/audio", want: &Route{Handler: "audio"}},
		{name: "recorders", topic: "trengine/feeds/recorders", want: &Route{Handler: "recorders"}},
		{name: "recorder", topic: "trengine/feeds/recorder", want: &Route{Handler: "recorder"}},
		{name: "rates", topic: "trengine/feeds/rates", want: &Route{Handler: "rates"}},
		{name: "config", topic: "trengine/feeds/config", want: &Route{Handler: "config"}},

		// Trunking messages with SysName extraction
		{name: "trunking_butco", topic: "trengine/messages/butco/message", want: &Route{Handler: "trunking_message", SysName: "butco"}},
		{name: "trunking_warco", topic: "trengine/messages/warco/message", want: &Route{Handler: "trunking_message", SysName: "warco"}},

		// Unit events with SysName extraction
		{name: "unit_on", topic: "trengine/units/butco/on", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_call", topic: "trengine/units/warco/call", want: &Route{Handler: "unit_event", SysName: "warco"}},
		{name: "unit_location", topic: "trengine/units/butco/location", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_off", topic: "trengine/units/butco/off", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_end", topic: "trengine/units/butco/end", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_join", topic: "trengine/units/butco/join", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_ackresp", topic: "trengine/units/butco/ackresp", want: &Route{Handler: "unit_event", SysName: "butco"}},
		{name: "unit_data", topic: "trengine/units/butco/data", want: &Route{Handler: "unit_event", SysName: "butco"}},

		// Custom prefixes — router only cares about trailing segments
		{name: "custom_prefix_feed", topic: "myradio/whatever/call_start", want: &Route{Handler: "call_start"}},
		{name: "custom_prefix_units", topic: "myradio/site1/on", want: &Route{Handler: "unit_event", SysName: "site1"}},
		{name: "custom_prefix_messages", topic: "myradio/site1/message", want: &Route{Handler: "trunking_message", SysName: "site1"}},
		{name: "custom_prefix_console", topic: "robotastic/feeds/trunk_recorder/console", want: &Route{Handler: "console"}},
		{name: "deep_prefix", topic: "org/dept/radio/rates", want: &Route{Handler: "rates"}},
		{name: "no_middle_segment", topic: "flat/call_end", want: &Route{Handler: "call_end"}},

		// Nil cases
		{name: "empty_string", topic: "", wantNil: true},
		{name: "single_segment", topic: "call_start", wantNil: true},
		{name: "unknown_suffix", topic: "trengine/feeds/unknown_handler", wantNil: true},
		{name: "unknown_unit_event", topic: "trengine/units/butco/bogus", wantNil: true},
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
