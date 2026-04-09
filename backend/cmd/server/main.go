package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/alerteval"
	"github.com/agentpulse/agentpulse/backend/internal/api"
	"github.com/agentpulse/agentpulse/backend/internal/config"
	"github.com/agentpulse/agentpulse/backend/internal/eval"
	"github.com/agentpulse/agentpulse/backend/internal/loopdetect"
	"github.com/agentpulse/agentpulse/backend/internal/store"
	chstore "github.com/agentpulse/agentpulse/backend/internal/store/clickhouse"
	pgstore "github.com/agentpulse/agentpulse/backend/internal/store/postgres"
	s3store "github.com/agentpulse/agentpulse/backend/internal/store/s3"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	cfg.WarnDefaults(slog.Warn)

	// ── Storage connections ────────────────────────────────────────────────
	chConn, err := chstore.Open(cfg.ClickHouse)
	if err != nil {
		slog.Error("clickhouse connect", "error", err)
		os.Exit(1)
	}
	defer chConn.Close()

	pgPool, err := pgstore.Open(cfg.Postgres)
	if err != nil {
		slog.Error("postgres connect", "error", err)
		os.Exit(1)
	}
	defer pgPool.Close()

	// ── Stores ────────────────────────────────────────────────────────────
	spanStore := chstore.NewSpanStore(chConn)
	runStore := chstore.NewRunStore(chConn)
	sessionStore := chstore.NewSessionStore(chConn)
	userStore := chstore.NewUserStore(chConn)
	searchStore := chstore.NewSearchStore(chConn)
	projectStore := pgstore.NewProjectStore(pgPool)
	topologyStore := pgstore.NewTopologyStore(pgPool)
	budgetStore := pgstore.NewBudgetStore(pgPool)
	evalStore := chstore.NewEvalStore(chConn)
	analyticsStore := chstore.NewAnalyticsStore(chConn)
	evalJobStore := pgstore.NewEvalJobStore(pgPool)
	evalConfigStore := pgstore.NewEvalConfigStore(pgPool)
	alertRuleStore := pgstore.NewAlertRuleStore(pgPool)
	loopStore := pgstore.NewLoopStore(pgPool)
	piiConfigStore := pgstore.NewProjectPIIConfigStore(pgPool)
	spanFeedbackStore := pgstore.NewSpanFeedbackStore(pgPool)

	// ── Payload store (S3 offloading) ─────────────────────────────────────
	var payloadStore store.PayloadStore
	if cfg.S3.Bucket != "" && cfg.S3.Endpoint != "" {
		ps, err := s3store.New(cfg.S3)
		if err != nil {
			slog.Error("s3 payload store init failed", "error", err)
			os.Exit(1)
		}
		payloadStore = ps
		slog.Info("S3 payload store initialized", "bucket", cfg.S3.Bucket)
	}

	// ── Root context (cancelled on SIGINT/SIGTERM) ─────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Alert hub ─────────────────────────────────────────────────────────
	hub := alert.NewHub(projectStore)
	go hub.Run()
	go alert.NewPoller(pgPool, hub).Run(ctx)
	go alerteval.NewEvaluator(chConn, alertRuleStore, hub, loopStore).Run(ctx)
	slog.Info("alert evaluator started")
	go loopdetect.NewDetector(chConn, pgPool, loopStore, topologyStore, projectStore).Run(ctx)
	slog.Info("loop detector started")

	// ── Eval workers ──────────────────────────────────────────────────────
	// Start the eval pipeline if at least one judge provider key is configured.
	providerKeys := eval.ProviderKeys{
		Anthropic: cfg.AnthropicAPIKey,
		OpenAI:    cfg.OpenAIAPIKey,
		Google:    cfg.GoogleAIAPIKey,
	}
	if cfg.AnthropicAPIKey != "" || cfg.OpenAIAPIKey != "" || cfg.GoogleAIAPIKey != "" {
		// Worker reloads registry from evalConfigStore every 60s to pick up new custom evals.
		go eval.NewEnqueuer(chConn, evalJobStore, evalConfigStore).Run(ctx)
		go eval.NewWorker(chConn, evalJobStore, evalStore, evalConfigStore, providerKeys, payloadStore).Run(ctx)
		slog.Info("eval worker started")
	} else {
		slog.Info("eval worker disabled (no judge provider API keys set)")
	}

	// ── HTTP server ───────────────────────────────────────────────────────
	if !cfg.CORS.DevMode && len(cfg.CORS.AllowedOrigins) == 0 {
		slog.Warn("CORS_ALLOWED_ORIGINS is not set in production mode — all browser cross-origin requests will be blocked")
	}
	router := api.NewRouter(projectStore, runStore, spanStore, topologyStore, budgetStore, evalStore, evalConfigStore, alertRuleStore, analyticsStore, loopStore, sessionStore, userStore, searchStore, piiConfigStore, spanFeedbackStore, payloadStore, pgPool, hub, cfg.CORS.AllowedOrigins, cfg.CORS.DevMode, providerKeys)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting AgentPulse API", "addr", cfg.HTTPAddr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	slog.Info("shutting down server...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("server stopped")
}
