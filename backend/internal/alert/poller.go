package alert

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Poller polls budget_alerts and alert_events for new rows and publishes them
// to the Hub. This bridges the gap between the collector/evaluator (which write
// alerts to Postgres directly) and the WebSocket hub (which pushes to clients).
type Poller struct {
	pool        *pgxpool.Pool
	hub         *Hub
	interval    time.Duration
	lastChecked time.Time
}

// NewPoller creates a Poller that checks for alerts every interval.
func NewPoller(pool *pgxpool.Pool, hub *Hub) *Poller {
	return &Poller{
		pool:        pool,
		hub:         hub,
		interval:    5 * time.Second,
		lastChecked: time.Now(),
	}
}

// Run starts the polling loop. Call in a goroutine; stops when ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	now := p.pollBudgetAlerts(ctx)
	now2 := p.pollSignalAlerts(ctx)
	if now2.After(now) {
		now = now2
	}
	if now.After(p.lastChecked) {
		p.lastChecked = now
	}
}

func (p *Poller) pollBudgetAlerts(ctx context.Context) time.Time {
	rows, err := p.pool.Query(ctx, `
		SELECT ba.id, ba.rule_id, ba.project_id, ba.run_id,
		       ba.current_cost, ba.threshold_usd, ba.action_taken,
		       br.name
		FROM budget_alerts ba
		JOIN budget_rules br ON br.id = ba.rule_id
		WHERE ba.triggered_at > $1
		ORDER BY ba.triggered_at ASC
	`, p.lastChecked)
	if err != nil {
		slog.Error("alert poller budget query", "error", err)
		return p.lastChecked
	}
	defer rows.Close()

	last := p.lastChecked
	for rows.Next() {
		var (
			id, ruleID, projectID string
			runID                 *string
			currentCost           float64
			thresholdUSD          float64
			actionTaken           string
			ruleName              string
		)
		if err := rows.Scan(&id, &ruleID, &projectID, &runID,
			&currentCost, &thresholdUSD, &actionTaken, &ruleName); err != nil {
			slog.Error("alert poller budget scan", "error", err)
			continue
		}
		evt := Event{
			Type:      "budget.alert",
			ProjectID: projectID,
			RuleID:    ruleID,
			RuleName:  ruleName,
			CostUSD:   currentCost,
			LimitUSD:  thresholdUSD,
			Action:    actionTaken,
		}
		if runID != nil {
			evt.RunID = *runID
		}
		p.hub.Publish(evt)
		last = time.Now()
	}
	if err := rows.Err(); err != nil {
		slog.Error("alert poller budget rows", "error", err)
	}
	return last
}

func (p *Poller) pollSignalAlerts(ctx context.Context) time.Time {
	rows, err := p.pool.Query(ctx, `
		SELECT ae.rule_id, ae.project_id, ae.signal_type,
		       ae.current_value, ae.threshold, ae.compare_op, ae.action_taken,
		       ar.name
		FROM alert_events ae
		JOIN alert_rules ar ON ar.id = ae.rule_id
		WHERE ae.triggered_at > $1
		ORDER BY ae.triggered_at ASC
	`, p.lastChecked)
	if err != nil {
		slog.Error("alert poller signal query", "error", err)
		return p.lastChecked
	}
	defer rows.Close()

	last := p.lastChecked
	for rows.Next() {
		var (
			ruleID, projectID, signalType string
			currentValue, threshold       float64
			compareOp, actionTaken        string
			ruleName                      string
		)
		if err := rows.Scan(&ruleID, &projectID, &signalType,
			&currentValue, &threshold, &compareOp, &actionTaken, &ruleName); err != nil {
			slog.Error("alert poller signal scan", "error", err)
			continue
		}
		p.hub.Publish(Event{
			Type:         "signal.alert",
			ProjectID:    projectID,
			RuleID:       ruleID,
			RuleName:     ruleName,
			SignalType:   signalType,
			CurrentValue: currentValue,
			Threshold:    threshold,
			Action:       actionTaken,
		})
		last = time.Now()
	}
	if err := rows.Err(); err != nil {
		slog.Error("alert poller signal rows", "error", err)
	}
	return last
}
