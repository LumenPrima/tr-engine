package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

// mockUnitEventQuerier implements unitEventQuerier for testing.
type mockUnitEventQuerier struct {
	filter database.GlobalUnitEventFilter // last filter received
	events []database.UnitEventAPI
	total  int
	err    error
}

func (m *mockUnitEventQuerier) ListUnitEventsGlobal(_ context.Context, filter database.GlobalUnitEventFilter) ([]database.UnitEventAPI, int, error) {
	m.filter = filter
	return m.events, m.total, m.err
}

func TestListUnitEventsGlobal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	sampleEvents := []database.UnitEventAPI{
		{ID: 1, EventType: "call", Time: now, SystemID: 1, UnitRID: 100, Tgid: intPtr(200)},
		{ID: 2, EventType: "end", Time: now, SystemID: 1, UnitRID: 101, Tgid: intPtr(201)},
	}

	t.Run("missing_system_filter_returns_400", func(t *testing.T) {
		h := &UnitEventsHandler{db: &mockUnitEventQuerier{}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		var body ErrorResponse
		json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("system_id_filter_accepted", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.SystemIDs) != 1 || mock.filter.SystemIDs[0] != 1 {
			t.Errorf("SystemIDs = %v, want [1]", mock.filter.SystemIDs)
		}
	})

	t.Run("sysid_filter_accepted", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?sysid=BEE00", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.Sysids) != 1 || mock.filter.Sysids[0] != "BEE00" {
			t.Errorf("Sysids = %v, want [BEE00]", mock.filter.Sysids)
		}
	})

	t.Run("time_range_exceeds_24h_returns_400", func(t *testing.T) {
		mock := &mockUnitEventQuerier{}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		start := now.Add(-25 * time.Hour).Format(time.RFC3339)
		end := now.Format(time.RFC3339)
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&start_time="+start+"&end_time="+end, nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("time_range_within_24h_accepted", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		start := now.Add(-12 * time.Hour).Format(time.RFC3339)
		end := now.Format(time.RFC3339)
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&start_time="+start+"&end_time="+end, nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if mock.filter.StartTime == nil || mock.filter.EndTime == nil {
			t.Fatal("expected StartTime and EndTime to be set")
		}
	})

	t.Run("event_type_filter_parsed", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&type=call,end", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.EventTypes) != 2 {
			t.Fatalf("EventTypes len = %d, want 2", len(mock.filter.EventTypes))
		}
		if mock.filter.EventTypes[0] != "call" || mock.filter.EventTypes[1] != "end" {
			t.Errorf("EventTypes = %v, want [call end]", mock.filter.EventTypes)
		}
	})

	t.Run("multi_value_filters", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&unit_id=1,2&tgid=100,200", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if len(mock.filter.UnitIDs) != 2 || mock.filter.UnitIDs[0] != 1 || mock.filter.UnitIDs[1] != 2 {
			t.Errorf("UnitIDs = %v, want [1 2]", mock.filter.UnitIDs)
		}
		if len(mock.filter.Tgids) != 2 || mock.filter.Tgids[0] != 100 || mock.filter.Tgids[1] != 200 {
			t.Errorf("Tgids = %v, want [100 200]", mock.filter.Tgids)
		}
	})

	t.Run("emergency_filter", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&emergency=true", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if mock.filter.Emergency == nil || !*mock.filter.Emergency {
			t.Errorf("Emergency = %v, want &true", mock.filter.Emergency)
		}
	})

	t.Run("pagination_passed_through", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents[:1], total: 10}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&limit=10&offset=5", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if mock.filter.Limit != 10 {
			t.Errorf("Limit = %d, want 10", mock.filter.Limit)
		}
		if mock.filter.Offset != 5 {
			t.Errorf("Offset = %d, want 5", mock.filter.Offset)
		}

		var body map[string]any
		json.Unmarshal(rec.Body.Bytes(), &body)
		if int(body["limit"].(float64)) != 10 {
			t.Errorf("response limit = %v, want 10", body["limit"])
		}
		if int(body["offset"].(float64)) != 5 {
			t.Errorf("response offset = %v, want 5", body["offset"])
		}
	})

	t.Run("sort_param_applied", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1&sort=-unit_rid", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		want := "ue.unit_rid DESC"
		if mock.filter.Sort != want {
			t.Errorf("Sort = %q, want %q", mock.filter.Sort, want)
		}
	})

	t.Run("db_error_returns_500", func(t *testing.T) {
		mock := &mockUnitEventQuerier{err: errors.New("connection refused")}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1", nil)
		h.ListUnitEventsGlobal(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("response_body_structure", func(t *testing.T) {
		mock := &mockUnitEventQuerier{events: sampleEvents, total: 2}
		h := &UnitEventsHandler{db: mock}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/unit-events?system_id=1", nil)
		h.ListUnitEventsGlobal(rec, req)

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON decode: %v", err)
		}
		events, ok := body["events"].([]any)
		if !ok {
			t.Fatal("expected events array in response")
		}
		if len(events) != 2 {
			t.Errorf("events len = %d, want 2", len(events))
		}
		if int(body["total"].(float64)) != 2 {
			t.Errorf("total = %v, want 2", body["total"])
		}
	})
}

func intPtr(v int) *int {
	return &v
}
