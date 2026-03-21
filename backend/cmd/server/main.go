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
	"github.com/agentpulse/agentpulse/backend/internal/api"
	"github.com/agentpulse/agentpulse/backend/internal/config"
	chstore "github.com/agentpulse/agentpulse/backend/internal/store/clickhouse"
	pgstore "github.com/agentpulse/agentpulse/backend/internal/store/postgres"
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
	projectStore := pgstore.NewProjectStore(pgPool)
	topologyStore := pgstore.NewTopologyStore(pgPool)
	budgetStore := pgstore.NewBudgetStore(pgPool)

	// ── Root context (cancelled on SIGINT/SIGTERM) ─────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Alert hub ─────────────────────────────────────────────────────────
	hub := alert.NewHub()
	go hub.Run()
	go alert.NewPoller(pgPool, hub).Run(ctx)

	// ── HTTP server ───────────────────────────────────────────────────────
	router := api.NewRouter(projectStore, runStore, spanStore, topologyStore, budgetStore, hub)

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
