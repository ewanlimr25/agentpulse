package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// makeRLHandler builds a chi router with the given RateLimiter applied.
func makeRLHandler(rl *middleware.RateLimiter) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/projects/{projectID}/evals/baseline", func(r chi.Router) {
		r.Use(rl.Middleware())
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

// makeRunHandler builds a chi router that mimics run-scoped routes: no
// {projectID} param, but a project injected into the request context (as
// RunAuth middleware does after successful authentication).
func makeRunHandler(rl *middleware.RateLimiter, projectID string) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/runs/{runID}", func(r chi.Router) {
		// Inject the authenticated project into context, simulating RunAuth.
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				p := &domain.Project{ID: projectID}
				next.ServeHTTP(w, req.WithContext(
					middleware.WithProject(req.Context(), p),
				))
			})
		})
		r.Use(rl.Middleware())
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

func TestRateLimit_AllowsRequestsBelowLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(60, time.Minute)
	defer rl.Stop()
	h := makeRLHandler(rl)

	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-pass/evals/baseline/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimit_BlocksRequestsAboveLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(60, time.Minute)
	defer rl.Stop()
	h := makeRLHandler(rl)
	url := "/api/v1/projects/proj-exceed/evals/baseline/"

	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after limit exceeded, got %d", w.Code)
	}
}

func TestRateLimit_RetryAfterHeaderPresent(t *testing.T) {
	rl := middleware.NewRateLimiter(60, time.Minute)
	defer rl.Stop()
	h := makeRLHandler(rl)
	url := "/api/v1/projects/proj-retry/evals/baseline/"

	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
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
	rl := middleware.NewRateLimiter(60, time.Minute)
	defer rl.Stop()

	r := chi.NewRouter()
	r.With(rl.Middleware()).Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
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
	rl := middleware.NewRateLimiter(60, time.Minute)
	defer rl.Stop()
	h := makeRLHandler(rl)

	// Exhaust project A.
	for i := 0; i < 61; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-iso-a/evals/baseline/", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Project B must still be allowed.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-iso-b/evals/baseline/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected project B to be unaffected by project A's limit, got %d", w.Code)
	}
}

// TestRateLimit_EvictsStaleBuckets verifies that buckets not seen for
// longer than the stale threshold are removed by the eviction loop.
func TestRateLimit_EvictsStaleBuckets(t *testing.T) {
	window := 50 * time.Millisecond
	rl := middleware.NewRateLimiter(60, window)
	defer rl.Stop()
	h := makeRLHandler(rl)

	// Create a bucket.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-evict/evals/baseline/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if rl.Len() != 1 {
		t.Fatalf("expected 1 bucket after request, got %d", rl.Len())
	}

	// Wait for the bucket to become stale (staleAge = 2×window) and the
	// eviction ticker to fire (fires at window/2 interval).
	time.Sleep(window*3 + 10*time.Millisecond)

	if rl.Len() != 0 {
		t.Errorf("expected stale bucket to be evicted, got %d buckets", rl.Len())
	}
}

// TestRateLimit_RetainsActiveBuckets verifies that buckets with recent
// activity are not evicted.
func TestRateLimit_RetainsActiveBuckets(t *testing.T) {
	window := 200 * time.Millisecond
	rl := middleware.NewRateLimiter(60, window)
	defer rl.Stop()
	h := makeRLHandler(rl)

	// Send a request to create the bucket.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-active/evals/baseline/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	// Wait for one eviction tick but NOT long enough for the bucket to be stale.
	time.Sleep(window/2 + 10*time.Millisecond)

	if rl.Len() != 1 {
		t.Errorf("expected active bucket to be retained, got %d buckets", rl.Len())
	}
}

// TestRateLimit_StopPreventsGoroutineLeak verifies that Stop() terminates the
// eviction goroutine.
func TestRateLimit_StopPreventsGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	rl := middleware.NewRateLimiter(60, 50*time.Millisecond)

	// Give the goroutine time to start.
	time.Sleep(5 * time.Millisecond)
	during := runtime.NumGoroutine()
	if during <= before {
		t.Logf("warning: could not confirm goroutine started (before=%d, during=%d)", before, during)
	}

	rl.Stop()
	// Give the goroutine time to exit.
	time.Sleep(5 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > before+1 { // +1 tolerance for unrelated goroutines
		t.Errorf("goroutine leak suspected: before=%d, after Stop=%d", before, after)
	}
}

// TestRateLimit_ManyProjectsEvicted is a regression test for the original
// unbounded map bug: many stale buckets must all be evictable.
// Uses EvictStale() directly to avoid timing-dependent failures under parallel load.
func TestRateLimit_ManyProjectsEvicted(t *testing.T) {
	// Use a tiny window so all buckets are immediately stale after creation.
	window := time.Millisecond
	rl := middleware.NewRateLimiter(60, window)
	defer rl.Stop()
	h := makeRLHandler(rl)

	const n = 10_000
	for i := 0; i < n; i++ {
		url := "/api/v1/projects/proj-many-" + itoa(i) + "/evals/baseline/"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Wait for buckets to become stale (staleAge = 2×window = 2ms).
	time.Sleep(5 * time.Millisecond)

	// Trigger eviction directly — deterministic, no reliance on ticker timing.
	rl.EvictStale()

	if got := rl.Len(); got != 0 {
		t.Errorf("expected 0 buckets after eviction, got %d", got)
	}
}

// TestRateLimit_RunScopedRoutesRateLimited verifies that run-scoped routes
// (no {projectID} URL param) are rate-limited using the project ID injected
// into context by the RunAuth middleware.
func TestRateLimit_RunScopedRoutesRateLimited(t *testing.T) {
	rl := middleware.NewRateLimiter(5, time.Minute) // low limit to keep test fast
	defer rl.Stop()
	h := makeRunHandler(rl, "proj-run-scope")

	// First 5 should pass.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-abc/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 6th should be rate-limited.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-abc/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for run-scoped route over limit, got %d", w.Code)
	}
}

// TestRateLimit_RunScopedNoContextPassThrough verifies that run-scoped routes
// with no project in context (unauthenticated) still pass through unchanged.
func TestRateLimit_RunScopedNoContextPassThrough(t *testing.T) {
	rl := middleware.NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	r := chi.NewRouter()
	r.Route("/api/v1/runs/{runID}", func(r chi.Router) {
		r.Use(rl.Middleware())
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-xyz/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected passthrough 200 with no project context, got %d", i+1, w.Code)
		}
	}
}

// TestRateLimit_StopIdempotent verifies that calling Stop() multiple times does not panic.
func TestRateLimit_StopIdempotent(t *testing.T) {
	rl := middleware.NewRateLimiter(60, time.Minute)
	rl.Stop()
	rl.Stop() // must not panic
}

// TestRateLimit_ConcurrentEvictionAndRequests checks for data races under
// concurrent load. Run with -race.
func TestRateLimit_ConcurrentEvictionAndRequests(t *testing.T) {
	window := 20 * time.Millisecond
	rl := middleware.NewRateLimiter(1000, window)
	defer rl.Stop()
	h := makeRLHandler(rl)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	for g := 0; g < 20; g++ {
		gID := g
		go func() {
			for {
				select {
				case <-ctx.Done():
					done <- struct{}{}
					return
				default:
					url := "/api/v1/projects/proj-race-" + itoa(gID) + "/evals/baseline/"
					req := httptest.NewRequest(http.MethodGet, url, nil)
					h.ServeHTTP(httptest.NewRecorder(), req)
				}
			}
		}()
	}
	for g := 0; g < 20; g++ {
		<-done
	}
}

// itoa is a minimal int-to-string helper to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
