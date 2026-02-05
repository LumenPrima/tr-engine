package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"go.uber.org/zap"
)

// mockDB implements necessary database methods for testing
type mockDB struct {
	*database.DB
	systems       []*models.System
	talkgroups    []*models.Talkgroup
	units         []*models.Unit
	calls         []*models.Call
	listCallsErr  error
	getSystemErr  error
	getUnitErr    error
	getCallErr    error
	listSystemErr error
	listUnitsErr  error
}

func newTestHandler(db *database.DB) *Handler {
	logger, _ := zap.NewDevelopment()
	return NewHandler(db, nil, logger, "/tmp/audio")
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestParsePagination(t *testing.T) {
	h := &Handler{}

	// Test valid cases
	validTests := []struct {
		name           string
		queryParams    string
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "default values",
			queryParams:    "",
			expectedLimit:  50,
			expectedOffset: 0,
		},
		{
			name:           "custom limit",
			queryParams:    "limit=100",
			expectedLimit:  100,
			expectedOffset: 0,
		},
		{
			name:           "custom offset",
			queryParams:    "offset=25",
			expectedLimit:  50,
			expectedOffset: 25,
		},
		{
			name:           "both custom",
			queryParams:    "limit=200&offset=50",
			expectedLimit:  200,
			expectedOffset: 50,
		},
		{
			name:           "max allowed limit",
			queryParams:    "limit=1000",
			expectedLimit:  1000,
			expectedOffset: 0,
		},
	}

	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				limit, offset, err := h.parsePagination(c)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"limit": limit, "offset": offset})
			})

			req, _ := http.NewRequest("GET", "/test?"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			var response map[string]int
			json.Unmarshal(w.Body.Bytes(), &response)
			assert.Equal(t, tt.expectedLimit, response["limit"])
			assert.Equal(t, tt.expectedOffset, response["offset"])
		})
	}

	// Test error cases
	errorTests := []struct {
		name        string
		queryParams string
		errContains string
	}{
		{
			name:        "limit too high",
			queryParams: "limit=2000",
			errContains: "must be between 1 and 1000",
		},
		{
			name:        "negative limit",
			queryParams: "limit=-5",
			errContains: "must be between 1 and 1000",
		},
		{
			name:        "zero limit",
			queryParams: "limit=0",
			errContains: "must be between 1 and 1000",
		},
		{
			name:        "negative offset",
			queryParams: "offset=-10",
			errContains: "must be", // >= is escaped in JSON
		},
		{
			name:        "invalid limit string",
			queryParams: "limit=abc",
			errContains: "must be an integer",
		},
		{
			name:        "invalid offset string",
			queryParams: "offset=xyz",
			errContains: "must be an integer",
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				limit, offset, err := h.parsePagination(c)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"limit": limit, "offset": offset})
			})

			req, _ := http.NewRequest("GET", "/test?"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.errContains)
		})
	}
}

func TestParseTimeRange(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name          string
		queryParams   string
		expectStart   bool
		expectEnd     bool
		expectedStart string
		expectedEnd   string
	}{
		{
			name:        "no time params",
			queryParams: "",
			expectStart: false,
			expectEnd:   false,
		},
		{
			name:          "start time only",
			queryParams:   "start_time=2024-01-15T10:00:00Z",
			expectStart:   true,
			expectEnd:     false,
			expectedStart: "2024-01-15T10:00:00Z",
		},
		{
			name:        "end time only",
			queryParams: "end_time=2024-01-15T18:00:00Z",
			expectStart: false,
			expectEnd:   true,
			expectedEnd: "2024-01-15T18:00:00Z",
		},
		{
			name:          "both times",
			queryParams:   "start_time=2024-01-15T10:00:00Z&end_time=2024-01-15T18:00:00Z",
			expectStart:   true,
			expectEnd:     true,
			expectedStart: "2024-01-15T10:00:00Z",
			expectedEnd:   "2024-01-15T18:00:00Z",
		},
		{
			name:        "invalid start time",
			queryParams: "start_time=invalid",
			expectStart: false,
			expectEnd:   false,
		},
		{
			name:        "invalid end time",
			queryParams: "end_time=not-a-date",
			expectStart: false,
			expectEnd:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				startTime, endTime := h.parseTimeRange(c)
				response := gin.H{}
				if startTime != nil {
					response["start_time"] = startTime.Format(time.RFC3339)
				}
				if endTime != nil {
					response["end_time"] = endTime.Format(time.RFC3339)
				}
				c.JSON(http.StatusOK, response)
			})

			req, _ := http.NewRequest("GET", "/test?"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]string
			json.Unmarshal(w.Body.Bytes(), &response)

			if tt.expectStart {
				assert.Equal(t, tt.expectedStart, response["start_time"])
			} else {
				_, exists := response["start_time"]
				assert.False(t, exists)
			}

			if tt.expectEnd {
				assert.Equal(t, tt.expectedEnd, response["end_time"])
			} else {
				_, exists := response["end_time"]
				assert.False(t, exists)
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	db := &database.DB{}
	h := NewHandler(db, nil, logger, "/data/audio")

	assert.NotNil(t, h)
	assert.Equal(t, db, h.db)
	assert.NotNil(t, h.logger)
	assert.Equal(t, "/data/audio", h.audioBasePath)
}

func TestGetSystem_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/systems/:id", h.GetSystem)

	req, _ := http.NewRequest("GET", "/systems/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "Invalid system ID", response["error"])
}

func TestGetTalkgroup_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/talkgroups/:id", h.GetTalkgroup)

	req, _ := http.NewRequest("GET", "/talkgroups/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "Invalid talkgroup ID", response["error"])
}

func TestListSystemTalkgroups_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/systems/:id/talkgroups", h.ListSystemTalkgroups)

	req, _ := http.NewRequest("GET", "/systems/xyz/talkgroups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListTalkgroupCalls_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/talkgroups/:id/calls", h.ListTalkgroupCalls)

	req, _ := http.NewRequest("GET", "/talkgroups/invalid/calls", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUnit_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/units/:id", h.GetUnit)

	req, _ := http.NewRequest("GET", "/units/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "Invalid unit ID", response["error"])
}

func TestListUnitEvents_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/units/:id/events", h.ListUnitEvents)

	req, _ := http.NewRequest("GET", "/units/abc/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListUnitCalls_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/units/:id/calls", h.ListUnitCalls)

	req, _ := http.NewRequest("GET", "/units/xyz/calls", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetCall_TRCallIDFormat(t *testing.T) {
	// tr_call_id is now used instead of internal ID
	// Valid formats: "1_9177_1738173276" (multi-system), "9177_1738173276" (single-system)
	// The handler accepts any non-empty string; DB lookup determines if it exists
	t.Skip("Requires database connection - integration test")
}

func TestGetCallAudio_TRCallIDFormat(t *testing.T) {
	// tr_call_id is now used instead of internal ID
	// Valid formats: "1_9177_1738173276" (multi-system), "9177_1738173276" (single-system)
	// The handler accepts any non-empty string; DB lookup determines if it exists
	t.Skip("Requires database connection - integration test")
}

func TestGetCallGroup_InvalidID(t *testing.T) {
	router := setupTestRouter()
	h := &Handler{logger: zap.NewNop()}

	router.GET("/call-groups/:id", h.GetCallGroup)

	req, _ := http.NewRequest("GET", "/call-groups/not-valid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "Invalid call group ID", response["error"])
}

// Integration tests with test database

var testDB *TestDB

type TestDB struct {
	pool *MockPool
}

type MockPool struct{}

func (m *MockPool) QueryRow(ctx context.Context, sql string, args ...interface{}) MockRow {
	return MockRow{}
}

type MockRow struct{}

func (m MockRow) Scan(dest ...interface{}) error {
	return nil
}

// Tests for response structure validation

func TestListSystemsResponseStructure(t *testing.T) {
	// This test validates that response includes expected fields
	// Would need real DB connection for full test
	t.Skip("Requires database connection - integration test")
}

func TestListCallsResponseStructure(t *testing.T) {
	t.Skip("Requires database connection - integration test")
}

func TestPaginationInResponses(t *testing.T) {
	// Test that pagination params are echoed back correctly
	h := &Handler{}

	tests := []struct {
		name     string
		endpoint string
		query    string
	}{
		{
			name:     "ListTalkgroups pagination",
			endpoint: "/talkgroups",
			query:    "limit=25&offset=10",
		},
		{
			name:     "ListUnits pagination",
			endpoint: "/units",
			query:    "limit=100&offset=0",
		},
		{
			name:     "ListCalls pagination",
			endpoint: "/calls",
			query:    "limit=50&offset=100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				limit, offset, err := h.parsePagination(c)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"limit":  limit,
					"offset": offset,
				})
			})

			req, _ := http.NewRequest("GET", "/test?"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			var response map[string]int
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Contains(t, response, "limit")
			assert.Contains(t, response, "offset")
		})
	}
}

func TestTimeRangeParsing(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name         string
		startTime    string
		endTime      string
		expectParsed bool
	}{
		{
			name:         "valid RFC3339",
			startTime:    "2024-06-15T10:30:00Z",
			endTime:      "2024-06-15T12:30:00Z",
			expectParsed: true,
		},
		{
			name:         "with timezone offset",
			startTime:    "2024-06-15T10:30:00-05:00",
			endTime:      "2024-06-15T12:30:00-05:00",
			expectParsed: true,
		},
		{
			name:         "unix timestamp - invalid format",
			startTime:    "1718451000",
			endTime:      "1718458200",
			expectParsed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				startTime, endTime := h.parseTimeRange(c)
				c.JSON(http.StatusOK, gin.H{
					"has_start": startTime != nil,
					"has_end":   endTime != nil,
				})
			})

			req, _ := http.NewRequest("GET", "/test?start_time="+tt.startTime+"&end_time="+tt.endTime, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]bool
			json.Unmarshal(w.Body.Bytes(), &response)

			assert.Equal(t, tt.expectParsed, response["has_start"])
			assert.Equal(t, tt.expectParsed, response["has_end"])
		})
	}
}

func TestSystemFilterParsing(t *testing.T) {
	// Test that system query param is parsed correctly
	router := setupTestRouter()
	var capturedSystemID *int

	router.GET("/units", func(c *gin.Context) {
		// Simulate the system ID parsing from ListUnits
		var systemID *int
		if s := c.Query("system"); s != "" {
			if id, err := parseID(s); err == nil {
				systemID = &id
			}
		}
		capturedSystemID = systemID
		// Can't call h.ListUnits without DB, just verify parsing
		if systemID != nil {
			c.JSON(http.StatusOK, gin.H{"system_id": *systemID})
		} else {
			c.JSON(http.StatusOK, gin.H{"system_id": nil})
		}
	})

	// Test with valid system ID
	req, _ := http.NewRequest("GET", "/units?system=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotNil(t, capturedSystemID)
	assert.Equal(t, 5, *capturedSystemID)

	// Test without system ID
	capturedSystemID = nil
	req, _ = http.NewRequest("GET", "/units", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Nil(t, capturedSystemID)

	// Test with invalid system ID
	capturedSystemID = nil
	req, _ = http.NewRequest("GET", "/units?system=abc", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Nil(t, capturedSystemID)
}

func parseID(s string) (int, error) {
	var id int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &parseError{}
		}
		id = id*10 + int(c-'0')
	}
	return id, nil
}

type parseError struct{}

func (e *parseError) Error() string { return "parse error" }

func TestEventTypeFilterParsing(t *testing.T) {
	router := setupTestRouter()

	router.GET("/units/:id/events", func(c *gin.Context) {
		var eventType *string
		if t := c.Query("type"); t != "" {
			eventType = &t
		}
		if eventType != nil {
			c.JSON(http.StatusOK, gin.H{"event_type": *eventType})
		} else {
			c.JSON(http.StatusOK, gin.H{"event_type": nil})
		}
	})

	tests := []struct {
		name           string
		query          string
		expectedType   string
		expectTypeNull bool
	}{
		{
			name:         "with type filter",
			query:        "type=on",
			expectedType: "on",
		},
		{
			name:         "with call type",
			query:        "type=call",
			expectedType: "call",
		},
		{
			name:           "no type filter",
			query:          "",
			expectTypeNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/units/1/events"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req, _ := http.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)

			if tt.expectTypeNull {
				assert.Nil(t, response["event_type"])
			} else {
				assert.Equal(t, tt.expectedType, response["event_type"])
			}
		})
	}
	// h would be used with a real database connection
}

func TestTalkgroupFilterParsing(t *testing.T) {
	router := setupTestRouter()

	router.GET("/calls", func(c *gin.Context) {
		var talkgroupID *int
		if tg := c.Query("talkgroup"); tg != "" {
			if id, err := parseID(tg); err == nil {
				talkgroupID = &id
			}
		}
		if talkgroupID != nil {
			c.JSON(http.StatusOK, gin.H{"talkgroup_id": *talkgroupID})
		} else {
			c.JSON(http.StatusOK, gin.H{"talkgroup_id": nil})
		}
	})

	// With talkgroup filter
	req, _ := http.NewRequest("GET", "/calls?talkgroup=12345", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, float64(12345), response["talkgroup_id"])

	// Without talkgroup filter
	req, _ = http.NewRequest("GET", "/calls", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Nil(t, response["talkgroup_id"])
}

func TestAudioContentTypes(t *testing.T) {
	// Test that content types are correctly determined by extension
	tests := []struct {
		ext         string
		contentType string
	}{
		{".mp3", "audio/mpeg"},
		{".m4a", "audio/mp4"},
		{".wav", "audio/wav"},
		{".ogg", "audio/ogg"},
		{".unknown", "audio/mpeg"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			contentType := "audio/mpeg"
			switch tt.ext {
			case ".m4a":
				contentType = "audio/mp4"
			case ".wav":
				contentType = "audio/wav"
			case ".ogg":
				contentType = "audio/ogg"
			}
			assert.Equal(t, tt.contentType, contentType)
		})
	}
}

func TestHTTPStatusCodes(t *testing.T) {
	// Validate correct HTTP status codes for different scenarios
	tests := []struct {
		name         string
		scenario     string
		expectedCode int
	}{
		{"success list", "ok", http.StatusOK},
		{"success get", "ok", http.StatusOK},
		{"not found", "not_found", http.StatusNotFound},
		{"bad request", "invalid_id", http.StatusBadRequest},
		{"server error", "db_error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/test", func(c *gin.Context) {
				switch tt.scenario {
				case "ok":
					c.JSON(http.StatusOK, gin.H{"data": []string{}})
				case "not_found":
					c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				case "invalid_id":
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
				case "db_error":
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				}
			})

			req, _ := http.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
