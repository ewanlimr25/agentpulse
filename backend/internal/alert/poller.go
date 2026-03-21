package alert

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Poller polls budget_alerts for new rows and publishes them to the Hub.
// This bridges the gap between the collector (which writes alerts to Postgres
// directly) and the WebSocket hub (which needs to push them to clients).
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
		slog.Error("alert poller query", "error", err)
		return
	}
	defer rows.Close()

	now := p.lastChecked
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
			slog.Error("alert poller scan", "error", err)
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
		now = time.Now()
	}

	if err := rows.Err(); err != nil {
		slog.Error("alert poller rows", "error", err)
		return
	}

	p.lastChecked = now
}
