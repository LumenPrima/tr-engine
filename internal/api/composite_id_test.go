package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

// newRequestWithChiParam builds a request with a chi URL param injected into context.
func newRequestWithChiParam(param, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, value)
	req := httptest.NewRequest("GET", "/", nil)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestWriteAmbiguous(t *testing.T) {
	rec := httptest.NewRecorder()
	matches := []database.AmbiguousMatch{
		{SystemID: 1, SystemName: "butco", Sysid: "348"},
		{SystemID: 2, SystemName: "warco", Sysid: "34D"},
	}
	WriteAmbiguous(rec, 100, matches)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	var body AmbiguousErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if !strings.Contains(body.Error, "100") {
		t.Errorf("Error = %q, expected entity ID 100 in message", body.Error)
	}
	if len(body.Matches) != 2 {
		t.Fatalf("Matches len = %d, want 2", len(body.Matches))
	}
	if body.Matches[0].SystemName != "butco" {
		t.Errorf("first match system = %q, want butco", body.Matches[0].SystemName)
	}
}

func TestParseCompositeID(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantSysID  int
		wantEntID  int
		wantPlain  bool
		wantErr    bool
	}{
		{"composite", "1:100", 1, 100, false, false},
		{"plain", "100", 0, 100, true, false},
		{"missing_param", "", 0, 0, false, true},
		{"invalid_system_id", "abc:100", 0, 0, false, true},
		{"invalid_entity_id", "1:abc", 0, 0, false, true},
		{"non_numeric_plain", "abc", 0, 0, false, true},
		{"composite_dash", "3-48686", 3, 48686, false, false},
		{"dash_large_ids", "1-1234567", 1, 1234567, false, false},
		{"invalid_dash_system", "abc-100", 0, 0, false, true},
		{"invalid_dash_entity", "1-abc", 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.value == "" {
				// No chi param at all
				req = httptest.NewRequest("GET", "/", nil)
				rctx := chi.NewRouteContext()
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			} else {
				req = newRequestWithChiParam("id", tt.value)
			}

			cid, err := ParseCompositeID(req, "id")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cid.SystemID != tt.wantSysID {
				t.Errorf("SystemID = %d, want %d", cid.SystemID, tt.wantSysID)
			}
			if cid.EntityID != tt.wantEntID {
				t.Errorf("EntityID = %d, want %d", cid.EntityID, tt.wantEntID)
			}
			if cid.IsPlain != tt.wantPlain {
				t.Errorf("IsPlain = %v, want %v", cid.IsPlain, tt.wantPlain)
			}
		})
	}
}
