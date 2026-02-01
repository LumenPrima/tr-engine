package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/trunk-recorder/tr-engine/internal/config"
)

const (
	sessionCookieName = "tr_session"
	sessionDuration   = 24 * time.Hour
)

// AuthMiddleware handles authentication for the API server
type AuthMiddleware struct {
	config   config.AuthConfig
	sessions sync.Map // map[sessionToken]expiresAt
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(cfg config.AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
	}
}

// APIKeyAuth returns middleware that validates API key for REST endpoints
func (a *AuthMiddleware) APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.config.Enabled {
			c.Next()
			return
		}

		// Check Authorization header
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Extract bearer token
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format, expected: Bearer <key>"})
			c.Abort()
			return
		}

		key := strings.TrimPrefix(auth, "Bearer ")

		// Check against configured API keys
		valid := false
		for _, allowedKey := range a.config.APIKeys {
			if subtle.ConstantTimeCompare([]byte(key), []byte(allowedKey)) == 1 {
				valid = true
				break
			}
		}

		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// DashboardAuth returns middleware that validates session cookie for dashboard
func (a *AuthMiddleware) DashboardAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.config.Enabled {
			c.Next()
			return
		}

		// Check session cookie
		token, err := c.Cookie(sessionCookieName)
		if err != nil || !a.validateSession(token) {
			// Redirect to login
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

// Login handles the login form submission
func (a *AuthMiddleware) Login(c *gin.Context) {
	if !a.config.Enabled {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")

	// Validate credentials
	validUser := subtle.ConstantTimeCompare([]byte(username), []byte(a.config.Dashboard.Username)) == 1
	validPass := subtle.ConstantTimeCompare([]byte(password), []byte(a.config.Dashboard.Password)) == 1

	if !validUser || !validPass {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"Error": "Invalid username or password",
		})
		return
	}

	// Create session
	token := a.createSession()

	// Set cookie
	c.SetCookie(
		sessionCookieName,
		token,
		int(sessionDuration.Seconds()),
		"/",
		"",    // domain
		false, // secure (set true in production with HTTPS)
		true,  // httpOnly
	)

	// Redirect to dashboard
	redirect := c.Query("redirect")
	if redirect == "" {
		redirect = "/dashboard"
	}
	c.Redirect(http.StatusFound, redirect)
}

// Logout handles logout
func (a *AuthMiddleware) Logout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookieName)
	if token != "" {
		a.sessions.Delete(token)
	}

	// Clear cookie
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

// LoginPage serves the login page
func (a *AuthMiddleware) LoginPage(c *gin.Context) {
	if !a.config.Enabled {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}

	// If already logged in, redirect to dashboard
	token, err := c.Cookie(sessionCookieName)
	if err == nil && a.validateSession(token) {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}

	c.HTML(http.StatusOK, "login.html", gin.H{})
}

// createSession creates a new session token
func (a *AuthMiddleware) createSession() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	token := hex.EncodeToString(bytes)

	a.sessions.Store(token, time.Now().Add(sessionDuration))
	return token
}

// validateSession checks if a session token is valid
func (a *AuthMiddleware) validateSession(token string) bool {
	if token == "" {
		return false
	}

	expiresAt, ok := a.sessions.Load(token)
	if !ok {
		return false
	}

	if time.Now().After(expiresAt.(time.Time)) {
		a.sessions.Delete(token)
		return false
	}

	return true
}

// CleanupSessions removes expired sessions (call periodically)
func (a *AuthMiddleware) CleanupSessions() {
	now := time.Now()
	a.sessions.Range(func(key, value any) bool {
		if now.After(value.(time.Time)) {
			a.sessions.Delete(key)
		}
		return true
	})
}

// IsEnabled returns whether auth is enabled
func (a *AuthMiddleware) IsEnabled() bool {
	return a.config.Enabled
}
