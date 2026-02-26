package database

import (
	"encoding/json"
	"time"
)

// NormalizeSrcFreqTimestamps converts int64 Unix "time" fields in a
// freq_list or src_list JSONB array to RFC 3339 strings. Returns the
// input unchanged on nil, null, empty array, or decode error.
func NormalizeSrcFreqTimestamps(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return raw
	}

	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return raw // malformed — pass through
	}

	changed := false
	for _, entry := range entries {
		v, ok := entry["time"]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64: // JSON numbers decode as float64
			if t == 0 {
				delete(entry, "time")
			} else {
				entry["time"] = time.Unix(int64(t), 0).UTC().Format(time.RFC3339)
			}
			changed = true
		case string:
			// already a string — leave as-is
		}
	}

	if !changed {
		return raw
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return raw
	}
	return out
}
