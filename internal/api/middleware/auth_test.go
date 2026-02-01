package middleware

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trunk-recorder/tr-engine/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ============================================================================
// API Key Generation Tests
// ============================================================================

func TestGenerateAPIKey(t *testing.T) {
	plaintext, prefix := GenerateAPIKey()

	// Check format
	assert.True(t, strings.HasPrefix(plaintext, "tr_api_"), "key should start with tr_api_")
	assert.True(t, strings.HasPrefix(prefix, "tr_api_"), "prefix should start with tr_api_")

	// Check lengths
	assert.Equal(t, 71, len(plaintext), "full key should be 71 chars (7 prefix + 64 hex)")
	assert.Equal(t, 12, len(prefix), "prefix should be 12 chars")

	// Check uniqueness
	plaintext2, _ := GenerateAPIKey()
	assert.NotEqual(t, plaintext, plaintext2, "keys should be unique")
}

func TestHashAPIKey(t *testing.T) {
	key := "tr_api_test123"
	hash := HashAPIKey(key)

	// SHA-256 produces 64 hex characters
	assert.Equal(t, 64, len(hash), "hash should be 64 hex chars")

	// Same input should produce same hash
	hash2 := HashAPIKey(key)
	assert.Equal(t, hash, hash2, "same key should produce same hash")

	// Different input should produce different hash
	hash3 := HashAPIKey("tr_api_different")
	assert.NotEqual(t, hash, hash3, "different keys should produce different hashes")
}

// ============================================================================
// API Key Auth Middleware Tests (Config Keys)
// ============================================================================

func TestAPIKeyAuth_Disabled(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: false}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"test_key"},
	}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "missing authorization header")
}

func TestAPIKeyAuth_InvalidFormat(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"test_key"},
	}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123") // Wrong format
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid authorization format")
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"correct_key"},
	}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong_key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid API key")
}

func TestAPIKeyAuth_ValidConfigKey(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"valid_key_1", "valid_key_2"},
	}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		source, _ := c.Get("api_key_source")
		c.String(http.StatusOK, "source:%s", source)
	})

	// Test first key
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid_key_1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "source:config")

	// Test second key
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid_key_2")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_MultipleKeys(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		APIKeys: []string{"key1", "key2", "key3"},
	}, nil)

	router := gin.New()
	router.Use(auth.APIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Each key should work
	for _, key := range []string{"key1", "key2", "key3"} {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "key %s should be valid", key)
	}
}

// ============================================================================
// Dashboard Auth Middleware Tests
// ============================================================================

func TestDashboardAuth_Disabled(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: false}, nil)

	router := gin.New()
	router.Use(auth.DashboardAuth())
	router.GET("/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "dashboard")
	})

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDashboardAuth_NoSession(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)

	router := gin.New()
	router.Use(auth.DashboardAuth())
	router.GET("/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "dashboard")
	})

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/login", w.Header().Get("Location"))
}

func TestDashboardAuth_ValidSession(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)

	// Create a session
	token := auth.createSession()

	router := gin.New()
	router.Use(auth.DashboardAuth())
	router.GET("/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "dashboard")
	})

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "dashboard", w.Body.String())
}

func TestDashboardAuth_InvalidSession(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)

	router := gin.New()
	router.Use(auth.DashboardAuth())
	router.GET("/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "dashboard")
	})

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "invalid_token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/login", w.Header().Get("Location"))
}

// ============================================================================
// Session Management Tests
// ============================================================================

func TestCreateSession(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{}, nil)

	token1 := auth.createSession()
	token2 := auth.createSession()

	// Tokens should be unique
	assert.NotEqual(t, token1, token2)

	// Tokens should be 64 hex chars
	assert.Equal(t, 64, len(token1))
	assert.Equal(t, 64, len(token2))

	// Both should be valid
	assert.True(t, auth.validateSession(token1))
	assert.True(t, auth.validateSession(token2))
}

func TestValidateSession_Empty(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{}, nil)
	assert.False(t, auth.validateSession(""))
}

func TestValidateSession_Invalid(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{}, nil)
	assert.False(t, auth.validateSession("nonexistent_token"))
}

func TestCleanupSessions(t *testing.T) {
	auth := &AuthMiddleware{
		config: config.AuthConfig{},
	}

	// Add an expired session directly
	expiredToken := "expired_session"
	auth.sessions.Store(expiredToken, time.Now().Add(-1*time.Hour))

	// Add a valid session
	validToken := "valid_session"
	auth.sessions.Store(validToken, time.Now().Add(1*time.Hour))

	// Run cleanup
	auth.CleanupSessions()

	// Expired should be removed
	_, exists := auth.sessions.Load(expiredToken)
	assert.False(t, exists, "expired session should be removed")

	// Valid should remain
	_, exists = auth.sessions.Load(validToken)
	assert.True(t, exists, "valid session should remain")
}

// ============================================================================
// Login Tests
// ============================================================================

func TestLogin_Disabled(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: false}, nil)

	router := gin.New()
	router.POST("/login", auth.Login)

	req, _ := http.NewRequest("POST", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/dashboard", w.Header().Get("Location"))
}

func TestLogin_InvalidCredentials(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		Dashboard: config.DashboardAuth{
			Username: "admin",
			Password: "secret",
		},
	}, nil)

	router := gin.New()
	router.SetHTMLTemplate(createTestTemplate())
	router.POST("/login", auth.Login)

	req, _ := http.NewRequest("POST", "/login", strings.NewReader("username=admin&password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_ValidCredentials(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		Dashboard: config.DashboardAuth{
			Username: "admin",
			Password: "secret",
		},
	}, nil)

	router := gin.New()
	router.POST("/login", auth.Login)

	req, _ := http.NewRequest("POST", "/login", strings.NewReader("username=admin&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/dashboard", w.Header().Get("Location"))

	// Check that session cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "session cookie should be set")
	assert.NotEmpty(t, sessionCookie.Value)
}

func TestLogin_WithRedirect(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{
		Enabled: true,
		Dashboard: config.DashboardAuth{
			Username: "admin",
			Password: "secret",
		},
	}, nil)

	router := gin.New()
	router.POST("/login", auth.Login)

	req, _ := http.NewRequest("POST", "/login?redirect=/recorders", strings.NewReader("username=admin&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/recorders", w.Header().Get("Location"))
}

// ============================================================================
// Logout Tests
// ============================================================================

func TestLogout(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)

	// Create a session first
	token := auth.createSession()
	assert.True(t, auth.validateSession(token))

	router := gin.New()
	router.GET("/logout", auth.Logout)

	req, _ := http.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/login", w.Header().Get("Location"))

	// Session should be invalidated
	assert.False(t, auth.validateSession(token))

	// Cookie should be cleared (max-age = -1 or expires in past)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			assert.True(t, c.MaxAge < 0 || c.Expires.Before(time.Now()))
		}
	}
}

// ============================================================================
// Login Page Tests
// ============================================================================

func TestLoginPage_Disabled(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: false}, nil)

	router := gin.New()
	router.GET("/login", auth.LoginPage)

	req, _ := http.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/dashboard", w.Header().Get("Location"))
}

func TestLoginPage_AlreadyLoggedIn(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)
	token := auth.createSession()

	router := gin.New()
	router.GET("/login", auth.LoginPage)

	req, _ := http.NewRequest("GET", "/login", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/dashboard", w.Header().Get("Location"))
}

func TestLoginPage_NotLoggedIn(t *testing.T) {
	auth := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)

	router := gin.New()
	router.SetHTMLTemplate(createTestTemplate())
	router.GET("/login", auth.LoginPage)

	req, _ := http.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ============================================================================
// IsEnabled Tests
// ============================================================================

func TestIsEnabled(t *testing.T) {
	authEnabled := NewAuthMiddleware(config.AuthConfig{Enabled: true}, nil)
	authDisabled := NewAuthMiddleware(config.AuthConfig{Enabled: false}, nil)

	assert.True(t, authEnabled.IsEnabled())
	assert.False(t, authDisabled.IsEnabled())
}

// ============================================================================
// Helper Functions
// ============================================================================

func createTestTemplate() *template.Template {
	tmpl := template.New("login.html")
	tmpl, _ = tmpl.Parse(`<!DOCTYPE html><html><body>{{if .Error}}{{.Error}}{{end}}Login</body></html>`)
	return tmpl
}
