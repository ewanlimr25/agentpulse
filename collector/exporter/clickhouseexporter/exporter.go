package clickhouseexporter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// payloadAttrs are the four attribute keys eligible for offloading.
var payloadAttrs = [4]string{
	"gen_ai.prompt",
	"gen_ai.completion",
	"tool.input",
	"tool.output",
}

// validKeyComponent validates that a project_id or run_id is safe for use in an S3 key.
var validKeyComponent = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

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
	payloads PayloadStore

	mu  sync.Mutex
	buf []spanRow

	flushCh chan struct{}
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func newTracesExporter(cfg *Config, logger *zap.Logger, ins inserter, store PayloadStore) *tracesExporter {
	return &tracesExporter{
		cfg:      cfg,
		logger:   logger,
		inserter: ins,
		payloads: store,
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

// probeSchema verifies the spans table has the expected columns.
// This prevents silent span data loss if migrations have not yet been applied.
func (e *tracesExporter) probeSchema(ctx context.Context) error {
	conn, err := connect(e.cfg)
	if err != nil {
		return fmt.Errorf("connecting to clickhouse: %w", err)
	}
	defer conn.Close()
	if err := conn.Exec(ctx, fmt.Sprintf("SELECT user_id, ttft_ms, payload_s3_key FROM %s.%s LIMIT 0", e.cfg.Database, e.cfg.Table)); err != nil {
		return fmt.Errorf("column missing from %s.%s — run migrations 009_user_id.sql, 013_ttft.sql, and 016_payload_ref.sql before starting the collector: %w", e.cfg.Database, e.cfg.Table, err)
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

	// Offload oversized payloads to S3 before inserting into ClickHouse.
	if e.cfg.S3.Enabled && e.payloads != nil {
		e.offloadBatch(ctx, rows)
	}

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

// offloadBatch processes a batch of rows concurrently, offloading oversized payload
// attributes to S3. Uses a bounded goroutine pool of 8 workers.
func (e *tracesExporter) offloadBatch(ctx context.Context, rows []spanRow) {
	const poolSize = 8
	sem := make(chan struct{}, poolSize)
	var wg sync.WaitGroup

	for i := range rows {
		row := &rows[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			e.offloadRow(ctx, row)
		}()
	}
	wg.Wait()
}

// offloadRow checks a single row's payload attributes and offloads any that exceed
// the threshold. On any error it logs a warning and leaves the row unchanged (fail-open).
func (e *tracesExporter) offloadRow(ctx context.Context, row *spanRow) {
	// Validate key components before doing any work.
	if !validKeyComponent.MatchString(row.ProjectID) {
		e.logger.Warn("payload offload skipped: invalid project_id",
			zap.String("project_id", row.ProjectID),
			zap.String("span_id", row.SpanID),
		)
		return
	}
	if !validKeyComponent.MatchString(row.RunID) {
		e.logger.Warn("payload offload skipped: invalid run_id",
			zap.String("run_id", row.RunID),
			zap.String("span_id", row.SpanID),
		)
		return
	}
	if row.SpanID == "" {
		e.logger.Warn("payload offload skipped: empty span_id")
		return
	}

	threshold := e.cfg.S3.ThresholdBytes

	// Collect oversized fields.
	oversized := make(map[string]string)
	for _, key := range payloadAttrs {
		val, ok := row.Attributes[key]
		if ok && len(val) > threshold {
			oversized[key] = val
		}
	}

	if len(oversized) == 0 {
		return
	}

	// Build the JSON payload.
	data, err := json.Marshal(oversized)
	if err != nil {
		e.logger.Warn("payload offload skipped: could not marshal payload",
			zap.String("span_id", row.SpanID),
			zap.Error(err),
		)
		return
	}

	// Construct S3 key: {project_id}/{YYYY-MM-DD}/{run_id}/{span_id}.json
	date := row.StartTime.UTC().Format("2006-01-02")
	s3Key := fmt.Sprintf("%s/%s/%s/%s.json", row.ProjectID, date, row.RunID, row.SpanID)

	if err := e.payloads.Put(ctx, s3Key, data); err != nil {
		e.logger.Warn("payload offload failed, keeping attributes inline",
			zap.String("span_id", row.SpanID),
			zap.String("s3_key", s3Key),
			zap.Error(err),
		)
		return
	}

	// On success: clear the offloaded fields and set metadata.
	for key := range oversized {
		row.Attributes[key] = ""
	}
	row.Attributes["agentpulse.payload_offloaded"] = "true"
	row.PayloadS3Key = s3Key
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
		attributes, resource_attrs, events,
		payload_s3_key
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
			r.PayloadS3Key,
		); err != nil {
			return fmt.Errorf("appending row: %w", err)
		}
	}

	return batch.Send()
}

func (c *clickhouseInserter) Close() error { return nil }
