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
)

// IndieStores opens SQLite + DuckDB + local-FS payload store and returns a
// fully-wired StoreBundle. Each store is the indie-mode (SQLite or DuckDB)
// implementation of its interface — no stubs remain after P0-1 follow-up.
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

		// SQLite metadata stores
		Projects:       sqlite.NewProjectStore(sqliteDB),
		IngestTokens:   sqlite.NewIngestTokenStore(sqliteDB),
		Topology:       sqlite.NewTopologyStore(sqliteDB),
		Budget:         sqlite.NewBudgetStore(sqliteDB),
		EvalConfigs:    sqlite.NewEvalConfigStore(sqliteDB),
		EvalJobs:       sqlite.NewEvalJobStore(sqliteDB),
		AlertRules:     sqlite.NewAlertRuleStore(sqliteDB),
		Loops:          sqlite.NewLoopStore(sqliteDB),
		PIIConfigs:     sqlite.NewProjectPIIConfigStore(sqliteDB),
		SpanFeedback:   sqlite.NewSpanFeedbackStore(sqliteDB),
		Playground:     sqlite.NewPlaygroundStore(sqliteDB),
		RunTags:        sqlite.NewRunTagStore(sqliteDB),
		RunAnnotations: sqlite.NewRunAnnotationStore(sqliteDB),
		PushSubs:       sqlite.NewPushSubscriptionStore(sqliteDB),
		EmailDigests:   sqlite.NewEmailDigestStore(sqliteDB),
		Retention:      sqlite.NewRetentionStore(sqliteDB),
		PurgeJobs:      sqlite.NewPurgeJobStore(sqliteDB),

		// DuckDB columnar stores
		Spans:     duckdb.NewSpanStore(duckDB),
		Runs:      duckdb.NewRunStore(duckDB),
		Sessions:  duckdb.NewSessionStore(duckDB),
		Users:     duckdb.NewUserStore(duckDB),
		Search:    duckdb.NewSearchStore(duckDB),
		Evals:     duckdb.NewEvalStore(duckDB),
		Analytics: duckdb.NewAnalyticsStore(duckDB),
		Exports:   duckdb.NewExportStore(duckDB),

		// Filesystem payloads
		Payloads: payloads,
	}
	return b, nil
}
