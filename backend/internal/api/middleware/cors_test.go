package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
)

func TestNewCORS_DevModeWildcard(t *testing.T) {
	handler := middleware.NewCORS(nil, true)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("dev mode: expected ACAO=*, got %q", got)
	}
}

func TestNewCORS_ProductionMatchingOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	handler := middleware.NewCORS(allowed, false)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("expected ACAO=https://app.example.com, got %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary: Origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected ACAC=true, got %q", got)
	}
}

func TestNewCORS_ProductionNonMatchingOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	handler := middleware.NewCORS(allowed, false)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("non-matching origin: expected no ACAO header, got %q", got)
	}
}

func TestNewCORS_OptionsPreflight(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	handler := middleware.NewCORS(allowed, false)(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/runs", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight: expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("preflight: expected ACAO=https://app.example.com, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("preflight: expected Access-Control-Allow-Methods header")
	}
}

func TestNewCORS_MultipleAllowedOrigins(t *testing.T) {
	allowed := []string{"https://app.example.com", "https://staging.example.com"}
	handler := middleware.NewCORS(allowed, false)(okHandler())

	for _, origin := range allowed {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q: expected ACAO=%q, got %q", origin, origin, got)
		}
	}

	// An origin not in the list must not match.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://other.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin: expected no ACAO header, got %q", got)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
