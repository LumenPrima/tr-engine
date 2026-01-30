package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/api/rest"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
	"go.uber.org/zap"
)

var testDB *testutil.TestDB
var handler *rest.Handler
var router *gin.Engine

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// Set up shared test database
	opts := testutil.DefaultTestDBOptions()
	testDB = testutil.NewTestDBForMain(opts)
	if testDB == nil {
		os.Exit(1)
	}

	// Create handler with real DB
	logger, _ := zap.NewDevelopment()
	handler = rest.NewHandler(testDB.DB, nil, logger, "/tmp/audio")

	// Set up router with API routes
	router = gin.New()
	api := router.Group("/api/v1")
	{
		// Systems
		api.GET("/systems", handler.ListSystems)
		api.GET("/systems/:id", handler.GetSystem)
		api.GET("/systems/:id/talkgroups", handler.ListSystemTalkgroups)

		// Talkgroups
		api.GET("/talkgroups", handler.ListTalkgroups)
		api.GET("/talkgroups/:id", handler.GetTalkgroup)
		api.GET("/talkgroups/:id/calls", handler.ListTalkgroupCalls)

		// Units
		api.GET("/units", handler.ListUnits)
		api.GET("/units/active", handler.ListActiveUnits)
		api.GET("/units/:id", handler.GetUnit)
		api.GET("/units/:id/events", handler.ListUnitEvents)
		api.GET("/units/:id/calls", handler.ListUnitCalls)

		// Calls
		api.GET("/calls", handler.ListCalls)
		api.GET("/calls/recent", handler.GetRecentCalls)
		api.GET("/calls/:id", handler.GetCall)
		api.GET("/calls/:id/transmissions", handler.GetCallTransmissions)
		api.GET("/calls/:id/frequencies", handler.GetCallFrequencies)

		// Stats
		api.GET("/stats", handler.GetStats)
	}

	code := m.Run()

	testDB.Close()
	os.Exit(code)
}

func setupTest(t *testing.T) *testutil.TestFixtures {
	t.Helper()
	testDB.Reset(t)
	return testutil.SeedTestData(t, testDB)
}

// ============================================================================
// System Endpoint Tests
// ============================================================================

func TestListSystems_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/systems", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Systems []struct {
			ID        int    `json:"id"`
			ShortName string `json:"short_name"`
			SysID     string `json:"sysid"`
		} `json:"systems"`
		Count int `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Systems, 2)

	// Verify systems are present
	shortNames := make(map[string]bool)
	for _, sys := range response.Systems {
		shortNames[sys.ShortName] = true
	}
	assert.True(t, shortNames["metro"])
	assert.True(t, shortNames["county"])
}

func TestGetSystem_Integration(t *testing.T) {
	f := setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/systems/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		ID        int    `json:"id"`
		ShortName string `json:"short_name"`
		SysID     string `json:"sysid"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, f.MetroSystemID, response.ID)
	assert.Equal(t, "metro", response.ShortName)
	assert.Equal(t, "348", response.SysID)
}

func TestGetSystem_NotFound(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/systems/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ============================================================================
// Talkgroup Endpoint Tests
// ============================================================================

func TestListTalkgroups_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/talkgroups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Talkgroups []struct {
			ID       int    `json:"id"`
			SYSID    string `json:"sysid"`
			TGID     int    `json:"tgid"`
			AlphaTag string `json:"alpha_tag"`
		} `json:"talkgroups"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Len(t, response.Talkgroups, 4)
}

func TestGetTalkgroup_BySysidAndTgid(t *testing.T) {
	_ = setupTest(t)

	// Test sysid:tgid format
	req, _ := http.NewRequest("GET", "/api/v1/talkgroups/348:9178", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		ID       int    `json:"id"`
		SYSID    string `json:"sysid"`
		TGID     int    `json:"tgid"`
		AlphaTag string `json:"alpha_tag"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "348", response.SYSID)
	assert.Equal(t, 9178, response.TGID)
	assert.Equal(t, "Metro PD Main", response.AlphaTag)
}

func TestGetTalkgroup_AmbiguousTgid_Returns409(t *testing.T) {
	_ = setupTest(t)

	// TGID 9178 exists in both systems, so plain lookup should return 409
	req, _ := http.NewRequest("GET", "/api/v1/talkgroups/9178", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response struct {
		Error      string `json:"error"`
		Message    string `json:"message"`
		Resolution string `json:"resolution"`
		Systems    []struct {
			SYSID    string `json:"sysid"`
			AlphaTag string `json:"alpha_tag"`
		} `json:"systems"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ambiguous_identifier", response.Error)
	assert.Len(t, response.Systems, 2)
}

func TestGetTalkgroup_UniqueTgid_ReturnsOK(t *testing.T) {
	_ = setupTest(t)

	// TGID 9200 only exists in metro system, so plain lookup should work
	req, _ := http.NewRequest("GET", "/api/v1/talkgroups/9200", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		SYSID    string `json:"sysid"`
		TGID     int    `json:"tgid"`
		AlphaTag string `json:"alpha_tag"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "348", response.SYSID)
	assert.Equal(t, 9200, response.TGID)
	assert.Equal(t, "Metro Fire", response.AlphaTag)
}

func TestGetTalkgroup_ByDatabaseID(t *testing.T) {
	f := setupTest(t)

	// Test id: format for database ID lookup
	req, _ := http.NewRequest("GET", "/api/v1/talkgroups/id:3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		ID       int    `json:"id"`
		AlphaTag string `json:"alpha_tag"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, f.MetroFireID, response.ID)
	assert.Equal(t, "Metro Fire", response.AlphaTag)
}

func TestListTalkgroups_WithSearch(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/talkgroups?search=Fire", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Talkgroups []struct {
			AlphaTag string `json:"alpha_tag"`
		} `json:"talkgroups"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Len(t, response.Talkgroups, 1)
	assert.Equal(t, "Metro Fire", response.Talkgroups[0].AlphaTag)
}

// ============================================================================
// Unit Endpoint Tests
// ============================================================================

func TestListUnits_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/units", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Units []struct {
			ID       int    `json:"id"`
			SYSID    string `json:"sysid"`
			UnitID   int64  `json:"unit_id"`
			AlphaTag string `json:"alpha_tag"`
		} `json:"units"`
		Count int `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.Count)
}

func TestGetUnit_BySysidAndUnitID(t *testing.T) {
	_ = setupTest(t)

	// Test sysid:unit_id format
	req, _ := http.NewRequest("GET", "/api/v1/units/348:1001234", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		SYSID    string `json:"sysid"`
		UnitID   int64  `json:"unit_id"`
		AlphaTag string `json:"alpha_tag"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "348", response.SYSID)
	assert.Equal(t, int64(1001234), response.UnitID)
	assert.Equal(t, "Metro Car 123", response.AlphaTag)
}

func TestGetUnit_AmbiguousUnitID_Returns409(t *testing.T) {
	_ = setupTest(t)

	// Unit 1001234 exists in both systems
	req, _ := http.NewRequest("GET", "/api/v1/units/1001234", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response struct {
		Error   string `json:"error"`
		Systems []struct {
			SYSID string `json:"sysid"`
		} `json:"systems"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ambiguous_identifier", response.Error)
	assert.Len(t, response.Systems, 2)
}

// ============================================================================
// Call Endpoint Tests
// ============================================================================

func TestListCalls_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Calls []struct {
			ID        int64  `json:"id"`
			TRCallID  string `json:"tr_call_id"`
			AudioPath string `json:"audio_path"`
			AudioURL  string `json:"audio_url"`
		} `json:"calls"`
		Count int `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Should only include calls with audio (4 of 5 fixtures have audio)
	assert.Equal(t, 4, response.Count)

	// Verify audio_url is populated
	for _, call := range response.Calls {
		if call.AudioPath != "" {
			assert.NotEmpty(t, call.AudioURL)
			assert.Contains(t, call.AudioURL, "/api/v1/calls/")
		}
	}
}

func TestListCalls_FilterBySystem(t *testing.T) {
	f := setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls?system=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Calls []struct {
			SystemID int `json:"system_id"`
		} `json:"calls"`
		Count int `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Metro has 3 calls with audio (call 5 is encrypted without audio)
	assert.Equal(t, 3, response.Count)
	for _, call := range response.Calls {
		assert.Equal(t, f.MetroSystemID, call.SystemID)
	}
}

func TestGetCall_ByID(t *testing.T) {
	f := setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		ID       int64  `json:"id"`
		TRCallID string `json:"tr_call_id"`
		AudioURL string `json:"audio_url"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, f.Call1ID, response.ID)
	assert.Equal(t, "1705312200_850387500_9178", response.TRCallID)
	assert.NotEmpty(t, response.AudioURL)
}

func TestGetCall_ByTRCallID(t *testing.T) {
	f := setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls/1705312200_850387500_9178", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		ID       int64  `json:"id"`
		TRCallID string `json:"tr_call_id"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, f.Call1ID, response.ID)
}

func TestGetCallTransmissions_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls/1/transmissions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Transmissions []struct {
			UnitRID  int64   `json:"unit_rid"`
			Position float32 `json:"position"`
			Duration float32 `json:"duration"`
		} `json:"transmissions"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Len(t, response.Transmissions, 3)

	// Verify order by position
	assert.Equal(t, float32(0.0), response.Transmissions[0].Position)
	assert.Equal(t, float32(5.5), response.Transmissions[1].Position)
	assert.Equal(t, float32(10.5), response.Transmissions[2].Position)
}

// ============================================================================
// Pagination Tests
// ============================================================================

func TestListCalls_Pagination(t *testing.T) {
	_ = setupTest(t)

	// First page
	req, _ := http.NewRequest("GET", "/api/v1/calls?limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response1 struct {
		Calls  []struct{ ID int64 } `json:"calls"`
		Count  int                  `json:"count"`
		Limit  int                  `json:"limit"`
		Offset int                  `json:"offset"`
	}
	json.Unmarshal(w.Body.Bytes(), &response1)

	assert.Equal(t, 2, response1.Count)
	assert.Equal(t, 2, response1.Limit)
	assert.Equal(t, 0, response1.Offset)

	// Second page
	req, _ = http.NewRequest("GET", "/api/v1/calls?limit=2&offset=2", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response2 struct {
		Calls  []struct{ ID int64 } `json:"calls"`
		Count  int                  `json:"count"`
		Offset int                  `json:"offset"`
	}
	json.Unmarshal(w.Body.Bytes(), &response2)

	assert.Equal(t, 2, response2.Count)
	assert.Equal(t, 2, response2.Offset)

	// Verify no overlap
	for _, c1 := range response1.Calls {
		for _, c2 := range response2.Calls {
			assert.NotEqual(t, c1.ID, c2.ID)
		}
	}
}

// ============================================================================
// Recent/Active Calls Tests
// ============================================================================

func TestGetRecentCalls_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/calls/recent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Calls []struct {
			ID       int64  `json:"id"`
			AudioURL string `json:"audio_url"`
			HasAudio bool   `json:"has_audio"`
		} `json:"calls"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// All returned calls should have audio
	for _, call := range response.Calls {
		assert.True(t, call.HasAudio)
	}
}

// ============================================================================
// Stats Endpoint Tests
// ============================================================================

func TestGetStats_Integration(t *testing.T) {
	_ = setupTest(t)

	req, _ := http.NewRequest("GET", "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		TotalSystems    int `json:"total_systems"`
		TotalTalkgroups int `json:"total_talkgroups"`
		TotalUnits      int `json:"total_units"`
		TotalCalls      int `json:"total_calls"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalSystems)
	assert.Equal(t, 4, response.TotalTalkgroups)
	assert.Equal(t, 3, response.TotalUnits)
	assert.Equal(t, 5, response.TotalCalls)
}
