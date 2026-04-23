package alerteval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/pushnotify"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const evaluationInterval = 60 * time.Second

// Evaluator polls all enabled AlertRules every 60 seconds, computes the
// current signal value via ClickHouse, and fires an AlertEvent when a
// threshold is crossed (with per-window de-duplication).
type Evaluator struct {
	ch         driver.Conn
	ruleStore  store.AlertRuleStore
	hub        *alert.Hub
	httpClient *http.Client
	loopStore  store.LoopStore
	pushSender *pushnotify.Sender // nil when VAPID not configured
}

// NewEvaluator returns a new Evaluator. pushSender may be nil to disable browser push notifications.
func NewEvaluator(ch driver.Conn, ruleStore store.AlertRuleStore, hub *alert.Hub, loopStore store.LoopStore, pushSender *pushnotify.Sender) *Evaluator {
	return &Evaluator{
		ch:         ch,
		ruleStore:  ruleStore,
		hub:        hub,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		loopStore:  loopStore,
		pushSender: pushSender,
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

	// Fire Slack notification if configured
	if rule.SlackWebhookURL != nil && *rule.SlackWebhookURL != "" {
		go e.fireSlack(rule.ID, rule.Name, *rule.SlackWebhookURL, evt)
	}

	// Fire Discord notification if configured
	if rule.DiscordWebhookURL != nil && *rule.DiscordWebhookURL != "" {
		go e.fireDiscord(rule.ID, rule.Name, *rule.DiscordWebhookURL, evt)
	}

	// Fire browser push notification if sender is configured
	if e.pushSender != nil {
		go e.pushSender.Notify(context.Background(), rule.ProjectID, rule.Name, formatAlertMsg(evt))
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
	case domain.SignalTypeAgentLoop:
		return QueryAgentLoopCount(ctx, e.loopStore, rule.ProjectID, rule.WindowSeconds)
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

// slackPayload is the JSON body sent to a Slack incoming webhook.
type slackPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type string          `json:"type"`
	Text slackBlockText  `json:"text"`
}

type slackBlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// fireSlack posts an alert notification to a Slack incoming webhook.
// The webhook URL must use the hooks.slack.com host.
func (e *Evaluator) fireSlack(ruleID, ruleName, webhookURL string, evt *domain.AlertEvent) {
	// Defence-in-depth: validate host even though the handler also validates.
	if !strings.HasPrefix(webhookURL, "https://hooks.slack.com/") {
		slog.Warn("alerteval slack skipped: unexpected host", "rule_id", ruleID, "url", webhookURL)
		return
	}

	msg := fmt.Sprintf("*%s* fired: %s is %g (threshold: %g)",
		ruleName, string(evt.SignalType), evt.CurrentValue, evt.Threshold)

	payload := slackPayload{
		Text: fmt.Sprintf("AgentPulse Alert: %s", ruleName),
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: slackBlockText{Type: "mrkdwn", Text: msg},
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := e.httpClient.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("alerteval slack failed", "rule_id", ruleID, "error", err)
		go e.updateChannelError(ruleID, fmt.Sprintf("slack: %s", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("slack non-2xx status=%d", resp.StatusCode)
		slog.Warn("alerteval slack non-2xx", "rule_id", ruleID, "status", resp.StatusCode)
		go e.updateChannelError(ruleID, errMsg)
	}
}

// discordPayload is the JSON body sent to a Discord webhook.
type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

const discordColorRed = 15548997

// fireDiscord posts an alert notification to a Discord webhook.
func (e *Evaluator) fireDiscord(ruleID, ruleName, webhookURL string, evt *domain.AlertEvent) {
	desc := fmt.Sprintf("%s: %s is %g (threshold: %g)",
		ruleName, string(evt.SignalType), evt.CurrentValue, evt.Threshold)

	payload := discordPayload{
		Embeds: []discordEmbed{
			{
				Title:       "AgentPulse Alert",
				Description: desc,
				Color:       discordColorRed,
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := e.httpClient.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("alerteval discord failed", "rule_id", ruleID, "error", err)
		go e.updateChannelError(ruleID, fmt.Sprintf("discord: %s", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("discord non-2xx status=%d", resp.StatusCode)
		slog.Warn("alerteval discord non-2xx", "rule_id", ruleID, "status", resp.StatusCode)
		go e.updateChannelError(ruleID, errMsg)
	}
}


// updateChannelError persists a channel delivery error to the rule store.
// Called in a goroutine; logs on failure rather than returning an error.
func (e *Evaluator) updateChannelError(ruleID, errMsg string) {
	if err := e.ruleStore.UpdateChannelError(context.Background(), ruleID, errMsg); err != nil {
		slog.Warn("alerteval update_channel_error", "rule_id", ruleID, "error", err)
	}
}

// formatAlertMsg returns a short human-readable description of an alert event.
func formatAlertMsg(evt *domain.AlertEvent) string {
	return fmt.Sprintf("%s is %g (threshold: %g)", string(evt.SignalType), evt.CurrentValue, evt.Threshold)
}
