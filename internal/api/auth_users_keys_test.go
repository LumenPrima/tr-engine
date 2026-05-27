package api

// Tests for auth/users/keys handler behavior NOT covered elsewhere:
//   - AuthHandler.Me with no user in context
//   - generateAccessToken / generateRefreshToken claim correctness

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/database"
	"golang.org/x/crypto/bcrypt"
)

// ── AuthHandler.Me ────────────────────────────────────────────────────────────

func TestAuthHandlerMe_NotAuthenticated(t *testing.T) {
	h := NewAuthHandler(nil, []byte("secret"), zerolog.Nop())
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	h.Me(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated me: want 401, got %d", rec.Code)
	}
}

// ── Token generation ──────────────────────────────────────────────────────────

func TestGenerateAccessToken_ClaimsAreCorrect(t *testing.T) {
	secret := []byte("test-secret")
	h := NewAuthHandler(nil, secret, zerolog.Nop())
	user := &database.User{
		ID:       99,
		Username: "alice@example.com",
		Role:     "editor",
		Enabled:  true,
	}

	signed, err := h.generateAccessToken(user)
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}

	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(signed, claims, jwtKeyFunc(secret))
	if err != nil || !tok.Valid {
		t.Fatalf("parse token: %v", err)
	}

	if claims.Subject != "99" {
		t.Errorf("subject = %q, want %q", claims.Subject, "99")
	}
	if claims.Username != "alice@example.com" {
		t.Errorf("username = %q, want alice@example.com", claims.Username)
	}
	if claims.Role != "editor" {
		t.Errorf("role = %q, want editor", claims.Role)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expiry not set")
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 || ttl > accessTokenExpiry {
		t.Errorf("expiry ttl = %v, expected in (0, %v]", ttl, accessTokenExpiry)
	}
}

func TestGenerateRefreshToken_ClaimsAreCorrect(t *testing.T) {
	secret := []byte("test-secret")
	store := newMockAuthStore(t, 42, "alice@example.com", "secret-password", "editor")
	h := NewAuthHandler(store, secret, zerolog.Nop())

	signed, err := h.generateRefreshToken(context.Background(), store.user.ID)
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}

	claims := &RefreshClaims{}
	tok, err := jwt.ParseWithClaims(signed, claims, jwtKeyFunc(secret))
	if err != nil || !tok.Valid {
		t.Fatalf("parse token: %v", err)
	}

	if claims.Subject != strconv.Itoa(store.user.ID) {
		t.Errorf("subject = %q, want %q", claims.Subject, strconv.Itoa(store.user.ID))
	}
	if claims.Type != "refresh" {
		t.Errorf("type = %q, want refresh", claims.Type)
	}
	if claims.ID == "" {
		t.Fatal("jti not set")
	}
	storedJTI, err := store.GetRefreshTokenJTI(context.Background(), store.user.ID)
	if err != nil {
		t.Fatalf("GetRefreshTokenJTI: %v", err)
	}
	if storedJTI != claims.ID {
		t.Errorf("stored jti = %q, want %q", storedJTI, claims.ID)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expiry not set")
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 || ttl > refreshTokenExpiry {
		t.Errorf("expiry ttl = %v, expected in (0, %v]", ttl, refreshTokenExpiry)
	}
}

func TestLoginSetsRefreshTokenJTI(t *testing.T) {
	store := newMockAuthStore(t, 7, "login@example.com", "secret-password", "admin")
	h := NewAuthHandler(store, []byte("test-secret"), zerolog.Nop())

	body := bytes.NewBufferString(`{"username":"` + store.user.Username + `","password":"secret-password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	refreshCookie := findCookie(t, rec.Result().Cookies(), refreshCookieName)
	claims := parseRefreshClaims(t, refreshCookie.Value, []byte("test-secret"))
	if claims.ID == "" {
		t.Fatal("login refresh token missing jti")
	}
	storedJTI, err := store.GetRefreshTokenJTI(context.Background(), store.user.ID)
	if err != nil {
		t.Fatalf("GetRefreshTokenJTI: %v", err)
	}
	if storedJTI != claims.ID {
		t.Fatalf("stored jti = %q, want %q", storedJTI, claims.ID)
	}
}

func TestRefreshRotatesRefreshTokenAndRejectsReuse(t *testing.T) {
	store := newMockAuthStore(t, 8, "refresh@example.com", "secret-password", "admin")
	secret := []byte("test-secret")
	h := NewAuthHandler(store, secret, zerolog.Nop())

	initialCookie := loginAndGetRefreshCookie(t, h, store.user.Username, "secret-password")
	initialClaims := parseRefreshClaims(t, initialCookie.Value, secret)

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshReq.AddCookie(initialCookie)
	refreshRec := httptest.NewRecorder()
	h.Refresh(refreshRec, refreshReq)

	if refreshRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", refreshRec.Code, http.StatusOK, refreshRec.Body.String())
	}
	rotatedCookie := findCookie(t, refreshRec.Result().Cookies(), refreshCookieName)
	rotatedClaims := parseRefreshClaims(t, rotatedCookie.Value, secret)
	if rotatedClaims.ID == initialClaims.ID {
		t.Fatalf("refresh token jti did not rotate: %q", rotatedClaims.ID)
	}
	storedJTI, err := store.GetRefreshTokenJTI(context.Background(), store.user.ID)
	if err != nil {
		t.Fatalf("GetRefreshTokenJTI: %v", err)
	}
	if storedJTI != rotatedClaims.ID {
		t.Fatalf("stored jti = %q, want rotated %q", storedJTI, rotatedClaims.ID)
	}

	reuseReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	reuseReq.AddCookie(initialCookie)
	reuseRec := httptest.NewRecorder()
	h.Refresh(reuseRec, reuseReq)

	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status = %d, want %d: %s", reuseRec.Code, http.StatusUnauthorized, reuseRec.Body.String())
	}
}

func TestLogoutInvalidatesRefreshTokenJTI(t *testing.T) {
	store := newMockAuthStore(t, 9, "logout@example.com", "secret-password", "admin")
	secret := []byte("test-secret")
	h := NewAuthHandler(store, secret, zerolog.Nop())

	refreshCookie := loginAndGetRefreshCookie(t, h, store.user.Username, "secret-password")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	logoutRec := httptest.NewRecorder()
	h.Logout(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", logoutRec.Code, http.StatusOK, logoutRec.Body.String())
	}
	storedJTI, err := store.GetRefreshTokenJTI(context.Background(), store.user.ID)
	if err != nil {
		t.Fatalf("GetRefreshTokenJTI: %v", err)
	}
	if storedJTI != "" {
		t.Fatalf("stored jti after logout = %q, want empty", storedJTI)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	h.Refresh(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want %d: %s", refreshRec.Code, http.StatusUnauthorized, refreshRec.Body.String())
	}
}

func loginAndGetRefreshCookie(t *testing.T, h *AuthHandler, username, password string) *http.Cookie {
	t.Helper()

	body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return findCookie(t, rec.Result().Cookies(), refreshCookieName)
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q not found", name)
	return nil
}

func parseRefreshClaims(t *testing.T, token string, secret []byte) *RefreshClaims {
	t.Helper()
	claims := &RefreshClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, jwtKeyFunc(secret))
	if err != nil || !parsed.Valid {
		t.Fatalf("parse refresh token: %v", err)
	}
	return claims
}

type mockAuthStore struct {
	user            *database.User
	refreshTokenJTI string
}

func newMockAuthStore(t *testing.T, userID int, username, password, role string) *mockAuthStore {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	now := time.Now()
	return &mockAuthStore{
		user: &database.User{
			ID:           userID,
			Username:     database.NormalizeUsername(username),
			PasswordHash: string(hash),
			Role:         role,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}

func (m *mockAuthStore) GetUserByUsername(_ context.Context, username string) (*database.User, error) {
	if m.user != nil && m.user.Username == database.NormalizeUsername(username) {
		return m.user, nil
	}
	return nil, nil
}

func (m *mockAuthStore) UpdateLastLogin(_ context.Context, id int) error {
	if m.user != nil && m.user.ID == id {
		now := time.Now()
		m.user.LastLogin = &now
	}
	return nil
}

func (m *mockAuthStore) SetRefreshTokenJTI(_ context.Context, userID int, jti string) error {
	if m.user != nil && m.user.ID == userID {
		m.refreshTokenJTI = jti
	}
	return nil
}

func (m *mockAuthStore) GetRefreshTokenJTI(_ context.Context, userID int) (string, error) {
	if m.user != nil && m.user.ID == userID {
		return m.refreshTokenJTI, nil
	}
	return "", nil
}

func (m *mockAuthStore) GetUserByID(_ context.Context, id int) (*database.User, error) {
	if m.user != nil && m.user.ID == id {
		return m.user, nil
	}
	return nil, nil
}
