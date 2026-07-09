package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

type mockRecentEventQuerier struct {
	filter database.RecentEventFilter
	events []database.RecentEventAPI
	total  int
	err    error
}

func (m *mockRecentEventQuerier) GetRecentEvents(_ context.Context, filter database.RecentEventFilter) ([]database.RecentEventAPI, int, error) {
	m.filter = filter
	return m.events, m.total, m.err
}

func TestRecentEventsHandler(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 34, 56, 0, time.UTC)
	unitTime := now.Add(-6 * time.Second)
	callSystemID := 1
	unitSystemID := 1
	unitSiteID := 1
	callTgid := 9178
	unitTgid := 9178
	unitID := 924003

	sampleEvents := []database.RecentEventAPI{
		{ID: "call:48531", Type: "call", Time: now, SystemID: &callSystemID, Tgid: &callTgid},
		{ID: "unit_event:123456", Type: "unit_event", Time: unitTime, SystemID: &unitSystemID, SiteID: &unitSiteID, Tgid: &unitTgid, UnitID: &unitID},
	}

	t.Run("mixed_call_and_unit_event_ordering", func(t *testing.T) {
		mock := &mockRecentEventQuerier{events: sampleEvents, total: 2}
		h := &RecentEventsHandler{querier: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/recent-events?limit=25", nil)

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var body struct {
			Events []database.RecentEventAPI `json:"events"`
			Total  int                       `json:"total"`
			Limit  int                       `json:"limit"`
			Offset int                       `json:"offset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON decode: %v", err)
		}
		if len(body.Events) != 2 {
			t.Fatalf("events len = %d, want 2", len(body.Events))
		}
		if body.Events[0].ID != "call:48531" || body.Events[1].ID != "unit_event:123456" {
			t.Fatalf("event order = [%s %s], want [call:48531 unit_event:123456]", body.Events[0].ID, body.Events[1].ID)
		}
		if body.Total != 2 || body.Limit != 25 || body.Offset != 0 {
			t.Fatalf("envelope = %+v, want total=2 limit=25 offset=0", body)
		}
	})

	t.Run("types_call_filter", func(t *testing.T) {
		mock := &mockRecentEventQuerier{events: sampleEvents[:1], total: 1}
		h := &RecentEventsHandler{querier: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/recent-events?types=call", nil)

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.Types) != 1 || mock.filter.Types[0] != "call" {
			t.Fatalf("Types = %v, want [call]", mock.filter.Types)
		}
	})

	t.Run("types_unit_event_filter", func(t *testing.T) {
		mock := &mockRecentEventQuerier{events: sampleEvents[1:], total: 1}
		h := &RecentEventsHandler{querier: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/recent-events?types=unit_event", nil)

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.Types) != 1 || mock.filter.Types[0] != "unit_event" {
			t.Fatalf("Types = %v, want [unit_event]", mock.filter.Types)
		}
	})

	t.Run("pagination_envelope", func(t *testing.T) {
		mock := &mockRecentEventQuerier{events: sampleEvents[:1], total: 7}
		h := &RecentEventsHandler{querier: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/recent-events?limit=10&offset=5&system_id=1&site_id=1&tgid=9178&unit_id=924003", nil)

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if mock.filter.Limit != 10 || mock.filter.Offset != 5 {
			t.Fatalf("filter pagination = limit:%d offset:%d, want limit:10 offset:5", mock.filter.Limit, mock.filter.Offset)
		}
		if len(mock.filter.SystemIDs) != 1 || mock.filter.SystemIDs[0] != 1 {
			t.Fatalf("SystemIDs = %v, want [1]", mock.filter.SystemIDs)
		}
		if len(mock.filter.SiteIDs) != 1 || mock.filter.SiteIDs[0] != 1 {
			t.Fatalf("SiteIDs = %v, want [1]", mock.filter.SiteIDs)
		}
		if len(mock.filter.Tgids) != 1 || mock.filter.Tgids[0] != 9178 {
			t.Fatalf("Tgids = %v, want [9178]", mock.filter.Tgids)
		}
		if len(mock.filter.UnitIDs) != 1 || mock.filter.UnitIDs[0] != 924003 {
			t.Fatalf("UnitIDs = %v, want [924003]", mock.filter.UnitIDs)
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON decode: %v", err)
		}
		if int(body["total"].(float64)) != 7 || int(body["limit"].(float64)) != 10 || int(body["offset"].(float64)) != 5 {
			t.Fatalf("response envelope = %#v, want total=7 limit=10 offset=5", body)
		}
	})

	t.Run("invalid_time_range_returns_400", func(t *testing.T) {
		mock := &mockRecentEventQuerier{}
		h := &RecentEventsHandler{querier: mock}
		rec := httptest.NewRecorder()
		start := now.Format(time.RFC3339)
		end := now.Add(-time.Minute).Format(time.RFC3339)
		req := httptest.NewRequest("GET", "/recent-events?start_time="+start+"&end_time="+end, nil)

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		var body ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON decode: %v", err)
		}
		if body.Code != ErrInvalidTimeRange {
			t.Fatalf("error code = %q, want %q", body.Code, ErrInvalidTimeRange)
		}
	})
}
