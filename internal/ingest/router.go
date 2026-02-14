package ingest

import "strings"

// Route describes a parsed MQTT topic.
type Route struct {
	Handler string // handler name: "status", "systems", "system", "calls_active", "call_start", "call_end", "audio", "recorders", "recorder", "rates", "unit_event"
	SysName string // only set for unit event topics
}

// ParseTopic maps an MQTT topic string to a Route.
//
// Feed topics:
//
//	trdash/feeds/trunk_recorder/status → status
//	trdash/feeds/systems               → systems
//	trdash/feeds/system                → system
//	trdash/feeds/calls_active          → calls_active
//	trdash/feeds/call_start            → call_start
//	trdash/feeds/call_end              → call_end
//	trdash/feeds/audio                 → audio
//	trdash/feeds/recorders             → recorders
//	trdash/feeds/recorder              → recorder
//	trdash/feeds/rates                 → rates
//
// Unit event topics:
//
//	trdash/units/{sys_name}/{event_type} → unit_event (event_type extracted from payload)
func ParseTopic(topic string) *Route {
	parts := strings.Split(topic, "/")

	if len(parts) >= 3 && parts[0] == "trdash" {
		switch parts[1] {
		case "feeds":
			feed := strings.Join(parts[2:], "/")
			switch feed {
			case "trunk_recorder/status":
				return &Route{Handler: "status"}
			case "systems":
				return &Route{Handler: "systems"}
			case "system":
				return &Route{Handler: "system"}
			case "calls_active":
				return &Route{Handler: "calls_active"}
			case "call_start":
				return &Route{Handler: "call_start"}
			case "call_end":
				return &Route{Handler: "call_end"}
			case "audio":
				return &Route{Handler: "audio"}
			case "recorders":
				return &Route{Handler: "recorders"}
			case "recorder":
				return &Route{Handler: "recorder"}
			case "rates":
				return &Route{Handler: "rates"}
			case "config":
				return &Route{Handler: "config"}
			}
		case "units":
			if len(parts) == 4 {
				return &Route{
					Handler: "unit_event",
					SysName: parts[2],
				}
			}
		}
	}

	return nil
}
