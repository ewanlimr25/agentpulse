// Indie-mode bundle: SQLite metadata + DuckDB spans + local-FS payloads.
// Compiled only when the `duckdb` build tag is set; without it bootstrap
// returns ErrIndieDuckDBMissing from indie_fallback.go.
//
//go:build duckdb

package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/agentpulse/agentpulse/backend/internal/store/duckdb"
	"github.com/agentpulse/agentpulse/backend/internal/store/localfs"
	"github.com/agentpulse/agentpulse/backend/internal/store/sqlite"
	"github.com/agentpulse/agentpulse/backend/internal/store/stub"
)

// IndieStores opens SQLite + DuckDB + local-FS payload store and returns a
// StoreBundle filled with real implementations for the ported stores and stubs
// for everything else.
func IndieStores(ctx context.Context, dataDir string) (*StoreBundle, error) {
	sqliteDB, err := sqlite.Open(filepath.Join(dataDir, "agentpulse.db"))
	if err != nil {
		return nil, fmt.Errorf("indie: open sqlite: %w", err)
	}

	duckDB, err := duckdb.Open(filepath.Join(dataDir, "spans.duckdb"))
	if err != nil {
		_ = sqliteDB.Close()
		return nil, fmt.Errorf("indie: open duckdb: %w", err)
	}

	payloads, err := localfs.New(filepath.Join(dataDir, "payloads"))
	if err != nil {
		_ = sqliteDB.Close()
		_ = duckDB.Close()
		return nil, fmt.Errorf("indie: open payloads: %w", err)
	}

	b := &StoreBundle{
		SQLiteDB: sqliteDB,
		DuckDB:   duckDB,

		Projects:     sqlite.NewProjectStore(sqliteDB),
		IngestTokens: sqlite.NewIngestTokenStore(sqliteDB),

		Spans: duckdb.NewSpanStore(duckDB),
		Runs:  duckdb.NewRunStore(duckDB),

		Payloads: payloads,

		// Stubs for stores not yet ported. Each returns ErrNotImplemented or a
		// benign empty result. Replacing these with SQLite/DuckDB-backed
		// implementations is the work that completes P0-1.
		Topology:       stub.NewTopologyStore(),
		Budget:         stub.NewBudgetStore(),
		Evals:          stub.NewEvalStore(),
		EvalConfigs:    stub.NewEvalConfigStore(),
		AlertRules:     stub.NewAlertRuleStore(),
		Analytics:      stub.NewAnalyticsStore(),
		Loops:          stub.NewLoopStore(),
		Sessions:       stub.NewSessionStore(),
		Users:          stub.NewUserStore(),
		Search:         stub.NewSearchStore(),
		PIIConfigs:     stub.NewProjectPIIConfigStore(),
		SpanFeedback:   stub.NewSpanFeedbackStore(),
		Playground:     stub.NewPlaygroundStore(),
		Exports:        stub.NewExportStore(),
		RunTags:        stub.NewRunTagStore(),
		RunAnnotations: stub.NewRunAnnotationStore(),
		PushSubs:       stub.NewPushSubscriptionStore(),
		EmailDigests:   stub.NewEmailDigestStore(),
		Retention:      stub.NewRetentionStore(),
		PurgeJobs:      stub.NewPurgeJobStore(),
		EvalJobs:       stub.NewEvalJobStore(),
	}
	return b, nil
}
