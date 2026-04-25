package bootstrap

import (
	"context"
	"fmt"

	"github.com/agentpulse/agentpulse/backend/internal/config"
	"github.com/agentpulse/agentpulse/backend/internal/store"
	chstore "github.com/agentpulse/agentpulse/backend/internal/store/clickhouse"
	pgstore "github.com/agentpulse/agentpulse/backend/internal/store/postgres"
	s3store "github.com/agentpulse/agentpulse/backend/internal/store/s3"
)

// TeamStores opens Postgres + ClickHouse + S3 (when configured) and returns a
// fully-populated StoreBundle. Missing required schema migrations surface as
// an error here rather than crashing later.
func TeamStores(ctx context.Context, cfg *config.Config) (*StoreBundle, error) {
	chConn, err := chstore.Open(cfg.ClickHouse)
	if err != nil {
		return nil, fmt.Errorf("clickhouse connect: %w", err)
	}

	pgPool, err := pgstore.Open(cfg.Postgres)
	if err != nil {
		chConn.Close()
		return nil, fmt.Errorf("postgres connect: %w", err)
	}

	if _, err := pgPool.Exec(ctx, "SELECT 1 FROM run_tags LIMIT 1"); err != nil {
		pgPool.Close()
		chConn.Close()
		return nil, fmt.Errorf("run_tags table not found — apply migration 011_run_tags_annotations.up.sql: %w", err)
	}
	if _, err := pgPool.Exec(ctx, "SELECT 1 FROM push_subscriptions LIMIT 1"); err != nil {
		pgPool.Close()
		chConn.Close()
		return nil, fmt.Errorf("push_subscriptions table not found — apply migration 013_push_subscriptions.up.sql: %w", err)
	}
	if _, err := pgPool.Exec(ctx, "SELECT 1 FROM project_ingest_tokens LIMIT 1"); err != nil {
		pgPool.Close()
		chConn.Close()
		return nil, fmt.Errorf("project_ingest_tokens table not found — apply migration 012_ingest_tokens.up.sql: %w", err)
	}

	var payloads store.PayloadStore
	if cfg.S3.Bucket != "" && cfg.S3.Endpoint != "" {
		ps, err := s3store.New(cfg.S3)
		if err != nil {
			pgPool.Close()
			chConn.Close()
			return nil, fmt.Errorf("s3 init: %w", err)
		}
		payloads = ps
	}

	b := &StoreBundle{
		PgPool: pgPool,

		Projects:        pgstore.NewProjectStore(pgPool),
		Topology:        pgstore.NewTopologyStore(pgPool),
		Budget:          pgstore.NewBudgetStore(pgPool),
		EvalConfigs:     pgstore.NewEvalConfigStore(pgPool),
		AlertRules:      pgstore.NewAlertRuleStore(pgPool),
		Loops:           pgstore.NewLoopStore(pgPool),
		PIIConfigs:      pgstore.NewProjectPIIConfigStore(pgPool),
		SpanFeedback:    pgstore.NewSpanFeedbackStore(pgPool),
		Playground:      pgstore.NewPlaygroundStore(pgPool),
		RunTags:         pgstore.NewRunTagStore(pgPool),
		RunAnnotations:  pgstore.NewRunAnnotationStore(pgPool),
		PushSubs:        pgstore.NewPushSubscriptionStore(pgPool),
		EmailDigests:    pgstore.NewEmailDigestStore(pgPool),
		IngestTokens:    pgstore.NewIngestTokenStore(pgPool),
		Retention:       pgstore.NewRetentionStore(pgPool),
		PurgeJobs:       pgstore.NewPurgeJobStore(pgPool),
		EvalJobs:        pgstore.NewEvalJobStore(pgPool),

		Spans:     chstore.NewSpanStore(chConn),
		Runs:      chstore.NewRunStore(chConn),
		Sessions:  chstore.NewSessionStore(chConn),
		Users:     chstore.NewUserStore(chConn),
		Search:    chstore.NewSearchStore(chConn),
		Evals:     chstore.NewEvalStore(chConn),
		Analytics: chstore.NewAnalyticsStore(chConn),
		Exports:   chstore.NewExportStore(chConn),

		Payloads: payloads,
	}
	// Stash the chConn on the bundle so cmd/server can reach it for components
	// that still take a raw driver.Conn (audit writer, eval workers, etc.).
	b.teamCH = chConn
	return b, nil
}
