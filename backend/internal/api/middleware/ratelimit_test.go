package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
)

// makeRateLimitedHandler wraps a simple 200-OK handler inside a Chi router
// with the RateLimit middleware applied to a route that exposes {projectID}.
func makeRateLimitedHandler() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/projects/{projectID}/evals/baseline", func(r chi.Router) {
		r.Use(middleware.RateLimit)
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

func TestRateLimit_AllowsRequestsBelowLimit(t *testing.T) {
	h := makeRateLimitedHandler()

	// Send 60 requests — all must pass.
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-rl-pass/evals/baseline/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimit_BlocksRequestsAboveLimit(t *testing.T) {
	// Use a unique project ID so this test doesn't share bucket state with others.
	h := makeRateLimitedHandler()
	projectID := "proj-rl-exceed"
	url := "/api/v1/projects/" + projectID + "/evals/baseline/"

	// Exhaust the limit (60 requests).
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}

	// The 61st request must be rejected with 429.
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after limit exceeded, got %d", w.Code)
	}
}

func TestRateLimit_RetryAfterHeaderPresent(t *testing.T) {
	h := makeRateLimitedHandler()
	projectID := "proj-rl-retry-after"
	url := "/api/v1/projects/" + projectID + "/evals/baseline/"

	// Exhaust the limit.
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("expected Retry-After: 60, got %q", got)
	}
}

func TestRateLimit_PassThroughWhenNoProjectID(t *testing.T) {
	// Routes without {projectID} must be passed through unchanged even when
	// the rate limiter is applied — it should be a no-op for such routes.
	r := chi.NewRouter()
	r.With(middleware.RateLimit).Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 for no-projectID route, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimit_IsolatedPerProject(t *testing.T) {
	// Exhausting the limit for one project must not affect another project.
	h := makeRateLimitedHandler()

	// Exhaust project A.
	for i := 0; i < 61; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-rl-iso-a/evals/baseline/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}

	// Project B must still be allowed.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-rl-iso-b/evals/baseline/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected project B to be unaffected by project A's rate limit, got %d", w.Code)
	}
}
