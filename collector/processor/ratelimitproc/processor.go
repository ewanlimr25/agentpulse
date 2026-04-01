package ratelimitproc

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const attrProjectID = "agentpulse.project_id"

// rateLimitProcessor enforces per-project token-bucket rate limiting on
// incoming trace batches. Excess batches are dropped (not forwarded) and
// logged. Spans with no project ID always pass through (fail-open).
type rateLimitProcessor struct {
	cfg     *Config
	buckets *bucketMap
	logger  *zap.Logger
}

func newRateLimitProcessor(logger *zap.Logger, cfg *Config) *rateLimitProcessor {
	return &rateLimitProcessor{
		cfg:     cfg,
		buckets: newBucketMap(cfg.RatePerSecond, cfg.BurstSize, cfg.StaleAfter),
		logger:  logger,
	}
}

// Start launches the background bucket-eviction goroutine.
func (p *rateLimitProcessor) Start(_ context.Context, _ component.Host) error {
	p.buckets.start()
	return nil
}

// Shutdown stops the background eviction goroutine.
func (p *rateLimitProcessor) Shutdown(_ context.Context) error {
	p.buckets.stop()
	return nil
}

// ProcessTraces enforces the rate limit per project. ResourceSpans that exceed
// the rate are removed from the batch; the rest are forwarded unchanged.
func (p *rateLimitProcessor) ProcessTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	td.ResourceSpans().RemoveIf(func(rs ptrace.ResourceSpans) bool {
		projectID := extractProjectID(rs)
		if projectID == "" {
			// No project ID — pass through (fail-open).
			return false
		}

		if p.buckets.allow(projectID) {
			return false // keep
		}

		spanCount := countSpans(rs)
		p.logger.Warn("rate limit exceeded, dropping spans",
			zap.String("project_id", projectID),
			zap.Int("span_count", spanCount),
		)
		return true // drop
	})

	return td, nil
}

// extractProjectID looks for agentpulse.project_id in resource attributes
// first, then falls back to the first span's attributes (same pattern as
// budgetenforceproc).
func extractProjectID(rs ptrace.ResourceSpans) string {
	if v, ok := rs.Resource().Attributes().Get(attrProjectID); ok {
		if s := v.Str(); s != "" {
			return s
		}
	}
	for i := range rs.ScopeSpans().Len() {
		ss := rs.ScopeSpans().At(i)
		for j := range ss.Spans().Len() {
			if v, ok := ss.Spans().At(j).Attributes().Get(attrProjectID); ok {
				if s := v.Str(); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func countSpans(rs ptrace.ResourceSpans) int {
	n := 0
	for i := range rs.ScopeSpans().Len() {
		n += rs.ScopeSpans().At(i).Spans().Len()
	}
	return n
}
