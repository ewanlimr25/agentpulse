package clickhouseexporter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

// ── Mock inserter ─────────────────────────────────────────────────────────────

type mockInserter struct {
	mu       sync.Mutex
	rows     []spanRow
	callCount int
	failUntil int // fail the first N calls
	err       error
}

func (m *mockInserter) Insert(_ context.Context, rows []spanRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.callCount <= m.failUntil {
		return m.err
	}
	m.rows = append(m.rows, rows...)
	return nil
}

func (m *mockInserter) Close() error { return nil }

func (m *mockInserter) inserted() []spanRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rows
}

func (m *mockInserter) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func testConfig() *Config {
	return &Config{
		Database:      "agentpulse",
		Table:         "spans",
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
		MaxRetries:    3,
	}
}

func makeTraces(spans []map[string]any) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()
	for _, attrs := range spans {
		span := ss.Spans().AppendEmpty()
		span.SetName("test-span")
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))
		for k, v := range attrs {
			switch val := v.(type) {
			case string:
				span.Attributes().PutStr(k, val)
			case int64:
				span.Attributes().PutInt(k, val)
			case float64:
				span.Attributes().PutDouble(k, val)
			}
		}
	}
	return td
}

// ── Model mapping tests ───────────────────────────────────────────────────────

func TestSpanRowFromOTel_MapsBasicFields(t *testing.T) {
	td := makeTraces([]map[string]any{
		{
			"agentpulse.run_id":      "run-123",
			"agentpulse.span_kind":   "llm.call",
			"agentpulse.agent.name":  "ResearchAgent",
			"agentpulse.model_id":    "gpt-4o",
			"agentpulse.input_tokens":  int64(100),
			"agentpulse.output_tokens": int64(50),
			"agentpulse.cost_usd":      0.0025,
		},
	})

	rs := td.ResourceSpans().At(0)
	span := rs.ScopeSpans().At(0).Spans().At(0)
	row := spanRowFromOTel(span, rs.Resource(), "proj-abc")

	assert.Equal(t, "run-123", row.RunID)
	assert.Equal(t, "proj-abc", row.ProjectID)
	assert.Equal(t, "llm.call", row.AgentSpanKind)
	assert.Equal(t, "ResearchAgent", row.AgentName)
	assert.Equal(t, "gpt-4o", row.ModelID)
	assert.Equal(t, uint32(100), row.InputTokens)
	assert.Equal(t, uint32(50), row.OutputTokens)
	assert.InDelta(t, 0.0025, row.CostUSD, 0.0001)
	assert.Equal(t, "test-service", row.ServiceName)
}

func TestSpanRowFromOTel_ResourceProjectIDOverridesFallback(t *testing.T) {
	td := makeTraces([]map[string]any{})
	rs := td.ResourceSpans().At(0)
	rs.Resource().Attributes().PutStr("agentpulse.project_id", "proj-from-resource")
	span := rs.ScopeSpans().At(0).Spans().AppendEmpty()
	span.SetName("s")

	row := spanRowFromOTel(span, rs.Resource(), "proj-fallback")
	assert.Equal(t, "proj-from-resource", row.ProjectID)
}

func TestSpanRowFromOTel_AllSpanAttributesPreserved(t *testing.T) {
	td := makeTraces([]map[string]any{
		{"custom.key": "custom-value", "another.key": "another-value"},
	})
	rs := td.ResourceSpans().At(0)
	span := rs.ScopeSpans().At(0).Spans().At(0)
	row := spanRowFromOTel(span, rs.Resource(), "")

	assert.Equal(t, "custom-value", row.Attributes["custom.key"])
	assert.Equal(t, "another-value", row.Attributes["another.key"])
}

func TestSpanRowFromOTel_EventsSerializedAsJSON(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("s")

	event := span.Events().AppendEmpty()
	event.SetName("tool.start")
	event.Attributes().PutStr("tool.name", "web_search")

	row := spanRowFromOTel(span, rs.Resource(), "")
	assert.Contains(t, row.Events, "tool.start")
	assert.Contains(t, row.Events, "web_search")
}

func TestSpanRowFromOTel_EmptySpanNoError(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	row := spanRowFromOTel(span, rs.Resource(), "")
	assert.Equal(t, "", row.AgentSpanKind) // model reads attrs; processor sets "unknown"
	assert.Equal(t, "[]", row.Events)
}

// ── Batching tests ────────────────────────────────────────────────────────────

func TestExporter_FlushesOnBatchSizeReached(t *testing.T) {
	ins := &mockInserter{}
	cfg := testConfig()
	cfg.BatchSize = 3
	cfg.FlushInterval = 10 * time.Second // don't flush by time

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins)
	require.NoError(t, exp.Start(context.Background(), nil))
	defer exp.Shutdown(context.Background()) //nolint:errcheck

	// Send exactly BatchSize spans
	err := exp.ConsumeTraces(context.Background(), makeTraces([]map[string]any{{}, {}, {}}))
	require.NoError(t, err)

	// Allow flush goroutine to process
	assert.Eventually(t, func() bool {
		return len(ins.inserted()) == 3
	}, 2*time.Second, 50*time.Millisecond)
}

func TestExporter_FlushesOnInterval(t *testing.T) {
	ins := &mockInserter{}
	cfg := testConfig()
	cfg.BatchSize = 100           // large — won't trigger on size
	cfg.FlushInterval = 50 * time.Millisecond

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins)
	require.NoError(t, exp.Start(context.Background(), nil))
	defer exp.Shutdown(context.Background()) //nolint:errcheck

	// Send 2 spans — below batch size
	err := exp.ConsumeTraces(context.Background(), makeTraces([]map[string]any{{}, {}}))
	require.NoError(t, err)

	// Should flush after FlushInterval
	assert.Eventually(t, func() bool {
		return len(ins.inserted()) == 2
	}, 2*time.Second, 50*time.Millisecond)
}

func TestExporter_FlushesOnShutdown(t *testing.T) {
	ins := &mockInserter{}
	cfg := testConfig()
	cfg.BatchSize = 100
	cfg.FlushInterval = 10 * time.Second

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins)
	require.NoError(t, exp.Start(context.Background(), nil))

	err := exp.ConsumeTraces(context.Background(), makeTraces([]map[string]any{{}}))
	require.NoError(t, err)

	// Shutdown should flush remaining spans
	require.NoError(t, exp.Shutdown(context.Background()))
	assert.Len(t, ins.inserted(), 1)
}

// ── Retry tests ───────────────────────────────────────────────────────────────

func TestExporter_RetriesOnInsertFailure(t *testing.T) {
	ins := &mockInserter{
		failUntil: 2, // fail first 2 calls, succeed on 3rd
		err:       errors.New("connection refused"),
	}
	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.FlushInterval = 10 * time.Second
	cfg.MaxRetries = 3

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins)
	require.NoError(t, exp.Start(context.Background(), nil))
	defer exp.Shutdown(context.Background()) //nolint:errcheck

	err := exp.ConsumeTraces(context.Background(), makeTraces([]map[string]any{{}}))
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return len(ins.inserted()) == 1
	}, 5*time.Second, 100*time.Millisecond)
	assert.Equal(t, 3, ins.calls()) // 2 failures + 1 success
}

func TestExporter_DropsSpans_WhenAllRetriesExhausted(t *testing.T) {
	ins := &mockInserter{
		failUntil: 999,
		err:       errors.New("clickhouse unavailable"),
	}
	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.FlushInterval = 10 * time.Second
	cfg.MaxRetries = 2

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins)
	require.NoError(t, exp.Start(context.Background(), nil))
	defer exp.Shutdown(context.Background()) //nolint:errcheck

	err := exp.ConsumeTraces(context.Background(), makeTraces([]map[string]any{{}}))
	require.NoError(t, err) // ConsumeTraces itself should not error

	// Give time for retries to exhaust
	assert.Eventually(t, func() bool {
		return ins.calls() >= cfg.MaxRetries
	}, 5*time.Second, 100*time.Millisecond)

	assert.Empty(t, ins.inserted(), "spans should be dropped after all retries fail")
}

// ── DSN parsing tests ─────────────────────────────────────────────────────────

func TestExtractHost(t *testing.T) {
	assert.Equal(t, "localhost:9000", extractHost("clickhouse://user:pass@localhost:9000/db"))
}

func TestExtractUser(t *testing.T) {
	assert.Equal(t, "user", extractUser("clickhouse://user:pass@localhost:9000/db"))
}

func TestExtractPassword(t *testing.T) {
	assert.Equal(t, "pass", extractPassword("clickhouse://user:pass@localhost:9000/db"))
}

func TestExtractHost_InvalidDSN(t *testing.T) {
	assert.Equal(t, "localhost:9000", extractHost("not-a-dsn"))
}
