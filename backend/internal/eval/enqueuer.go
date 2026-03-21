package eval

import (
	"context"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const enqueueInterval = 30 * time.Second

// Enqueuer polls ClickHouse for unprocessed llm.call spans and
// inserts eval_jobs for them. ON CONFLICT DO NOTHING handles duplicates.
type Enqueuer struct {
	chConn   driver.Conn
	jobStore store.EvalJobStore
}

func NewEnqueuer(chConn driver.Conn, jobStore store.EvalJobStore) *Enqueuer {
	return &Enqueuer{chConn: chConn, jobStore: jobStore}
}

func (e *Enqueuer) Run(ctx context.Context) {
	e.enqueue(ctx)
	t := time.NewTicker(enqueueInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.enqueue(ctx)
		}
	}
}

func (e *Enqueuer) enqueue(ctx context.Context) {
	// Query spans from the last 24 hours with prompt data available
	rows, err := e.chConn.Query(ctx, `
		SELECT span_id, run_id, project_id
		FROM spans
		WHERE agent_span_kind = 'llm.call'
		  AND start_time >= now() - INTERVAL 24 HOUR
		  AND attributes['gen_ai.prompt'] != ''
		ORDER BY start_time DESC
		LIMIT 500
	`)
	if err != nil {
		slog.Error("eval enqueuer: query spans", "error", err)
		return
	}
	defer rows.Close()

	var jobs []*domain.EvalJob
	for rows.Next() {
		j := &domain.EvalJob{EvalName: "relevance"}
		if err := rows.Scan(&j.SpanID, &j.RunID, &j.ProjectID); err != nil {
			slog.Error("eval enqueuer: scan", "error", err)
			continue
		}
		jobs = append(jobs, j)
	}

	if len(jobs) == 0 {
		return
	}

	if err := e.jobStore.Enqueue(ctx, jobs); err != nil {
		slog.Error("eval enqueuer: enqueue", "error", err)
		return
	}

	slog.Info("eval enqueuer: enqueued jobs", "count", len(jobs))
}
