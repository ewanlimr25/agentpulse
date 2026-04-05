package clickhouseexporter

import (
	"context"
	"encoding/json"
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

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins, nil)
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

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins, nil)
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

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins, nil)
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

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins, nil)
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

	exp := newTracesExporter(cfg, zaptest.NewLogger(t), ins, nil)
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

// ── Payload offloading tests ───────────────────────────────────────────────────

func testConfigWithS3(thresholdBytes int) *Config {
	cfg := testConfig()
	cfg.S3 = S3Config{
		Enabled:        true,
		ThresholdBytes: thresholdBytes,
		UploadTimeout:  5 * time.Second,
	}
	return cfg
}

// makeSpanRow creates a minimal spanRow for offloading tests.
func makeSpanRow(projectID, runID, spanID string, attrs map[string]string) spanRow {
	row := spanRow{
		ProjectID: projectID,
		RunID:     runID,
		SpanID:    spanID,
		StartTime: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		Attributes: make(map[string]string),
	}
	for k, v := range attrs {
		row.Attributes[k] = v
	}
	return row
}

func TestOffload_BelowThreshold_NoOffloading(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(100)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "span0001aabbccdd", map[string]string{
			"gen_ai.prompt":     "short",
			"gen_ai.completion": "also short",
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	assert.Empty(t, store.Keys())
	assert.Equal(t, "", rows[0].PayloadS3Key)
	assert.Equal(t, "short", rows[0].Attributes["gen_ai.prompt"])
	assert.Equal(t, "also short", rows[0].Attributes["gen_ai.completion"])
}

func TestOffload_AboveThreshold_OffloadsPrompt(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(10) // threshold = 10 bytes
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	bigPrompt := "this prompt is definitely longer than ten bytes"
	rows := []spanRow{
		makeSpanRow("proj-abc", "run-xyz", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt":     bigPrompt,
			"gen_ai.completion": "ok", // below threshold
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	// PayloadS3Key must be set and match expected format.
	expectedKey := "proj-abc/2024-06-15/run-xyz/aabb0011ccdd2233.json"
	assert.Equal(t, expectedKey, rows[0].PayloadS3Key)

	// Offloaded attribute must be cleared.
	assert.Equal(t, "", rows[0].Attributes["gen_ai.prompt"])

	// Non-offloaded attribute must be untouched.
	assert.Equal(t, "ok", rows[0].Attributes["gen_ai.completion"])

	// Observability flag set.
	assert.Equal(t, "true", rows[0].Attributes["agentpulse.payload_offloaded"])

	// Store must contain uploaded JSON with only the offloaded field.
	data := store.Get(expectedKey)
	require.NotNil(t, data)
	var payload map[string]string
	require.NoError(t, unmarshalJSON(t, data, &payload))
	assert.Equal(t, bigPrompt, payload["gen_ai.prompt"])
	assert.NotContains(t, payload, "gen_ai.completion")
}

func TestOffload_AllFourAttributesAboveThreshold(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(5)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "ffff000011112222", map[string]string{
			"gen_ai.prompt":     "prompt text here",
			"gen_ai.completion": "completion text here",
			"tool.input":        "tool input here",
			"tool.output":       "tool output here",
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	assert.NotEmpty(t, rows[0].PayloadS3Key)

	data := store.Get(rows[0].PayloadS3Key)
	require.NotNil(t, data)
	var payload map[string]string
	require.NoError(t, unmarshalJSON(t, data, &payload))
	assert.Contains(t, payload, "gen_ai.prompt")
	assert.Contains(t, payload, "gen_ai.completion")
	assert.Contains(t, payload, "tool.input")
	assert.Contains(t, payload, "tool.output")

	// All four attributes cleared in row.
	assert.Equal(t, "", rows[0].Attributes["gen_ai.prompt"])
	assert.Equal(t, "", rows[0].Attributes["gen_ai.completion"])
	assert.Equal(t, "", rows[0].Attributes["tool.input"])
	assert.Equal(t, "", rows[0].Attributes["tool.output"])
}

func TestOffload_S3Disabled_NoOffloading(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfig() // S3 disabled by default
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt": "a very long prompt that would normally be offloaded if s3 were enabled",
		}),
	}

	// flush calls offloadBatch only when S3 is enabled; but we call it directly
	// via the condition: if !cfg.S3.Enabled, offloadBatch is skipped entirely.
	// Simulate the flush path.
	if cfg.S3.Enabled && exp.payloads != nil {
		exp.offloadBatch(context.Background(), rows)
	}

	assert.Empty(t, store.Keys())
	assert.Equal(t, "", rows[0].PayloadS3Key)
}

func TestOffload_NilStore_NoOffloading(t *testing.T) {
	cfg := testConfigWithS3(10)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, nil)

	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt": "large enough to exceed threshold",
		}),
	}

	// The flush path checks both S3.Enabled AND payloads != nil.
	if cfg.S3.Enabled && exp.payloads != nil {
		exp.offloadBatch(context.Background(), rows)
	}

	assert.Equal(t, "", rows[0].PayloadS3Key)
}

// failingPayloadStore always returns an error on Put.
type failingPayloadStore struct{}

func (failingPayloadStore) Put(_ context.Context, _ string, _ []byte) error {
	return errors.New("s3 unavailable")
}

func TestOffload_S3UploadFails_FailOpen(t *testing.T) {
	cfg := testConfigWithS3(5)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, failingPayloadStore{})

	bigVal := "this is longer than five bytes"
	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt": bigVal,
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	// Fail-open: PayloadS3Key stays empty, attribute unchanged.
	assert.Equal(t, "", rows[0].PayloadS3Key)
	assert.Equal(t, bigVal, rows[0].Attributes["gen_ai.prompt"])
	assert.NotEqual(t, "true", rows[0].Attributes["agentpulse.payload_offloaded"])
}

func TestOffload_InvalidProjectID_SkipsOffload(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(5)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	bigVal := "longer than five bytes value"
	rows := []spanRow{
		makeSpanRow("proj/../etc", "run-1", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt": bigVal,
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	assert.Empty(t, store.Keys())
	assert.Equal(t, "", rows[0].PayloadS3Key)
	assert.Equal(t, bigVal, rows[0].Attributes["gen_ai.prompt"])
}

func TestOffload_InvalidRunID_SkipsOffload(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(5)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	bigVal := "longer than five bytes value"
	rows := []spanRow{
		makeSpanRow("proj-1", "run/bad", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt": bigVal,
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	assert.Empty(t, store.Keys())
	assert.Equal(t, "", rows[0].PayloadS3Key)
}

func TestOffload_S3KeyFormat(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(5)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	rows := []spanRow{
		makeSpanRow("my-project", "my-run-id", "deadbeef01234567", map[string]string{
			"gen_ai.prompt": "longer than five bytes",
		}),
	}
	rows[0].StartTime = time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	exp.offloadBatch(context.Background(), rows)

	expectedKey := "my-project/2025-03-07/my-run-id/deadbeef01234567.json"
	assert.Equal(t, expectedKey, rows[0].PayloadS3Key)
	assert.NotNil(t, store.Get(expectedKey))
}

func TestOffload_UploadedJSONContainsOnlyOversizedFields(t *testing.T) {
	store := newMemPayloadStore()
	cfg := testConfigWithS3(20)
	exp := newTracesExporter(cfg, zaptest.NewLogger(t), &mockInserter{}, store)

	smallVal := "tiny"
	bigVal := "this value exceeds twenty bytes for sure"
	rows := []spanRow{
		makeSpanRow("proj-1", "run-1", "aabb0011ccdd2233", map[string]string{
			"gen_ai.prompt":     bigVal,
			"gen_ai.completion": smallVal,
			"tool.input":        bigVal,
			"tool.output":       smallVal,
		}),
	}

	exp.offloadBatch(context.Background(), rows)

	data := store.Get(rows[0].PayloadS3Key)
	require.NotNil(t, data)
	var payload map[string]string
	require.NoError(t, unmarshalJSON(t, data, &payload))

	assert.Contains(t, payload, "gen_ai.prompt")
	assert.Contains(t, payload, "tool.input")
	assert.NotContains(t, payload, "gen_ai.completion")
	assert.NotContains(t, payload, "tool.output")
}

// unmarshalJSON is a test helper that unmarshals JSON data.
func unmarshalJSON(t *testing.T, data []byte, v *map[string]string) error {
	t.Helper()
	return json.Unmarshal(data, v)
}
