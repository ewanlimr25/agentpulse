package ratelimitproc

import (
	"sync"
	"time"
)

// tokenBucket is a single per-project token bucket.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
}

// allow attempts to consume one token. It refills tokens based on elapsed
// time since the last refill, capped at burst. Returns true if a token was
// consumed, false if the bucket is empty (rate limit exceeded).
func (b *tokenBucket) allow(rate float64, burst int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.lastRefill = now

	b.tokens += elapsed * rate
	max := float64(burst)
	if b.tokens > max {
		b.tokens = max
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// bucketMap holds per-project token buckets and evicts stale ones.
type bucketMap struct {
	mu         sync.RWMutex
	buckets    map[string]*tokenBucket
	rate       float64
	burst      int
	staleAfter time.Duration
	stopCh     chan struct{}
	stoppedCh  chan struct{}
}

func newBucketMap(rate float64, burst int, staleAfter time.Duration) *bucketMap {
	return &bucketMap{
		buckets:    make(map[string]*tokenBucket),
		rate:       rate,
		burst:      burst,
		staleAfter: staleAfter,
		stopCh:     make(chan struct{}),
		stoppedCh:  make(chan struct{}),
	}
}

// allow returns true if the project has tokens available.
func (m *bucketMap) allow(projectID string) bool {
	b := m.getOrCreate(projectID)
	return b.allow(m.rate, m.burst)
}

func (m *bucketMap) getOrCreate(projectID string) *tokenBucket {
	m.mu.RLock()
	b, ok := m.buckets[projectID]
	m.mu.RUnlock()
	if ok {
		return b
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok = m.buckets[projectID]; ok {
		return b
	}
	b = &tokenBucket{
		tokens:     float64(m.burst),
		lastRefill: time.Now(),
	}
	m.buckets[projectID] = b
	return b
}

// start launches the background eviction goroutine.
func (m *bucketMap) start() {
	go m.evictLoop()
}

// stop terminates the background eviction goroutine.
func (m *bucketMap) stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	<-m.stoppedCh
}

func (m *bucketMap) evictLoop() {
	defer close(m.stoppedCh)
	ticker := time.NewTicker(m.staleAfter / 2)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.evictStale()
		}
	}
}

func (m *bucketMap) evictStale() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.buckets {
		b.mu.Lock()
		idle := now.Sub(b.lastRefill)
		b.mu.Unlock()
		if idle > m.staleAfter {
			delete(m.buckets, id)
		}
	}
}

// len returns the number of tracked buckets. Used in tests.
func (m *bucketMap) len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.buckets)
}
