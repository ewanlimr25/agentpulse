package budgetenforceproc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const (
	attrCostUSD   = "agentpulse.cost_usd"
	attrRunID     = "agentpulse.run_id"
	attrProjectID = "agentpulse.project_id"
	attrAgentName = "agentpulse.agent.name"

	attrUserID    = "agentpulse.user_id"

	// attrBudgetHalted is stamped onto spans when a halt rule fires,
	// so SDK wrappers can detect mid-run budget breaches.
	attrBudgetHalted = "agentpulse.budget.halted"
)

// budgetProcessor enforces budget rules by checking accumulated span cost
// against Postgres rules and writing alerts when thresholds are breached.
// It never drops spans — it is a pure side-effect processor.
type budgetProcessor struct {
	logger  *zap.Logger
	cfg     *Config
	store   *budgetStore
	acc     *accumulator
	dedup   *alertDedup
	http    *http.Client

	mu      sync.RWMutex
	rules   []budgetRule
	stopCh  chan struct{}
}

func newBudgetProcessor(logger *zap.Logger, cfg *Config, store *budgetStore) *budgetProcessor {
	return &budgetProcessor{
		logger: logger,
		cfg:    cfg,
		store:  store,
		acc:    newAccumulator(),
		dedup:  newAlertDedup(),
		http:   &http.Client{Timeout: cfg.WebhookTimeout},
		stopCh: make(chan struct{}),
	}
}

// Start loads rules from Postgres and launches the background refresh loop.
func (p *budgetProcessor) Start(ctx context.Context, _ component.Host) error {
	if err := p.refreshRules(ctx); err != nil {
		p.logger.Warn("initial budget rule load failed", zap.Error(err))
	}
	go p.refreshLoop()
	return nil
}

// Shutdown stops the refresh loop and closes the DB pool.
func (p *budgetProcessor) Shutdown(_ context.Context) error {
	close(p.stopCh)
	p.store.close()
	return nil
}

// ProcessTraces accumulates cost per span and checks rules after each batch.
func (p *budgetProcessor) ProcessTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	// Collect (runID, projectID, agentName, userID, cost) tuples from this batch.
	type spanCost struct {
		projectID string
		runID     string
		agentName string
		userID    string
		cost      float64
	}
	var costs []spanCost

	for i := range td.ResourceSpans().Len() {
		rs := td.ResourceSpans().At(i)
		for j := range rs.ScopeSpans().Len() {
			ss := rs.ScopeSpans().At(j)
			for k := range ss.Spans().Len() {
				span := ss.Spans().At(k)
				attrs := span.Attributes()

				cost := getDouble(attrs, attrCostUSD)
				if cost <= 0 {
					continue
				}
				runID := getString(attrs, attrRunID)
				projectID := getString(attrs, attrProjectID)
				if projectID == "" {
					// Fall back to resource attributes.
					projectID = getString(rs.Resource().Attributes(), attrProjectID)
				}
				if projectID == "" || runID == "" {
					continue
				}
				agentName := getString(attrs, attrAgentName)
				userID := getString(attrs, attrUserID)
				costs = append(costs, spanCost{projectID, runID, agentName, userID, cost})
			}
		}
	}

	// Accumulate and check rules.
	haltedRuns := make(map[string]bool)
	for _, sc := range costs {
		// Run-scoped accumulation
		runTotal := p.acc.add(costKey{projectID: sc.projectID, runID: sc.runID}, sc.cost)
		// Agent-scoped accumulation
		var agentTotal float64
		if sc.agentName != "" {
			agentTotal = p.acc.add(costKey{projectID: sc.projectID, runID: sc.runID, agentName: sc.agentName}, sc.cost)
		}
		// User-scoped accumulation
		var userTotal float64
		if sc.userID != "" {
			userTotal = p.acc.add(costKey{projectID: sc.projectID, userID: sc.userID}, sc.cost)
		}

		p.mu.RLock()
		rules := p.rules
		p.mu.RUnlock()

		for _, rule := range rules {
			if rule.projectID != sc.projectID {
				continue
			}
			var accumulated float64
			switch rule.scope {
			case scopeRun:
				accumulated = runTotal
			case scopeAgent:
				if sc.agentName == "" {
					continue
				}
				accumulated = agentTotal
			case scopeUser:
				if sc.userID == "" {
					continue
				}
				accumulated = userTotal
			default:
				continue
			}

			if accumulated < rule.thresholdUSD {
				continue
			}

			var isDuplicate bool
			if rule.scope == scopeUser {
				// User-scoped rules use DB-backed dedup so alerts re-fire in subsequent windows.
				dedupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				lastFired, err := p.store.lastAlertForUser(dedupCtx, rule.id, sc.userID)
				cancel()
				if err != nil {
					p.logger.Warn("user alert dedup check failed", zap.Error(err))
					isDuplicate = true // fail safe: don't double-fire
				} else if rule.windowSeconds != nil && !lastFired.IsZero() {
					window := time.Duration(*rule.windowSeconds) * time.Second
					isDuplicate = time.Since(lastFired) < window
				}
			} else {
				isDuplicate = !p.dedup.check(rule.id, sc.runID)
			}
			if isDuplicate {
				continue
			}

			// Threshold breached — fire alert.
			if rule.scope == scopeUser {
				go p.fireUserAlert(rule, sc.projectID, sc.userID, accumulated)
			} else {
				runIDPtr := &sc.runID
				go p.fireAlert(rule, sc.projectID, runIDPtr, accumulated)
			}

			if rule.action == actionHalt && rule.scope != scopeUser {
				haltedRuns[sc.runID] = true
			}
		}
	}

	// Stamp halted runs onto all their spans so SDKs can detect the breach.
	if len(haltedRuns) > 0 {
		for i := range td.ResourceSpans().Len() {
			rs := td.ResourceSpans().At(i)
			for j := range rs.ScopeSpans().Len() {
				ss := rs.ScopeSpans().At(j)
				for k := range ss.Spans().Len() {
					span := ss.Spans().At(k)
					runID := getString(span.Attributes(), attrRunID)
					if haltedRuns[runID] {
						span.Attributes().PutBool(attrBudgetHalted, true)
					}
				}
			}
		}
	}

	return td, nil
}

// fireAlert writes the alert to Postgres and calls the webhook (if configured).
func (p *budgetProcessor) fireAlert(rule budgetRule, projectID string, runID *string, cost float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runIDStr := ""
	if runID != nil {
		runIDStr = *runID
	}

	p.logger.Info("budget threshold breached",
		zap.String("rule", rule.name),
		zap.String("project_id", projectID),
		zap.String("run_id", runIDStr),
		zap.Float64("cost_usd", cost),
		zap.Float64("threshold_usd", rule.thresholdUSD),
		zap.String("action", string(rule.action)),
	)

	if err := p.store.writeAlert(ctx, rule.id, projectID, runID, cost, rule.thresholdUSD, string(rule.action)); err != nil {
		p.logger.Error("failed to write budget alert", zap.Error(err))
	}

	if rule.webhookURL != nil && *rule.webhookURL != "" {
		p.callWebhook(*rule.webhookURL, rule, projectID, runIDStr, cost)
	}

	if rule.action == actionHalt && runID != nil {
		p.acc.resetRun(projectID, *runID)
	}
}

// fireUserAlert writes a user-scoped budget alert to Postgres and calls the webhook.
// Note: user_id is intentionally excluded from webhook payloads to avoid PII leakage.
func (p *budgetProcessor) fireUserAlert(rule budgetRule, projectID, userID string, cost float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.logger.Info("user budget threshold breached",
		zap.String("rule", rule.name),
		zap.String("project_id", projectID),
		zap.Float64("cost_usd", cost),
		zap.Float64("threshold_usd", rule.thresholdUSD),
		zap.String("action", string(rule.action)),
	)

	if err := p.store.writeUserAlert(ctx, rule.id, projectID, userID, cost, rule.thresholdUSD, string(rule.action)); err != nil {
		p.logger.Error("failed to write user budget alert", zap.Error(err))
	}

	if rule.webhookURL != nil && *rule.webhookURL != "" {
		// Webhook payload deliberately omits user_id to avoid PII leakage to external URLs.
		p.callWebhook(*rule.webhookURL, rule, projectID, "", cost)
	}

	if rule.action == actionHalt {
		p.acc.resetUser(projectID, userID)
	}
}

type webhookPayload struct {
	Type         string  `json:"type"`
	ProjectID    string  `json:"project_id"`
	RunID        string  `json:"run_id,omitempty"`
	RuleID       string  `json:"rule_id"`
	RuleName     string  `json:"rule_name"`
	CostUSD      float64 `json:"cost_usd"`
	ThresholdUSD float64 `json:"threshold_usd"`
	Action       string  `json:"action"`
	FiredAt      string  `json:"fired_at"`
}

func (p *budgetProcessor) callWebhook(url string, rule budgetRule, projectID, runID string, cost float64) {
	payload := webhookPayload{
		Type:         "budget.alert",
		ProjectID:    projectID,
		RunID:        runID,
		RuleID:       rule.id,
		RuleName:     rule.name,
		CostUSD:      cost,
		ThresholdUSD: rule.thresholdUSD,
		Action:       string(rule.action),
		FiredAt:      time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		p.logger.Error("webhook marshal failed", zap.Error(err))
		return
	}

	resp, err := p.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		p.logger.Warn("webhook call failed", zap.String("url", url), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		p.logger.Warn("webhook returned error status",
			zap.String("url", url),
			zap.Int("status", resp.StatusCode),
		)
	}
}

// refreshLoop periodically reloads rules from Postgres.
func (p *budgetProcessor) refreshLoop() {
	ticker := time.NewTicker(p.cfg.RuleRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := p.refreshRules(ctx); err != nil {
				p.logger.Warn("budget rule refresh failed", zap.Error(err))
			}
			cancel()
		case <-p.stopCh:
			return
		}
	}
}

func (p *budgetProcessor) refreshRules(ctx context.Context) error {
	rules, err := p.store.loadRules(ctx)
	if err != nil {
		return fmt.Errorf("refreshRules: %w", err)
	}
	p.mu.Lock()
	p.rules = rules
	p.mu.Unlock()
	p.logger.Debug("budget rules refreshed", zap.Int("count", len(rules)))
	return nil
}

// ── Attribute helpers ────────────────────────────────────────────────────────

func getDouble(attrs pcommon.Map, key string) float64 {
	v, ok := attrs.Get(key)
	if !ok {
		return 0
	}
	switch v.Type() {
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeInt:
		return float64(v.Int())
	default:
		return 0
	}
}

func getString(attrs pcommon.Map, key string) string {
	v, ok := attrs.Get(key)
	if !ok {
		return ""
	}
	return v.Str()
}
