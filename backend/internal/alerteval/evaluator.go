package alerteval

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const evaluationInterval = 60 * time.Second

// Evaluator polls all enabled AlertRules every 60 seconds, computes the
// current signal value via ClickHouse, and fires an AlertEvent when a
// threshold is crossed (with per-window de-duplication).
type Evaluator struct {
	ch        driver.Conn
	ruleStore store.AlertRuleStore
	hub       *alert.Hub
	httpClient *http.Client
}

func NewEvaluator(ch driver.Conn, ruleStore store.AlertRuleStore, hub *alert.Hub) *Evaluator {
	return &Evaluator{
		ch:         ch,
		ruleStore:  ruleStore,
		hub:        hub,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Run starts the evaluation loop. Call in a goroutine; stops when ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	ticker := time.NewTicker(evaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

func (e *Evaluator) evaluate(ctx context.Context) {
	rules, err := e.ruleStore.ListEnabledRules(ctx)
	if err != nil {
		slog.Error("alerteval list_enabled_rules", "error", err)
		return
	}

	for _, rule := range rules {
		if err := e.evaluateRule(ctx, rule); err != nil {
			slog.Error("alerteval evaluate_rule", "rule_id", rule.ID, "error", err)
			// continue with other rules
		}
	}
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule *domain.AlertRule) error {
	current, err := e.querySignal(ctx, rule)
	if err != nil {
		return err
	}
	// -1 means insufficient data — skip this tick
	if current < 0 {
		return nil
	}

	if !thresholdCrossed(current, rule.Threshold, rule.CompareOp) {
		return nil
	}

	// De-duplication: don't re-fire within the rule's window
	last, err := e.ruleStore.LastEventForRule(ctx, rule.ID)
	if err != nil {
		return err
	}
	if last != nil && time.Since(last.TriggeredAt) < time.Duration(rule.WindowSeconds)*time.Second {
		return nil
	}

	evt := &domain.AlertEvent{
		ID:           uuid.New().String(),
		RuleID:       rule.ID,
		ProjectID:    rule.ProjectID,
		TriggeredAt:  time.Now(),
		SignalType:   rule.SignalType,
		CurrentValue: current,
		Threshold:    rule.Threshold,
		CompareOp:    rule.CompareOp,
		ActionTaken:  "notify",
		Metadata:     map[string]any{"scope_filter": rule.ScopeFilter},
	}

	if err := e.ruleStore.CreateEvent(ctx, evt); err != nil {
		return err
	}

	// Publish to WebSocket hub
	e.hub.Publish(alert.Event{
		Type:         "signal.alert",
		ProjectID:    rule.ProjectID,
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		SignalType:   string(rule.SignalType),
		CurrentValue: current,
		Threshold:    rule.Threshold,
		Action:       "notify",
	})

	// Fire webhook if configured
	if rule.WebhookURL != nil && *rule.WebhookURL != "" {
		go e.fireWebhook(*rule.WebhookURL, evt)
	}

	slog.Info("alerteval fired", "rule_id", rule.ID, "signal", rule.SignalType,
		"current", current, "threshold", rule.Threshold)
	return nil
}

func (e *Evaluator) querySignal(ctx context.Context, rule *domain.AlertRule) (float64, error) {
	switch rule.SignalType {
	case domain.SignalTypeErrorRate:
		return QueryErrorRate(ctx, e.ch, rule.ProjectID, rule.WindowSeconds)
	case domain.SignalTypeLatencyP95:
		return QueryLatencyP95(ctx, e.ch, rule.ProjectID, rule.WindowSeconds)
	case domain.SignalTypeQualityScore:
		return QueryQualityScore(ctx, e.ch, rule.ProjectID, rule.WindowSeconds)
	case domain.SignalTypeToolFailure:
		if rule.ScopeFilter == nil || *rule.ScopeFilter == "" {
			return -1, nil
		}
		return QueryToolFailureRate(ctx, e.ch, rule.ProjectID, *rule.ScopeFilter, rule.WindowSeconds)
	default:
		return -1, nil
	}
}

func thresholdCrossed(current, threshold float64, op domain.CompareOp) bool {
	switch op {
	case domain.CompareOpGt:
		return current > threshold
	case domain.CompareOpLt:
		return current < threshold
	default:
		return false
	}
}

type webhookPayload struct {
	EventID      string  `json:"event_id"`
	RuleID       string  `json:"rule_id"`
	ProjectID    string  `json:"project_id"`
	SignalType   string  `json:"signal_type"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	CompareOp    string  `json:"compare_op"`
	TriggeredAt  string  `json:"triggered_at"`
}

func (e *Evaluator) fireWebhook(url string, evt *domain.AlertEvent) {
	payload := webhookPayload{
		EventID:      evt.ID,
		RuleID:       evt.RuleID,
		ProjectID:    evt.ProjectID,
		SignalType:   string(evt.SignalType),
		CurrentValue: evt.CurrentValue,
		Threshold:    evt.Threshold,
		CompareOp:    string(evt.CompareOp),
		TriggeredAt:  evt.TriggeredAt.UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	resp, err := e.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("alerteval webhook failed", "url", url, "error", err)
		return
	}
	resp.Body.Close()
}
