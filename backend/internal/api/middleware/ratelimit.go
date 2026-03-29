package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
)

const (
	rateLimitReqs   = 60
	rateLimitWindow = time.Minute
)

type rateBucket struct {
	mu       sync.Mutex
	count    int
	windowAt time.Time
}

var (
	bucketsMu sync.RWMutex
	buckets   = make(map[string]*rateBucket)
)

// RateLimit enforces a per-project sliding-window rate limit of 60 req/min.
// Requests without a {projectID} URL parameter are passed through unchanged.
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := chi.URLParam(r, "projectID")
		if projectID == "" {
			next.ServeHTTP(w, r)
			return
		}

		bucket := getOrCreateBucket(projectID)

		bucket.mu.Lock()
		now := time.Now()
		if now.Sub(bucket.windowAt) >= rateLimitWindow {
			bucket.count = 0
			bucket.windowAt = now
		}
		bucket.count++
		over := bucket.count > rateLimitReqs
		bucket.mu.Unlock()

		if over {
			w.Header().Set("Retry-After", "60")
			httputil.Error(w, http.StatusTooManyRequests, "rate limit exceeded: 60 requests per minute per project")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getOrCreateBucket(projectID string) *rateBucket {
	bucketsMu.RLock()
	b, ok := buckets[projectID]
	bucketsMu.RUnlock()
	if ok {
		return b
	}

	bucketsMu.Lock()
	defer bucketsMu.Unlock()
	if b, ok = buckets[projectID]; ok {
		return b
	}
	b = &rateBucket{windowAt: time.Now()}
	buckets[projectID] = b
	return b
}
