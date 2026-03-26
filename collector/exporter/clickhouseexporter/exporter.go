package clickhouseexporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// inserter abstracts the ClickHouse write path for testability.
type inserter interface {
	Insert(ctx context.Context, rows []spanRow) error
	Close() error
}

// tracesExporter buffers spans and flushes them to ClickHouse in batches.
type tracesExporter struct {
	cfg      *Config
	logger   *zap.Logger
	inserter inserter

	mu     sync.Mutex
	buf    []spanRow

	flushCh chan struct{}
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func newTracesExporter(cfg *Config, logger *zap.Logger, ins inserter) *tracesExporter {
	return &tracesExporter{
		cfg:      cfg,
		logger:   logger,
		inserter: ins,
		buf:      make([]spanRow, 0, cfg.BatchSize),
		flushCh:  make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (e *tracesExporter) Start(ctx context.Context, _ component.Host) error {
	// Only probe the real ClickHouse inserter; skip when a test mock is injected.
	if _, isReal := e.inserter.(*clickhouseInserter); isReal {
		if err := e.probeSchema(ctx); err != nil {
			return fmt.Errorf("clickhouse schema probe failed: %w", err)
		}
	}
	go e.flushLoop()
	return nil
}

// probeSchema verifies the spans table has the user_id and ttft_ms columns.
// This prevents silent span data loss if migrations have not yet been applied.
func (e *tracesExporter) probeSchema(ctx context.Context) error {
	conn, err := connect(e.cfg)
	if err != nil {
		return fmt.Errorf("connecting to clickhouse: %w", err)
	}
	defer conn.Close()
	if err := conn.Exec(ctx, fmt.Sprintf("SELECT user_id, ttft_ms FROM %s.%s LIMIT 0", e.cfg.Database, e.cfg.Table)); err != nil {
		return fmt.Errorf("user_id or ttft_ms column missing from %s.%s — run migrations 009_user_id.sql and 013_ttft.sql before starting the collector: %w", e.cfg.Database, e.cfg.Table, err)
	}
	return nil
}

func (e *tracesExporter) Shutdown(ctx context.Context) error {
	close(e.stopCh)
	select {
	case <-e.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return e.inserter.Close()
}

// ConsumeTraces converts incoming spans to rows and buffers them.
// Signals the flush goroutine when the batch size is reached.
func (e *tracesExporter) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	e.mu.Lock()
	for i := range td.ResourceSpans().Len() {
		rs := td.ResourceSpans().At(i)
		// project_id may be a resource attribute or a span attribute (set by agentsemanticproc)
		resourceProjectID := strAttr(rs.Resource().Attributes(), "agentpulse.project_id")
		for j := range rs.ScopeSpans().Len() {
			ss := rs.ScopeSpans().At(j)
			for k := range ss.Spans().Len() {
				span := ss.Spans().At(k)
				projectID := resourceProjectID
				if pid := strAttr(span.Attributes(), "agentpulse.project_id"); pid != "" {
					projectID = pid
				}
				e.buf = append(e.buf, spanRowFromOTel(span, rs.Resource(), projectID))
			}
		}
	}
	shouldFlush := len(e.buf) >= e.cfg.BatchSize
	e.mu.Unlock()

	if shouldFlush {
		select {
		case e.flushCh <- struct{}{}:
		default:
		}
	}
	return nil
}

// flushLoop periodically flushes the buffer or flushes when signalled.
func (e *tracesExporter) flushLoop() {
	defer close(e.doneCh)
	ticker := time.NewTicker(e.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			e.flush(context.Background())
			return
		case <-ticker.C:
			e.flush(context.Background())
		case <-e.flushCh:
			e.flush(context.Background())
		}
	}
}

// flush drains the buffer and inserts all pending rows with retry.
func (e *tracesExporter) flush(ctx context.Context) {
	e.mu.Lock()
	if len(e.buf) == 0 {
		e.mu.Unlock()
		return
	}
	rows := e.buf
	e.buf = make([]spanRow, 0, e.cfg.BatchSize)
	e.mu.Unlock()

	var lastErr error
	for attempt := range e.cfg.MaxRetries {
		if err := e.inserter.Insert(ctx, rows); err != nil {
			lastErr = err
			wait := time.Duration(attempt+1) * 500 * time.Millisecond
			e.logger.Warn("clickhouse insert failed, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("rows", len(rows)),
				zap.Duration("wait", wait),
				zap.Error(err),
			)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
			continue
		}
		e.logger.Debug("flushed spans to clickhouse", zap.Int("rows", len(rows)))
		return
	}

	e.logger.Error("clickhouse insert failed after all retries, spans dropped",
		zap.Int("dropped", len(rows)),
		zap.Error(lastErr),
	)
}

// clickhouseInserter is the production inserter backed by the ClickHouse driver.
type clickhouseInserter struct {
	cfg *Config
}

func newClickhouseInserter(cfg *Config) (*clickhouseInserter, error) {
	return &clickhouseInserter{cfg: cfg}, nil
}

func (c *clickhouseInserter) Insert(ctx context.Context, rows []spanRow) error {
	conn, err := connect(c.cfg)
	if err != nil {
		return fmt.Errorf("connecting to clickhouse: %w", err)
	}
	defer conn.Close()

	batch, err := conn.PrepareBatch(ctx, fmt.Sprintf(`INSERT INTO %s.%s (
		trace_id, span_id, parent_span_id,
		run_id, project_id, session_id, user_id,
		agent_span_kind, agent_name, model_id,
		span_name, service_name, status_code, status_message,
		start_time, end_time,
		input_tokens, output_tokens, cost_usd, ttft_ms,
		attributes, resource_attrs, events
	) VALUES`, c.cfg.Database, c.cfg.Table))
	if err != nil {
		return fmt.Errorf("preparing batch: %w", err)
	}

	for _, r := range rows {
		if err := batch.Append(
			r.TraceID, r.SpanID, r.ParentSpanID,
			r.RunID, r.ProjectID, r.SessionID, r.UserID,
			r.AgentSpanKind, r.AgentName, r.ModelID,
			r.SpanName, r.ServiceName, r.StatusCode, r.StatusMessage,
			r.StartTime, r.EndTime,
			r.InputTokens, r.OutputTokens, r.CostUSD, r.TtftMs,
			r.Attributes, r.ResourceAttrs, r.Events,
		); err != nil {
			return fmt.Errorf("appending row: %w", err)
		}
	}

	return batch.Send()
}

func (c *clickhouseInserter) Close() error { return nil }
