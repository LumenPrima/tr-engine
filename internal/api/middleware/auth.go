package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
)

const (
	sessionCookieName = "tr_session"
	sessionDuration   = 24 * time.Hour
	apiKeyPrefix      = "tr_api_"
)

// AuthMiddleware handles authentication for the API server
type AuthMiddleware struct {
	config   config.AuthConfig
	db       *database.DB
	sessions sync.Map // map[sessionToken]expiresAt
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(cfg config.AuthConfig, db *database.DB) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
		db:     db,
	}
}

// GenerateAPIKey creates a new random API key
// Returns the plaintext key (only shown once) and the prefix for identification
func GenerateAPIKey() (plaintext, prefix string) {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	randomPart := hex.EncodeToString(bytes)
	plaintext = apiKeyPrefix + randomPart
	prefix = plaintext[:12] // "tr_api_" + first 5 chars of random
	return
}

// HashAPIKey creates a SHA-256 hash of the API key for storage
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
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

		// Check against configured API keys (root keys)
		for _, allowedKey := range a.config.APIKeys {
			if subtle.ConstantTimeCompare([]byte(key), []byte(allowedKey)) == 1 {
				c.Set("api_key_source", "config")
				c.Next()
				return
			}
		}

		// Check database keys
		if a.db != nil {
			keyHash := HashAPIKey(key)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			apiKey, err := a.db.GetAPIKeyByHash(ctx, keyHash)
			if err == nil && apiKey != nil {
				// Check if key is valid
				if apiKey.RevokedAt != nil {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "API key has been revoked"})
					c.Abort()
					return
				}
				if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "API key has expired"})
					c.Abort()
					return
				}

				// Update last used (async, don't block request)
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					a.db.UpdateAPIKeyLastUsed(ctx, apiKey.ID)
				}()

				c.Set("api_key_source", "database")
				c.Set("api_key_id", apiKey.ID)
				c.Set("api_key_name", apiKey.Name)
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		c.Abort()
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

// CreateAPIKey creates a new API key in the database
func (a *AuthMiddleware) CreateAPIKey(ctx context.Context, name string, scopes []string, readOnly bool, expiresAt *time.Time) (plaintext string, key *models.APIKey, err error) {
	plaintext, prefix := GenerateAPIKey()
	keyHash := HashAPIKey(plaintext)

	key, err = a.db.CreateAPIKey(ctx, keyHash, prefix, name, scopes, readOnly, expiresAt)
	if err != nil {
		return "", nil, err
	}

	return plaintext, key, nil
}

// ListAPIKeys returns all API keys
func (a *AuthMiddleware) ListAPIKeys(ctx context.Context) ([]*models.APIKey, error) {
	return a.db.ListAPIKeys(ctx)
}

// RevokeAPIKey revokes an API key
func (a *AuthMiddleware) RevokeAPIKey(ctx context.Context, id int) error {
	return a.db.RevokeAPIKey(ctx, id)
}

// DeleteAPIKey deletes an API key
func (a *AuthMiddleware) DeleteAPIKey(ctx context.Context, id int) error {
	return a.db.DeleteAPIKey(ctx, id)
}
