package piimaskerproc

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// fieldAllowlist contains span-attribute keys that are NEVER scanned for PII.
// These are AgentPulse internal fields that hold structured, non-free-text
// data and must not be corrupted by the redactor.
var fieldAllowlist = map[string]bool{
	"agentpulse.project_id":           true,
	"agentpulse.run_id":               true,
	"agentpulse.span_kind":            true,
	"agentpulse.session_id":           true,
	"agentpulse.user_id":              true,
	"agentpulse.agent.name":           true,
	"agentpulse.model_id":             true,
	"agentpulse.cost_usd":             true,
	"agentpulse.input_tokens":         true,
	"agentpulse.output_tokens":        true,
	"agentpulse.ttft_ms":              true,
	"agentpulse.budget.halted":        true,
	"agentpulse.pii_redactions_count": true,
}

// piiMaskerProcessor scans span attributes and event attributes for PII/secrets
// and replaces matches with [REDACTED:<name>] tokens. Redaction is opt-in per
// project; settings are loaded from Postgres and refreshed on a ticker plus
// a NOTIFY-based push channel.
type piiMaskerProcessor struct {
	cfg    *Config
	store  *piiSettingsStore
	logger *zap.Logger

	// Builtin patterns compiled once at Start.
	builtins        []piiPattern
	combinedBuiltin *regexp.Regexp

	mu              sync.RWMutex
	enabledProjects map[string]*projectPIISettings
	failedLoad      bool // true while initial Postgres load has not succeeded

	done chan struct{}
}

func newPIIMaskerProcessor(logger *zap.Logger, cfg *Config, store *piiSettingsStore) *piiMaskerProcessor {
	return &piiMaskerProcessor{
		cfg:             cfg,
		store:           store,
		logger:          logger,
		enabledProjects: make(map[string]*projectPIISettings),
		done:            make(chan struct{}),
	}
}

// Start compiles builtin patterns, loads initial settings, and launches
// background goroutines for periodic refresh and NOTIFY-based push refresh.
func (p *piiMaskerProcessor) Start(ctx context.Context, _ component.Host) error {
	// Compile builtin patterns once — shared across all goroutines (read-only).
	p.builtins = builtinPatterns()
	p.combinedBuiltin = buildCombinedRegex(p.builtins)

	// Attempt initial settings load.
	if err := p.refreshSettings(ctx); err != nil {
		p.logger.Warn("piimaskerproc: initial settings load failed — entering fail-closed mode (all projects redacted with builtins)",
			zap.Error(err),
		)
		p.mu.Lock()
		p.failedLoad = true
		p.mu.Unlock()
	}

	go p.refreshLoop()
	go p.listenLoop()

	return nil
}

// Shutdown signals background goroutines to stop and releases the DB pool.
func (p *piiMaskerProcessor) Shutdown(_ context.Context) error {
	close(p.done)
	p.store.close()
	return nil
}

// ProcessTraces scans every string attribute in the batch and replaces PII
// with named [REDACTED:…] tokens. Spans without a project_id pass through
// unchanged (fail-open for unidentified traffic). Spans whose project has
// redaction disabled pass through unchanged.
func (p *piiMaskerProcessor) ProcessTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	p.mu.RLock()
	failedLoad := p.failedLoad
	p.mu.RUnlock()

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)

		projectID := extractProjectIDFromRS(rs)
		if projectID == "" {
			// No project ID — pass through (fail-open for unidentified spans).
			continue
		}

		p.mu.RLock()
		settings, enabled := p.enabledProjects[projectID]
		p.mu.RUnlock()

		if !enabled && !failedLoad {
			// Redaction not opted in for this project — skip entirely.
			continue
		}

		// Build the effective pattern set for this project.
		patterns, combined := p.effectivePatterns(failedLoad, settings)

		totalRedactions := 0

		// Scan each span's attributes and events.
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)

				// Span attributes.
				span.Attributes().Range(func(key string, val pcommon.Value) bool {
					if fieldAllowlist[key] {
						return true
					}
					if val.Type() != pcommon.ValueTypeStr {
						return true
					}
					redacted, count := redact(val.Str(), patterns, combined)
					if count > 0 {
						val.SetStr(redacted)
						totalRedactions += count
					}
					return true
				})

				// Span events.
				for e := 0; e < span.Events().Len(); e++ {
					event := span.Events().At(e)
					event.Attributes().Range(func(key string, val pcommon.Value) bool {
						if fieldAllowlist[key] {
							return true
						}
						if val.Type() != pcommon.ValueTypeStr {
							return true
						}
						redacted, count := redact(val.Str(), patterns, combined)
						if count > 0 {
							val.SetStr(redacted)
							totalRedactions += count
						}
						return true
					})
				}

				// Stamp the redaction count onto the span when non-zero.
				if totalRedactions > 0 {
					span.Attributes().PutInt("agentpulse.pii_redactions_count", int64(totalRedactions))
				}

				// Reset per-span counter for the next span.
				totalRedactions = 0
			}
		}

		// Scan resource attributes — skip agentpulse.* keys (structural metadata).
		rs.Resource().Attributes().Range(func(key string, val pcommon.Value) bool {
			if strings.HasPrefix(key, "agentpulse.") {
				return true
			}
			if val.Type() != pcommon.ValueTypeStr {
				return true
			}
			redacted, count := redact(val.Str(), patterns, combined)
			if count > 0 {
				val.SetStr(redacted)
				// Resource-level redactions are not counted in per-span stamps.
			}
			return true
		})
	}

	return td, nil
}

// effectivePatterns returns the pattern slice and combined regex to use for a
// given project. In fail-closed mode only builtins are used. When settings are
// available, custom rules are appended to the builtins.
func (p *piiMaskerProcessor) effectivePatterns(failedLoad bool, settings *projectPIISettings) ([]piiPattern, *regexp.Regexp) {
	if failedLoad || settings == nil || len(settings.customRules) == 0 {
		return p.builtins, p.combinedBuiltin
	}
	// Merge builtins + custom rules.
	merged := make([]piiPattern, 0, len(p.builtins)+len(settings.customRules))
	merged = append(merged, p.builtins...)
	merged = append(merged, settings.customRules...)
	combined := buildCombinedRegex(merged)
	return merged, combined
}

// refreshSettings loads the latest PII settings from Postgres and updates the
// in-memory cache under a write lock.
func (p *piiMaskerProcessor) refreshSettings(ctx context.Context) error {
	projects, err := p.store.loadEnabledProjects(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.enabledProjects = projects
	p.failedLoad = false
	p.mu.Unlock()
	p.logger.Debug("piimaskerproc: settings refreshed", zap.Int("enabled_projects", len(projects)))
	return nil
}

// refreshLoop runs a periodic ticker to refresh PII settings from Postgres.
func (p *piiMaskerProcessor) refreshLoop() {
	ticker := time.NewTicker(p.cfg.SettingsRefreshInterval)
	defer ticker.Stop()

	// Warn log ticker for fail-closed state.
	warnTicker := time.NewTicker(30 * time.Second)
	defer warnTicker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := p.refreshSettings(ctx); err != nil {
				p.logger.Warn("piimaskerproc: settings refresh failed", zap.Error(err))
			}
			cancel()

		case <-warnTicker.C:
			p.mu.RLock()
			failed := p.failedLoad
			p.mu.RUnlock()
			if failed {
				p.logger.Warn("piimaskerproc: still in fail-closed mode — retrying Postgres connection")
			}

		case <-p.done:
			return
		}
	}
}

// listenLoop subscribes to the pii_settings_changed Postgres NOTIFY channel
// and triggers an immediate refresh on each notification. It restarts
// automatically after a 5s backoff if the connection drops.
func (p *piiMaskerProcessor) listenLoop() {
	for {
		select {
		case <-p.done:
			return
		default:
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel the listen context when shutdown is requested.
		go func() {
			select {
			case <-p.done:
				cancel()
			case <-ctx.Done():
			}
		}()

		err := p.store.listenForChanges(ctx, func() {
			refreshCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			if err := p.refreshSettings(refreshCtx); err != nil {
				p.logger.Warn("piimaskerproc: push refresh failed", zap.Error(err))
			} else {
				p.logger.Info("piimaskerproc: settings refreshed via NOTIFY")
			}
		})
		cancel()

		if err != nil && ctx.Err() == context.Canceled {
			// Normal shutdown.
			return
		}
		if err != nil {
			p.logger.Warn("piimaskerproc: listen connection dropped, reconnecting in 5s", zap.Error(err))
		}

		// Wait before reconnecting (unless shutting down).
		select {
		case <-time.After(5 * time.Second):
		case <-p.done:
			return
		}
	}
}

// extractProjectIDFromRS extracts agentpulse.project_id from resource
// attributes, falling back to the first span's attributes (mirrors ratelimitproc).
func extractProjectIDFromRS(rs ptrace.ResourceSpans) string {
	if v, ok := rs.Resource().Attributes().Get("agentpulse.project_id"); ok {
		if s := v.Str(); s != "" {
			return s
		}
	}
	for i := 0; i < rs.ScopeSpans().Len(); i++ {
		ss := rs.ScopeSpans().At(i)
		for j := 0; j < ss.Spans().Len(); j++ {
			if v, ok := ss.Spans().At(j).Attributes().Get("agentpulse.project_id"); ok {
				if s := v.Str(); s != "" {
					return s
				}
			}
		}
	}
	return ""
}
