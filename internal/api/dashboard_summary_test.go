package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/database"
)

type dashboardSummaryTestLiveData struct {
	activeCalls []ActiveCallData
	statuses    []TRInstanceStatusData
	metrics     *IngestMetricsData
}

func (m *dashboardSummaryTestLiveData) ActiveCalls() []ActiveCallData { return m.activeCalls }
func (m *dashboardSummaryTestLiveData) LatestRecorders() []RecorderStateData { return nil }
func (m *dashboardSummaryTestLiveData) TRInstanceStatus() []TRInstanceStatusData { return m.statuses }
func (m *dashboardSummaryTestLiveData) UnitAffiliations() []UnitAffiliationData { return nil }
func (m *dashboardSummaryTestLiveData) Subscribe(EventFilter) (<-chan SSEEvent, func()) {
	return nil, func() {}
}
func (m *dashboardSummaryTestLiveData) ReplaySince(string, EventFilter) []SSEEvent { return nil }
func (m *dashboardSummaryTestLiveData) WatcherStatus() *WatcherStatusData { return nil }
func (m *dashboardSummaryTestLiveData) TranscriptionStatus() *TranscriptionStatusData { return nil }
func (m *dashboardSummaryTestLiveData) EnqueueTranscription(int64) bool { return false }
func (m *dashboardSummaryTestLiveData) TranscriptionQueueStats() *TranscriptionQueueStatsData { return nil }
func (m *dashboardSummaryTestLiveData) IngestMetrics() *IngestMetricsData { return m.metrics }
func (m *dashboardSummaryTestLiveData) MaintenanceStatus() *MaintenanceStatusData { return nil }
func (m *dashboardSummaryTestLiveData) RunMaintenance(context.Context) (*MaintenanceRunData, error) {
	return nil, nil
}
func (m *dashboardSummaryTestLiveData) SubmitBackfill(context.Context, BackfillFiltersData) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *dashboardSummaryTestLiveData) BackfillStatus() *BackfillStatusData { return nil }
func (m *dashboardSummaryTestLiveData) CancelBackfill(int) bool { return false }
func (m *dashboardSummaryTestLiveData) SetRetention(context.Context, string, time.Duration) error { return nil }
func (m *dashboardSummaryTestLiveData) DeleteRetention(context.Context, string) error { return nil }

func TestDashboardSummaryHandlerHealthyResponse(t *testing.T) {
	db := mustOpenDashboardSummaryTestDB(t)
	if err := ensureDashboardSummaryQueriesAvailable(db); err != nil {
		t.Skipf("dashboard summary queries unavailable in local DB: %v", err)
	}
	live := &dashboardSummaryTestLiveData{
		activeCalls: []ActiveCallData{{CallID: 123, SystemID: 1, Tgid: 1001, StartTime: time.Now().UTC()}},
		statuses:    []TRInstanceStatusData{{InstanceID: "tr-test", Status: "connected", LastSeen: time.Now().UTC()}},
	}
	h := NewDashboardSummaryHandler(db, live, "dev", time.Now().Add(-1*time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary?hours=24&top_limit=10", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	for _, key := range []string{"health", "stats", "active_calls", "top_talkgroups"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response missing key %q", key)
		}
	}

	var health dashboardSummaryHealth
	if err := json.Unmarshal(body["health"], &health); err != nil {
		t.Fatalf("failed to decode health: %v", err)
	}
	if health.Database != "ok" {
		t.Fatalf("health.database = %q, want ok", health.Database)
	}
	if health.Mqtt != "ok" {
		t.Fatalf("health.mqtt = %q, want ok", health.Mqtt)
	}

	var activeCalls []ActiveCallData
	if err := json.Unmarshal(body["active_calls"], &activeCalls); err != nil {
		t.Fatalf("failed to decode active_calls: %v", err)
	}
	if len(activeCalls) != 1 {
		t.Fatalf("active_calls len = %d, want 1", len(activeCalls))
	}

	var top dashboardSummaryTopTalkgroups
	if err := json.Unmarshal(body["top_talkgroups"], &top); err != nil {
		t.Fatalf("failed to decode top_talkgroups: %v", err)
	}
	if top.Hours != 24 {
		t.Fatalf("top_talkgroups.hours = %d, want 24", top.Hours)
	}
	if top.Limit != 10 {
		t.Fatalf("top_talkgroups.limit = %d, want 10", top.Limit)
	}
}

func TestDashboardSummaryHandlerHoursValidation(t *testing.T) {
	h := NewDashboardSummaryHandler(nil, nil, "dev", time.Now())

	for _, query := range []string{"hours=-1", "hours=721"} {
		t.Run(query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary?"+query, nil)
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDashboardSummaryHandlerTopLimitValidation(t *testing.T) {
	h := NewDashboardSummaryHandler(nil, nil, "dev", time.Now())

	for _, query := range []string{"top_limit=0", "top_limit=101"} {
		t.Run(query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary?"+query, nil)
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDashboardSummaryHandlerDBFailure(t *testing.T) {
	db := mustOpenDashboardSummaryTestDB(t)
	db.Close()

	h := NewDashboardSummaryHandler(db, nil, "dev", time.Now())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func mustOpenDashboardSummaryTestDB(t *testing.T) *database.DB {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = loadDashboardSummaryTestDatabaseURL(t)
	}
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set and .env did not provide one")
	}

	db, err := database.Connect(context.Background(), databaseURL, zerolog.Nop())
	if err != nil {
		t.Fatalf("failed to connect test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ensureDashboardSummaryQueriesAvailable(db *database.DB) error {
	if _, err := aggregateStats(context.Background(), db, defaultDashboardSummaryHours); err != nil {
		return err
	}
	_, _, err := db.GetTalkgroupActivity(context.Background(), database.TalkgroupActivityFilter{Limit: 1})
	return err
}

func loadDashboardSummaryTestDatabaseURL(t *testing.T) string {
	t.Helper()

	envPath := filepath.Clean(filepath.Join("..", "..", ".env"))
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "DATABASE_URL" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		return value
	}

	return ""
}
