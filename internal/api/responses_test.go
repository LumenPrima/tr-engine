package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ── ParsePagination ──────────────────────────────────────────────────

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{"defaults", "", 50, 0},
		{"valid_custom", "limit=25&offset=10", 25, 10},
		{"limit_over_1000_clamps", "limit=2000", 50, 0},
		{"limit_zero_clamps", "limit=0", 50, 0},
		{"negative_offset_clamps", "offset=-5", 50, 0},
		{"non_numeric_ignored", "limit=abc&offset=xyz", 50, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?"+tt.query, nil)
			p := ParsePagination(req)
			if p.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", p.Limit, tt.wantLimit)
			}
			if p.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", p.Offset, tt.wantOffset)
			}
		})
	}
}

// ── ParseSort ────────────────────────────────────────────────────────

func TestParseSort(t *testing.T) {
	allowed := map[string]string{
		"name":       "tg_alpha_tag",
		"start_time": "start_time",
		"id":         "call_id",
	}

	tests := []struct {
		name         string
		query        string
		defaultField string
		wantField    string
		wantDesc     bool
	}{
		{"no_sort_uses_default", "", "name", "name", false},
		{"default_with_dash_prefix", "", "-start_time", "start_time", true},
		{"explicit_sort_param", "sort=id", "name", "id", false},
		{"sort_dash_prefix", "sort=-start_time", "name", "start_time", true},
		{"sort_dir_desc", "sort=name&sort_dir=desc", "id", "name", true},
		{"invalid_field_falls_back", "sort=bogus", "name", "name", false},
		{"dash_invalid_field_uses_default", "sort=-bogus", "name", "name", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?"+tt.query, nil)
			s := ParseSort(req, tt.defaultField, allowed)
			if s.Field != tt.wantField {
				t.Errorf("Field = %q, want %q", s.Field, tt.wantField)
			}
			if s.Desc != tt.wantDesc {
				t.Errorf("Desc = %v, want %v", s.Desc, tt.wantDesc)
			}
		})
	}
}

// ── SortParam SQL helpers ────────────────────────────────────────────

func TestSortParamSQL(t *testing.T) {
	allowed := map[string]string{
		"name": "tg_alpha_tag",
		"id":   "call_id",
	}

	t.Run("SQLColumn_with_mapping", func(t *testing.T) {
		s := SortParam{Field: "name"}
		if got := s.SQLColumn(allowed); got != "tg_alpha_tag" {
			t.Errorf("SQLColumn = %q, want %q", got, "tg_alpha_tag")
		}
	})

	t.Run("SQLColumn_fallback", func(t *testing.T) {
		s := SortParam{Field: "unmapped"}
		if got := s.SQLColumn(allowed); got != "unmapped" {
			t.Errorf("SQLColumn = %q, want %q", got, "unmapped")
		}
	})

	t.Run("SQLDirection_ASC", func(t *testing.T) {
		s := SortParam{Desc: false}
		if got := s.SQLDirection(); got != "ASC" {
			t.Errorf("SQLDirection = %q, want %q", got, "ASC")
		}
	})

	t.Run("SQLDirection_DESC", func(t *testing.T) {
		s := SortParam{Desc: true}
		if got := s.SQLDirection(); got != "DESC" {
			t.Errorf("SQLDirection = %q, want %q", got, "DESC")
		}
	})

	t.Run("SQLOrderBy", func(t *testing.T) {
		s := SortParam{Field: "name", Desc: true}
		if got := s.SQLOrderBy(allowed); got != "tg_alpha_tag DESC" {
			t.Errorf("SQLOrderBy = %q, want %q", got, "tg_alpha_tag DESC")
		}
	})
}

// ── QueryInt ─────────────────────────────────────────────────────────

func TestQueryInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?n=42", nil)
		v, ok := QueryInt(req, "n")
		if !ok || v != 42 {
			t.Errorf("got (%d, %v), want (42, true)", v, ok)
		}
	})
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		_, ok := QueryInt(req, "n")
		if ok {
			t.Error("expected ok=false for missing param")
		}
	})
	t.Run("non_numeric", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?n=abc", nil)
		_, ok := QueryInt(req, "n")
		if ok {
			t.Error("expected ok=false for non-numeric param")
		}
	})
}

// ── QueryInt64 ───────────────────────────────────────────────────────

func TestQueryInt64(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?n=9999999999", nil)
		v, ok := QueryInt64(req, "n")
		if !ok || v != 9999999999 {
			t.Errorf("got (%d, %v), want (9999999999, true)", v, ok)
		}
	})
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		_, ok := QueryInt64(req, "n")
		if ok {
			t.Error("expected ok=false")
		}
	})
	t.Run("non_numeric", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?n=abc", nil)
		_, ok := QueryInt64(req, "n")
		if ok {
			t.Error("expected ok=false")
		}
	})
}

// ── QueryBool ────────────────────────────────────────────────────────

func TestQueryBool(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?flag=true", nil)
		v, ok := QueryBool(req, "flag")
		if !ok || !v {
			t.Errorf("got (%v, %v), want (true, true)", v, ok)
		}
	})
	t.Run("false", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?flag=false", nil)
		v, ok := QueryBool(req, "flag")
		if !ok || v {
			t.Errorf("got (%v, %v), want (false, true)", v, ok)
		}
	})
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		_, ok := QueryBool(req, "flag")
		if ok {
			t.Error("expected ok=false")
		}
	})
	t.Run("invalid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?flag=maybe", nil)
		_, ok := QueryBool(req, "flag")
		if ok {
			t.Error("expected ok=false")
		}
	})
}

// ── QueryString ──────────────────────────────────────────────────────

func TestQueryString(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?q=hello", nil)
		v, ok := QueryString(req, "q")
		if !ok || v != "hello" {
			t.Errorf("got (%q, %v), want (\"hello\", true)", v, ok)
		}
	})
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		_, ok := QueryString(req, "q")
		if ok {
			t.Error("expected ok=false")
		}
	})
}

// ── QueryTime ────────────────────────────────────────────────────────

func TestQueryTime(t *testing.T) {
	t.Run("valid_rfc3339", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?t=2024-01-15T10:30:00Z", nil)
		v, ok := QueryTime(req, "t")
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		if !v.Equal(want) {
			t.Errorf("got %v, want %v", v, want)
		}
	})
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		_, ok := QueryTime(req, "t")
		if ok {
			t.Error("expected ok=false")
		}
	})
	t.Run("invalid_format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?t=not-a-time", nil)
		_, ok := QueryTime(req, "t")
		if ok {
			t.Error("expected ok=false")
		}
	})
}

// ── QueryIntList ─────────────────────────────────────────────────────

func TestQueryIntList(t *testing.T) {
	t.Run("missing_returns_nil", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		got := QueryIntList(req, "ids")
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("single_value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=42", nil)
		got := QueryIntList(req, "ids")
		if len(got) != 1 || got[0] != 42 {
			t.Errorf("got %v, want [42]", got)
		}
	})
	t.Run("multiple_values", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=1,2,3", nil)
		got := QueryIntList(req, "ids")
		if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
			t.Errorf("got %v, want [1 2 3]", got)
		}
	})
	t.Run("skips_invalid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=1,abc,3", nil)
		got := QueryIntList(req, "ids")
		if len(got) != 2 || got[0] != 1 || got[1] != 3 {
			t.Errorf("got %v, want [1 3]", got)
		}
	})
}

// ── PathInt ──────────────────────────────────────────────────────────

func TestPathInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := newRequestWithChiParam("id", "42")
		v, err := PathInt(req, "id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 42 {
			t.Errorf("got %d, want 42", v)
		}
	})
	t.Run("missing", func(t *testing.T) {
		rctx := chi.NewRouteContext()
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		_, err := PathInt(req, "id")
		if err == nil {
			t.Error("expected error for missing param")
		}
	})
	t.Run("non_numeric", func(t *testing.T) {
		req := newRequestWithChiParam("id", "abc")
		_, err := PathInt(req, "id")
		if err == nil {
			t.Error("expected error for non-numeric param")
		}
	})
}

// ── PathInt64 ────────────────────────────────────────────────────────

func TestPathInt64(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := newRequestWithChiParam("id", "9999999999")
		v, err := PathInt64(req, "id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 9999999999 {
			t.Errorf("got %d, want 9999999999", v)
		}
	})
	t.Run("missing", func(t *testing.T) {
		rctx := chi.NewRouteContext()
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		_, err := PathInt64(req, "id")
		if err == nil {
			t.Error("expected error for missing param")
		}
	})
	t.Run("non_numeric", func(t *testing.T) {
		req := newRequestWithChiParam("id", "abc")
		_, err := PathInt64(req, "id")
		if err == nil {
			t.Error("expected error for non-numeric param")
		}
	})
}

// ── WriteJSON ────────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"msg": "ok"})

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if body["msg"] != "ok" {
		t.Errorf("body = %v, want msg=ok", body)
	}
}

// ── WriteError ───────────────────────────────────────────────────────

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if body.Error != "bad input" {
		t.Errorf("Error = %q, want %q", body.Error, "bad input")
	}
}

// ── DecodeJSON ───────────────────────────────────────────────────────

func TestDecodeJSON(t *testing.T) {
	t.Run("valid_body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
		var dst struct {
			Name string `json:"name"`
		}
		if err := DecodeJSON(req, &dst); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dst.Name != "test" {
			t.Errorf("Name = %q, want %q", dst.Name, "test")
		}
	})
	t.Run("nil_body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		req.Body = nil
		var dst struct{}
		if err := DecodeJSON(req, &dst); err == nil {
			t.Error("expected error for nil body")
		}
	})
	t.Run("malformed_json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{bad`))
		var dst struct{}
		if err := DecodeJSON(req, &dst); err == nil {
			t.Error("expected error for malformed JSON")
		}
	})
}

// ── WriteErrorDetail ─────────────────────────────────────────────────

func TestWriteErrorDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteErrorDetail(rec, http.StatusUnprocessableEntity, "validation failed", "name is required")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if body.Error != "validation failed" {
		t.Errorf("Error = %q, want %q", body.Error, "validation failed")
	}
	if body.Detail != "name is required" {
		t.Errorf("Detail = %q, want %q", body.Detail, "name is required")
	}
}
