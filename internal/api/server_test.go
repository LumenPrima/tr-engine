package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// Import metrics package to register metrics with prometheus
	_ "github.com/trunk-recorder/tr-engine/internal/metrics"
)

func TestMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set up the metrics endpoint the same way as in setupRoutes
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Create a test request
	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that the response contains prometheus format
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty body")
	}

	// Check for some expected metric names from our metrics package
	// Note: CounterVec metrics only appear after being used with labels,
	// but Gauge and Histogram metrics appear immediately
	expectedMetrics := []string{
		"trengine_websocket_clients",
		"trengine_systems",
		"trengine_call_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !containsString(body, metric) {
			t.Errorf("Expected metric %s not found in response", metric)
		}
	}

	t.Logf("Response body length: %d", len(body))
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
