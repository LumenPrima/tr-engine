package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is a trivial handler that writes 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestRequestID(t *testing.T) {
	t.Run("generates_id_when_missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		RequestID(okHandler).ServeHTTP(rec, req)
		id := rec.Header().Get("X-Request-ID")
		if len(id) != 16 {
			t.Errorf("expected 16-char hex ID, got %q (len %d)", id, len(id))
		}
	})

	t.Run("preserves_provided_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Request-ID", "my-custom-id")
		RequestID(okHandler).ServeHTTP(rec, req)
		id := rec.Header().Get("X-Request-ID")
		if id != "my-custom-id" {
			t.Errorf("expected preserved ID %q, got %q", "my-custom-id", id)
		}
	})
}

func TestCORSWithOrigins(t *testing.T) {
	t.Run("empty_origins_allows_all", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		CORSWithOrigins(nil)(okHandler).ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("missing Access-Control-Allow-Origin: *")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("allowed_origin", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		CORSWithOrigins([]string{"https://example.com"})(okHandler).ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Error("expected origin echo")
		}
		if rec.Header().Get("Vary") != "Origin" {
			t.Error("expected Vary: Origin")
		}
	})

	t.Run("disallowed_origin_no_cors_headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://evil.com")
		CORSWithOrigins([]string{"https://example.com"})(okHandler).ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("should not set CORS header for disallowed origin")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("request should still be served, got %d", rec.Code)
		}
	})

	t.Run("disallowed_origin_options_returns_403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "https://evil.com")
		CORSWithOrigins([]string{"https://example.com"})(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("options_preflight_returns_204", func(t *testing.T) {
		called := false
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("OPTIONS", "/", nil)
		CORSWithOrigins(nil)(inner).ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rec.Code)
		}
		if called {
			t.Error("inner handler should not be called on OPTIONS preflight")
		}
	})
}

func TestRateLimiter(t *testing.T) {
	t.Run("allows_normal_traffic", func(t *testing.T) {
		handler := RateLimiter(100, 100)(okHandler)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("blocks_excess_traffic", func(t *testing.T) {
		// 1 req/s, burst of 2 — third request should be blocked
		handler := RateLimiter(1, 2)(okHandler)
		for i := 0; i < 2; i++ {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "5.6.7.8:1234"
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, rec.Code)
			}
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", rec.Code)
		}
		if rec.Header().Get("Retry-After") != "1" {
			t.Error("expected Retry-After header")
		}
	})

	t.Run("different_ips_independent", func(t *testing.T) {
		handler := RateLimiter(1, 1)(okHandler)
		// Exhaust IP A
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		handler.ServeHTTP(rec, req)

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/", nil)
		req2.RemoteAddr = "10.0.0.1:1234"
		handler.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusTooManyRequests {
			t.Errorf("IP A second request: expected 429, got %d", rec2.Code)
		}

		// IP B should still work
		rec3 := httptest.NewRecorder()
		req3 := httptest.NewRequest("GET", "/", nil)
		req3.RemoteAddr = "10.0.0.2:1234"
		handler.ServeHTTP(rec3, req3)
		if rec3.Code != http.StatusOK {
			t.Errorf("IP B first request: expected 200, got %d", rec3.Code)
		}
	})
}

func TestBearerAuth(t *testing.T) {
	t.Run("empty_token_passes_all", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		BearerAuth("")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("valid_bearer_header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer secret123")
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid_bearer_header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("missing_auth", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("query_param_fallback", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?token=secret123", nil)
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid_query_param", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?token=wrong", nil)
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("non_bearer_prefix", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Basic c2VjcmV0")
		BearerAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestRequireAuth(t *testing.T) {
	t.Run("empty_token_returns_403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		RequireAuth("")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("configured_token_passes_through", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		RequireAuth("secret123")(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})
}

func TestRecoverer(t *testing.T) {
	t.Run("normal_request_passes_through", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		Recoverer(okHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("panic_produces_500_json", func(t *testing.T) {
		panicker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		Recoverer(panicker).ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %q", ct)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("response is not valid JSON: %v", err)
		}
		if body["error"] != "internal server error" {
			t.Errorf("expected error message, got %v", body)
		}
	})
}
