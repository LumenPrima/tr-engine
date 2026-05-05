package database

import "testing"

func TestEffectiveTalkgroupPatchSource(t *testing.T) {
	text := "value"
	priority := 1

	tests := []struct {
		name    string
		current string
		got     string
		want    string
	}{
		{"no mutable fields preserves empty source", "", effectiveTalkgroupPatchSource("", nil, nil, nil, nil, nil), ""},
		{"no mutable fields preserves explicit source", "csv", effectiveTalkgroupPatchSource("csv", nil, nil, nil, nil, nil), "csv"},
		{"alpha tag marks manual", "", effectiveTalkgroupPatchSource("", &text, nil, nil, nil, nil), "manual"},
		{"description marks manual", "", effectiveTalkgroupPatchSource("", nil, &text, nil, nil, nil), "manual"},
		{"group marks manual", "", effectiveTalkgroupPatchSource("", nil, nil, &text, nil, nil), "manual"},
		{"tag marks manual", "", effectiveTalkgroupPatchSource("", nil, nil, nil, &text, nil), "manual"},
		{"priority marks manual", "", effectiveTalkgroupPatchSource("", nil, nil, nil, nil, &priority), "manual"},
		{"mutable field overrides explicit source", "csv", effectiveTalkgroupPatchSource("csv", &text, nil, nil, nil, nil), "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("effectiveTalkgroupPatchSource(%q) = %q, want %q", tt.current, tt.got, tt.want)
			}
		})
	}
}
