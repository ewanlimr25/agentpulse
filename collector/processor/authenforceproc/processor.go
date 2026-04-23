package authenforceproc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const (
	attrProjectID   = "agentpulse.project_id"
	attrIngestToken = "agentpulse.ingest_token"
)

// authEnforceProcessor validates agentpulse.ingest_token against the Postgres DB.
// ResourceSpans with invalid or missing tokens are dropped (fail-closed) or passed
// through (fail-open), depending on config.
type authEnforceProcessor struct {
	cfg    *Config
	store  *authStore
	logger *zap.Logger
}

func newAuthEnforceProcessor(ctx context.Context, logger *zap.Logger, cfg *Config) (*authEnforceProcessor, error) {
	if !cfg.Enabled {
		logger.Info("authenforceproc: token validation disabled — all spans will pass through")
		return &authEnforceProcessor{cfg: cfg, store: nil, logger: logger}, nil
	}

	s, err := newAuthStore(ctx, cfg.DSN)
	if err != nil {
		if cfg.FailOpen {
			logger.Warn("authenforceproc: could not connect to Postgres — running fail-open", zap.Error(err))
			return &authEnforceProcessor{cfg: cfg, store: nil, logger: logger}, nil
		}
		return nil, err
	}

	return &authEnforceProcessor{cfg: cfg, store: s, logger: logger}, nil
}

// Start is a no-op; the DB connection is established during construction.
func (p *authEnforceProcessor) Start(_ context.Context, _ component.Host) error {
	return nil
}

// Shutdown closes the Postgres pool.
func (p *authEnforceProcessor) Shutdown(_ context.Context) error {
	if p.store != nil {
		p.store.close()
	}
	return nil
}

// ProcessTraces validates the ingest token on each ResourceSpans batch.
// Invalid or missing tokens cause the ResourceSpans to be dropped and logged.
func (p *authEnforceProcessor) ProcessTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	if !p.cfg.Enabled {
		return td, nil
	}

	td.ResourceSpans().RemoveIf(func(rs ptrace.ResourceSpans) bool {
		projectID := extractAttr(rs, attrProjectID)
		rawToken := extractAttr(rs, attrIngestToken)

		// Missing project_id or token: apply fail-open/fail-closed policy.
		if projectID == "" || rawToken == "" {
			if p.cfg.FailOpen {
				return false // pass through
			}
			p.logger.Warn("authenforceproc: missing project_id or ingest_token — dropping spans (fail-closed)",
				zap.String("project_id", projectID),
				zap.Bool("has_token", rawToken != ""),
			)
			return true // drop
		}

		tokenHash := hashToken(rawToken)

		// DB unavailable: apply fail-open/fail-closed policy.
		if p.store == nil {
			if p.cfg.FailOpen {
				return false
			}
			p.logger.Warn("authenforceproc: no DB connection — dropping spans (fail-closed)",
				zap.String("project_id", projectID),
			)
			return true
		}

		record, err := p.store.getByHash(ctx, tokenHash)
		if err != nil {
			if p.cfg.FailOpen {
				p.logger.Warn("authenforceproc: DB error — passing spans through (fail-open)",
					zap.String("project_id", projectID),
					zap.Error(err),
				)
				return false
			}
			p.logger.Warn("authenforceproc: DB error — dropping spans (fail-closed)",
				zap.String("project_id", projectID),
				zap.Error(err),
			)
			return true
		}

		if record == nil {
			p.logger.Warn("authenforceproc: unknown ingest token — dropping spans",
				zap.String("project_id", projectID),
				zap.Int("span_count", countSpans(rs)),
			)
			return true // drop
		}

		if record.projectID != projectID {
			p.logger.Warn("authenforceproc: ingest token project_id mismatch — dropping spans",
				zap.String("claimed_project_id", projectID),
				zap.String("token_project_id", record.projectID),
				zap.Int("span_count", countSpans(rs)),
			)
			return true // drop
		}

		return false // token valid, keep
	})

	return td, nil
}

// extractAttr looks for an attribute in resource attributes first, then falls back
// to span attributes (same pattern as ratelimitproc/budgetenforceproc).
func extractAttr(rs ptrace.ResourceSpans, key string) string {
	if v, ok := rs.Resource().Attributes().Get(key); ok {
		if s := v.Str(); s != "" {
			return s
		}
	}
	for i := range rs.ScopeSpans().Len() {
		ss := rs.ScopeSpans().At(i)
		for j := range ss.Spans().Len() {
			if v, ok := ss.Spans().At(j).Attributes().Get(key); ok {
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

// hashToken returns the lowercase hex-encoded SHA-256 of the raw token.
// Mirrors authutil.HashToken in the backend without importing it.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
