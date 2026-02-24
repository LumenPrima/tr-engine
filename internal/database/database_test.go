package database

import (
	"testing"
)

// ── maskDSN ──────────────────────────────────────────────────────────

func TestMaskDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			"password_masked",
			"postgres://user:secret@localhost:5432/db",
			"postgres://user:%2A%2A%2A@localhost:5432/db",
		},
		{
			"no_password_unchanged",
			"postgres://localhost:5432/db",
			"postgres://localhost:5432/db",
		},
		{
			"malformed_returns_stars",
			"://bad\x00url",
			"***",
		},
		{
			"user_no_password",
			"postgres://user@localhost:5432/db",
			"postgres://user@localhost:5432/db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("maskDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

