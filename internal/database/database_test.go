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

// ── queryBuilder ─────────────────────────────────────────────────────

func TestQueryBuilder(t *testing.T) {
	t.Run("empty_returns_empty_string", func(t *testing.T) {
		qb := newQueryBuilder()
		if got := qb.WhereClause(); got != "" {
			t.Errorf("WhereClause = %q, want empty", got)
		}
		if args := qb.Args(); len(args) != 0 {
			t.Errorf("Args = %v, want empty", args)
		}
	})

	t.Run("single_Add", func(t *testing.T) {
		qb := newQueryBuilder()
		qb.Add("system_id = %s", 42)
		if got := qb.WhereClause(); got != " WHERE system_id = $1" {
			t.Errorf("WhereClause = %q", got)
		}
		args := qb.Args()
		if len(args) != 1 || args[0] != 42 {
			t.Errorf("Args = %v, want [42]", args)
		}
	})

	t.Run("multiple_Add_joins_with_AND", func(t *testing.T) {
		qb := newQueryBuilder()
		qb.Add("system_id = %s", 1)
		qb.Add("tgid = %s", 200)
		want := " WHERE system_id = $1 AND tgid = $2"
		if got := qb.WhereClause(); got != want {
			t.Errorf("WhereClause = %q, want %q", got, want)
		}
		args := qb.Args()
		if len(args) != 2 || args[0] != 1 || args[1] != 200 {
			t.Errorf("Args = %v, want [1 200]", args)
		}
	})

	t.Run("AddRaw_no_params", func(t *testing.T) {
		qb := newQueryBuilder()
		qb.AddRaw("emergency = true")
		if got := qb.WhereClause(); got != " WHERE emergency = true" {
			t.Errorf("WhereClause = %q", got)
		}
		if args := qb.Args(); len(args) != 0 {
			t.Errorf("Args = %v, want empty", args)
		}
	})

	t.Run("mixed_Add_and_AddRaw", func(t *testing.T) {
		qb := newQueryBuilder()
		qb.Add("system_id = %s", 5)
		qb.AddRaw("encrypted = false")
		qb.Add("tgid = %s", 100)
		want := " WHERE system_id = $1 AND encrypted = false AND tgid = $2"
		if got := qb.WhereClause(); got != want {
			t.Errorf("WhereClause = %q, want %q", got, want)
		}
		args := qb.Args()
		if len(args) != 2 || args[0] != 5 || args[1] != 100 {
			t.Errorf("Args = %v, want [5 100]", args)
		}
	})

	t.Run("arg_index_increments_correctly", func(t *testing.T) {
		qb := newQueryBuilder()
		qb.Add("a = %s", "x")
		qb.Add("b = %s", "y")
		qb.Add("c = %s", "z")
		args := qb.Args()
		if len(args) != 3 {
			t.Fatalf("len(Args) = %d, want 3", len(args))
		}
		if args[0] != "x" || args[1] != "y" || args[2] != "z" {
			t.Errorf("Args = %v, want [x y z]", args)
		}
	})
}
