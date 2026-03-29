package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
)

const (
	defaultRateLimitReqs   = 60
	defaultRateLimitWindow = time.Minute
	// A bucket is stale when it has not seen a request for 2× the window.
	defaultStaleMultiplier = 2
)

type rateBucket struct {
	mu       sync.Mutex
	count    int
	windowAt time.Time
	lastSeen time.Time
}

// RateLimiter is a per-project sliding-window rate limiter with background
// eviction of stale buckets to prevent unbounded memory growth.
type RateLimiter struct {
	mu        sync.RWMutex
	buckets   map[string]*rateBucket
	limit     int
	window    time.Duration
	staleAge  time.Duration // buckets older than this are evicted
	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// NewRateLimiter creates a RateLimiter and starts its background eviction
// goroutine. Call Stop() when the server shuts down to avoid goroutine leaks.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets:   make(map[string]*rateBucket),
		limit:     limit,
		window:    window,
		staleAge:  window * defaultStaleMultiplier,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	go rl.evictLoop()
	return rl
}

// Stop terminates the background eviction goroutine. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	select {
	case <-rl.stopCh:
		// already stopped
	default:
		close(rl.stopCh)
	}
	<-rl.stoppedCh
}

// Len returns the current number of tracked buckets. Used for testing.
func (rl *RateLimiter) Len() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.buckets)
}

// evictLoop runs a ticker every window/2 and removes buckets idle for staleAge.
func (rl *RateLimiter) evictLoop() {
	defer close(rl.stoppedCh)
	ticker := time.NewTicker(rl.window / 2)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.evictStale()
		}
	}
}

// EvictStale removes all buckets that have been idle for longer than staleAge.
// Exported for use in tests; normally called by the background eviction loop.
func (rl *RateLimiter) EvictStale() {
	rl.evictStale()
}

func (rl *RateLimiter) evictStale() {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for id, b := range rl.buckets {
		b.mu.Lock()
		idle := now.Sub(b.lastSeen)
		b.mu.Unlock()
		if idle > rl.staleAge {
			delete(rl.buckets, id)
		}
	}
}

// Middleware returns a Chi-compatible middleware that enforces the rate limit.
//
// The project ID is resolved in order:
//  1. {projectID} URL parameter (project-scoped routes)
//  2. Authenticated project from context (run-scoped routes, set by RunAuth)
//
// Requests where neither source yields a project ID are passed through unchanged.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := chi.URLParam(r, "projectID")
			if projectID == "" {
				// Fall back to the project injected by RunAuth middleware.
				if p, ok := ProjectFromContext(r.Context()); ok {
					projectID = p.ID
				}
			}
			if projectID == "" {
				next.ServeHTTP(w, r)
				return
			}

			bucket := rl.getOrCreate(projectID)

			bucket.mu.Lock()
			now := time.Now()
			if now.Sub(bucket.windowAt) >= rl.window {
				bucket.count = 0
				bucket.windowAt = now
			}
			bucket.count++
			bucket.lastSeen = now
			over := bucket.count > rl.limit
			bucket.mu.Unlock()

			if over {
				w.Header().Set("Retry-After", "60")
				httputil.Error(w, http.StatusTooManyRequests, "rate limit exceeded: 60 requests per minute per project")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) getOrCreate(projectID string) *rateBucket {
	rl.mu.RLock()
	b, ok := rl.buckets[projectID]
	rl.mu.RUnlock()
	if ok {
		return b
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if b, ok = rl.buckets[projectID]; ok {
		return b
	}
	now := time.Now()
	b = &rateBucket{windowAt: now, lastSeen: now}
	rl.buckets[projectID] = b
	return b
}

// defaultLimiter is the shared limiter used by the RateLimit middleware variable.
// Its eviction goroutine runs for the server lifetime. Tests should use
// NewRateLimiter to get isolated instances with Stop() control.
var defaultLimiter = NewRateLimiter(defaultRateLimitReqs, defaultRateLimitWindow)

// RateLimit is the default per-project rate limiting middleware (60 req/min).
// Applied to all authenticated routes in the router.
var RateLimit = defaultLimiter.Middleware()
