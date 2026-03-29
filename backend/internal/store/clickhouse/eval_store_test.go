package clickhouse

// Integration tests for EvalStore.BaselineByProject.
//
// These tests require a live ClickHouse instance and are skipped when running
// with -short (e.g. "go test -short ./...") or when the CLICKHOUSE_DSN
// environment variable is not set.
//
// To run locally:
//
//	CLICKHOUSE_DSN="clickhouse://localhost:9000" go test ./internal/store/clickhouse/... -v -run TestBaseline
//
// The test suite creates its own tables under a dedicated test database to
// avoid polluting production data.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// test fixture setup
// ---------------------------------------------------------------------------

const testDB = "agentpulse_test"

// openTestConn opens a ClickHouse connection to the test database.
// It returns nil and skips the test if CLICKHOUSE_DSN is not set.
func openTestConn(t *testing.T) driver.Conn {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping ClickHouse integration test in -short mode")
	}

	dsn := os.Getenv("CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("CLICKHOUSE_DSN not set — skipping ClickHouse integration test")
	}

	opts, err := clickhousego.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("invalid CLICKHOUSE_DSN: %v", err)
	}
	opts.Auth.Database = testDB

	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("clickhouse.Open: %v", err)
	}
	if err := conn.Ping(context.Background()); err != nil {
		t.Fatalf("clickhouse ping failed: %v", err)
	}
	return conn
}

// setupTestSchema creates the minimal tables needed by EvalStore tests.
func setupTestSchema(t *testing.T, conn driver.Conn) {
	t.Helper()
	ctx := context.Background()

	// Create test database.
	if err := conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDB)); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	// run_metrics table — used by the first query in BaselineByProject.
	if err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS run_metrics (
			project_id  String,
			run_id      String,
			min_start   DateTime64(9, 'UTC'),
			total_cost_usd Float64,
			total_tokens   UInt64,
			error_count    UInt64,
			span_count     UInt64
		) ENGINE = MergeTree()
		ORDER BY (project_id, min_start)
	`); err != nil {
		t.Fatalf("create run_metrics: %v", err)
	}

	// span_evals table — ReplacingMergeTree to match production.
	if err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS span_evals (
			project_id   String,
			run_id       String,
			span_id      String,
			eval_name    String,
			score        Float32,
			reasoning    String,
			judge_model  String,
			eval_version UInt16,
			created_at   DateTime64(9, 'UTC')
		) ENGINE = ReplacingMergeTree(created_at)
		ORDER BY (project_id, run_id, span_id, eval_name)
	`); err != nil {
		t.Fatalf("create span_evals: %v", err)
	}
}

// truncateTables removes all rows from the test tables after each test.
func truncateTables(t *testing.T, conn driver.Conn) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{"run_metrics", "span_evals"} {
		if err := conn.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE IF EXISTS %s", tbl)); err != nil {
			t.Errorf("truncate %s: %v", tbl, err)
		}
	}
}

// insertRunMetric writes a single run_metrics row for test setup.
func insertRunMetric(t *testing.T, conn driver.Conn, projectID, runID string, start time.Time) {
	t.Helper()
	err := conn.Exec(context.Background(), `
		INSERT INTO run_metrics (project_id, run_id, min_start, total_cost_usd, total_tokens, error_count, span_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, runID, start, 0.01, 1000, 0, 10,
	)
	if err != nil {
		t.Fatalf("insert run_metrics: %v", err)
	}
}

// insertSpanEval writes a single span_evals row for test setup.
func insertSpanEval(t *testing.T, conn driver.Conn, e *domain.SpanEval) {
	t.Helper()
	err := conn.Exec(context.Background(), `
		INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.RunID, e.SpanID, e.EvalName,
		e.Score, e.Reasoning, e.JudgeModel, e.EvalVersion, e.CreatedAt,
	)
	if err != nil {
		t.Fatalf("insert span_evals: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestBaselineByProject_EmptyRuns
// When the project has no runs in run_metrics, BaselineByProject must return
// an empty baseline (RunsConsidered=0, nil Types) — not an error.
// ---------------------------------------------------------------------------

func TestBaselineByProject_EmptyRuns(t *testing.T) {
	conn := openTestConn(t)
	defer conn.Close()
	setupTestSchema(t, conn)
	t.Cleanup(func() { truncateTables(t, conn) })

	store := NewEvalStore(conn)
	baseline, err := store.BaselineByProject(context.Background(), "proj-no-runs", 10)
	if err != nil {
		t.Fatalf("expected no error for project with no runs, got: %v", err)
	}
	if baseline == nil {
		t.Fatal("expected non-nil baseline even when project has no runs")
	}
	if baseline.RunsConsidered != 0 {
		t.Errorf("expected RunsConsidered=0, got %d", baseline.RunsConsidered)
	}
	if len(baseline.Types) != 0 {
		t.Errorf("expected nil/empty Types for project with no runs, got %v", baseline.Types)
	}
	if baseline.OverallScore != 0 {
		t.Errorf("expected OverallScore=0 for empty baseline, got %.3f", baseline.OverallScore)
	}
}

// ---------------------------------------------------------------------------
// TestBaselineByProject_MultipleEvalTypes
// When a project has runs with multiple eval types, BaselineByProject must
// return one EvalTypeBaseline per type and an unweighted OverallScore.
// ---------------------------------------------------------------------------

func TestBaselineByProject_MultipleEvalTypes(t *testing.T) {
	conn := openTestConn(t)
	defer conn.Close()
	setupTestSchema(t, conn)
	t.Cleanup(func() { truncateTables(t, conn) })

	projectID := "proj-multi-types"
	run1 := "run-1"
	run2 := "run-2"
	now := time.Now().UTC()

	// Insert two runs.
	insertRunMetric(t, conn, projectID, run1, now.Add(-2*time.Minute))
	insertRunMetric(t, conn, projectID, run2, now.Add(-1*time.Minute))

	// relevance: scores 0.80 and 0.60 → avg 0.70
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectID, RunID: run1, SpanID: "span-1",
		EvalName: "relevance", Score: 0.80, CreatedAt: now,
	})
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectID, RunID: run2, SpanID: "span-2",
		EvalName: "relevance", Score: 0.60, CreatedAt: now,
	})

	// hallucination: score 0.90 only in run1
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectID, RunID: run1, SpanID: "span-3",
		EvalName: "hallucination", Score: 0.90, CreatedAt: now,
	})

	store := NewEvalStore(conn)
	baseline, err := store.BaselineByProject(context.Background(), projectID, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if baseline.RunsConsidered != 2 {
		t.Errorf("expected RunsConsidered=2, got %d", baseline.RunsConsidered)
	}
	if len(baseline.Types) != 2 {
		t.Fatalf("expected 2 eval types, got %d: %+v", len(baseline.Types), baseline.Types)
	}

	// Build a name→type map for assertion independence from ordering.
	typeMap := make(map[string]domain.EvalTypeBaseline)
	for _, tp := range baseline.Types {
		typeMap[tp.EvalName] = tp
	}

	rel, ok := typeMap["relevance"]
	if !ok {
		t.Fatal("expected 'relevance' type in baseline")
	}
	// avg(0.80, 0.60) = 0.70 — allow small float tolerance.
	if absDiff(float64(rel.AvgScore), 0.70) > 0.01 {
		t.Errorf("relevance AvgScore: expected ~0.70, got %.3f", rel.AvgScore)
	}
	if rel.RunCount != 2 {
		t.Errorf("relevance RunCount: expected 2, got %d", rel.RunCount)
	}
	if rel.SpanCount != 2 {
		t.Errorf("relevance SpanCount: expected 2, got %d", rel.SpanCount)
	}

	hal, ok := typeMap["hallucination"]
	if !ok {
		t.Fatal("expected 'hallucination' type in baseline")
	}
	if absDiff(float64(hal.AvgScore), 0.90) > 0.01 {
		t.Errorf("hallucination AvgScore: expected ~0.90, got %.3f", hal.AvgScore)
	}
	if hal.RunCount != 1 {
		t.Errorf("hallucination RunCount: expected 1, got %d", hal.RunCount)
	}

	// OverallScore = unweighted avg of per-type avgs = (0.70 + 0.90) / 2 = 0.80
	if absDiff(float64(baseline.OverallScore), 0.80) > 0.01 {
		t.Errorf("OverallScore: expected ~0.80, got %.3f", baseline.OverallScore)
	}
}

// ---------------------------------------------------------------------------
// TestBaselineByProject_RespectsLastNRuns
// When lastNRuns=1, only the most recent run's evals should be included.
// ---------------------------------------------------------------------------

func TestBaselineByProject_RespectsLastNRuns(t *testing.T) {
	conn := openTestConn(t)
	defer conn.Close()
	setupTestSchema(t, conn)
	t.Cleanup(func() { truncateTables(t, conn) })

	projectID := "proj-n-runs"
	now := time.Now().UTC()

	runOld := "run-old"
	runNew := "run-new"

	insertRunMetric(t, conn, projectID, runOld, now.Add(-10*time.Minute))
	insertRunMetric(t, conn, projectID, runNew, now.Add(-1*time.Minute))

	// Old run has score 0.50; new run has score 0.90.
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectID, RunID: runOld, SpanID: "span-old",
		EvalName: "relevance", Score: 0.50, CreatedAt: now,
	})
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectID, RunID: runNew, SpanID: "span-new",
		EvalName: "relevance", Score: 0.90, CreatedAt: now,
	})

	store := NewEvalStore(conn)
	baseline, err := store.BaselineByProject(context.Background(), projectID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if baseline.RunsConsidered != 1 {
		t.Errorf("expected RunsConsidered=1, got %d", baseline.RunsConsidered)
	}
	if len(baseline.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(baseline.Types))
	}
	// Must reflect only the newest run (score ~0.90), not the old one.
	if absDiff(float64(baseline.Types[0].AvgScore), 0.90) > 0.01 {
		t.Errorf("expected score ~0.90 (newest run only), got %.3f", baseline.Types[0].AvgScore)
	}
}

// ---------------------------------------------------------------------------
// TestBaselineByProject_ProjectIsolation
// Evals from a different project must not bleed into the result.
// ---------------------------------------------------------------------------

func TestBaselineByProject_ProjectIsolation(t *testing.T) {
	conn := openTestConn(t)
	defer conn.Close()
	setupTestSchema(t, conn)
	t.Cleanup(func() { truncateTables(t, conn) })

	projectA := "proj-isolation-a"
	projectB := "proj-isolation-b"
	now := time.Now().UTC()

	insertRunMetric(t, conn, projectA, "run-a", now)
	insertRunMetric(t, conn, projectB, "run-b", now)

	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectA, RunID: "run-a", SpanID: "span-a",
		EvalName: "relevance", Score: 0.90, CreatedAt: now,
	})
	insertSpanEval(t, conn, &domain.SpanEval{
		ProjectID: projectB, RunID: "run-b", SpanID: "span-b",
		EvalName: "relevance", Score: 0.20, CreatedAt: now,
	})

	store := NewEvalStore(conn)
	baseline, err := store.BaselineByProject(context.Background(), projectA, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(baseline.Types) != 1 {
		t.Fatalf("expected 1 type for projectA, got %d", len(baseline.Types))
	}
	// projectA score should be ~0.90, not contaminated by projectB's 0.20.
	if absDiff(float64(baseline.Types[0].AvgScore), 0.90) > 0.01 {
		t.Errorf("expected score ~0.90 isolated to projectA, got %.3f", baseline.Types[0].AvgScore)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
