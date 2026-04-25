package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/alerteval"
	"github.com/agentpulse/agentpulse/backend/internal/api"
	"github.com/agentpulse/agentpulse/backend/internal/audit"
	"github.com/agentpulse/agentpulse/backend/internal/bootstrap"
	"github.com/agentpulse/agentpulse/backend/internal/config"
	"github.com/agentpulse/agentpulse/backend/internal/emaildigest"
	"github.com/agentpulse/agentpulse/backend/internal/eval"
	"github.com/agentpulse/agentpulse/backend/internal/llmclient"
	"github.com/agentpulse/agentpulse/backend/internal/loopdetect"
	"github.com/agentpulse/agentpulse/backend/internal/pricing"
	"github.com/agentpulse/agentpulse/backend/internal/pushnotify"
	"github.com/agentpulse/agentpulse/backend/internal/storage"
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

	switch cfg.Mode {
	case config.ModeIndie:
		runIndie(cfg)
	case config.ModeTeam:
		runTeam(cfg)
	default:
		slog.Error("invalid mode", "mode", cfg.Mode)
		os.Exit(1)
	}
}

// runTeam preserves the original Postgres + ClickHouse + S3 + external
// collector deployment. Behaviour is unchanged from before P0-1.
func runTeam(cfg *config.Config) {
	cfg.WarnDefaults(slog.Warn)
	if os.Getenv("APP_ENV") == "production" {
		cfg.ErrorDefaults(func(msg string, args ...any) {
			slog.Error(msg, args...)
			os.Exit(1)
		})
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bundle, err := bootstrap.TeamStores(ctx, cfg)
	if err != nil {
		slog.Error("team mode bootstrap", "error", err)
		os.Exit(1)
	}
	defer bundle.Close()

	auditWriter := audit.NewWriter(bundle.ClickHouseConn())
	startBackgroundServices(ctx, cfg, bundle, auditWriter, true /*startCollectorServices*/)
	startHTTP(ctx, cfg, bundle, auditWriter, nil /*indieReceiverHandler*/)
}

// runIndie starts the single-binary deployment: SQLite + DuckDB + local FS,
// plus the embedded OTLP receiver in place of the external collector.
func runIndie(cfg *config.Config) {
	dataDir, err := bootstrap.EnsureIndieDataDir(cfg.Indie.DataDir)
	if err != nil {
		slog.Error("indie: data dir", "error", err)
		os.Exit(1)
	}
	slog.Info("indie mode", "data_dir", dataDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bundle, err := bootstrap.IndieStores(ctx, dataDir)
	if err != nil {
		if errors.Is(err, bootstrap.ErrIndieDuckDBMissing) {
			slog.Error("indie mode unavailable", "error", err)
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}
		slog.Error("indie mode bootstrap", "error", err)
		os.Exit(1)
	}
	defer bundle.Close()

	first, err := bootstrap.MaybeBootstrapFirstRun(ctx, bundle.Projects, bundle.IngestTokens, dataDir, slog.Default())
	if err != nil {
		slog.Error("indie: first-run bootstrap", "error", err)
		os.Exit(1)
	}
	bootstrap.PrintFirstRun(first)

	otlpHandler, err := startIndieOTLP(ctx, cfg, bundle)
	if err != nil {
		slog.Error("indie: otlp receiver", "error", err)
		os.Exit(1)
	}

	auditWriter := audit.NewWriter(nil) // no-op in indie mode

	startBackgroundServices(ctx, cfg, bundle, auditWriter, false /*team-only services off*/)
	startHTTP(ctx, cfg, bundle, auditWriter, otlpHandler)
}

// startBackgroundServices spins up the alert hub, eval workers, retention
// enforcer, etc. The team-only flag short-circuits services that don't make
// sense in indie mode (where most stubs short-circuit themselves anyway).
func startBackgroundServices(
	ctx context.Context,
	cfg *config.Config,
	bundle *bootstrap.StoreBundle,
	auditWriter *audit.Writer,
	startCollectorServices bool,
) {
	zapLogger, err := zap.NewProduction()
	if err != nil {
		slog.Error("zap logger init", "error", err)
		os.Exit(1)
	}

	if !startCollectorServices {
		// Indie mode: skip team-only background services that would hammer the stubs.
		_ = zapLogger
		return
	}

	statsService := storage.NewStatsService(bundle.ClickHouseConn(), bundle.PgPool, bundle.Payloads)
	purgeExecutor := storage.NewPurgeExecutor(bundle.ClickHouseConn(), bundle.PgPool, bundle.Payloads, bundle.PurgeJobs, bundle.Runs, zapLogger)
	retentionEnforcer := storage.NewRetentionEnforcer(bundle.Retention, bundle.PurgeJobs, purgeExecutor, 6*time.Hour, zapLogger)

	hub := alert.NewHub(bundle.Projects)
	go hub.Run()
	go alert.NewPoller(bundle.PgPool, hub).Run(ctx)

	var pushSender *pushnotify.Sender
	if cfg.WebPush.VAPIDPublicKey != "" && cfg.WebPush.VAPIDPrivateKey != "" {
		pushSender = pushnotify.NewSender(cfg.WebPush.VAPIDPublicKey, cfg.WebPush.VAPIDPrivateKey, cfg.WebPush.Subject, bundle.PushSubs)
	}

	go alerteval.NewEvaluator(bundle.ClickHouseConn(), bundle.AlertRules, hub, bundle.Loops, pushSender).Run(ctx)
	retentionEnforcer.Start(ctx)

	emailDigestSender := emaildigest.NewSender(cfg.Email.ResendAPIKey, cfg.Email.FromAddress, bundle.EmailDigests, bundle.AlertRules)
	go emailDigestSender.Run(ctx)
	go loopdetect.NewDetector(bundle.ClickHouseConn(), bundle.PgPool, bundle.Loops, bundle.Topology, bundle.Projects).Run(ctx)

	providerKeys := eval.ProviderKeys{
		Anthropic: cfg.AnthropicAPIKey,
		OpenAI:    cfg.OpenAIAPIKey,
		Google:    cfg.GoogleAIAPIKey,
	}
	if cfg.AnthropicAPIKey != "" || cfg.OpenAIAPIKey != "" || cfg.GoogleAIAPIKey != "" {
		go eval.NewEnqueuer(bundle.ClickHouseConn(), bundle.EvalJobs, bundle.EvalConfigs).Run(ctx)
		go eval.NewWorker(bundle.ClickHouseConn(), bundle.EvalJobs, bundle.Evals, bundle.EvalConfigs, providerKeys, bundle.Payloads).Run(ctx)
	}

	_ = auditWriter
	_ = statsService
}

// startHTTP wires the API router and listens. In indie mode it also serves
// the embedded OTLP receiver alongside on a separate listener.
func startHTTP(
	ctx context.Context,
	cfg *config.Config,
	bundle *bootstrap.StoreBundle,
	auditWriter *audit.Writer,
	indieOTLP http.Handler,
) {
	if !cfg.CORS.DevMode && len(cfg.CORS.AllowedOrigins) == 0 {
		slog.Warn("CORS_ALLOWED_ORIGINS is not set in production mode — all browser cross-origin requests will be blocked")
	}

	zapLogger, _ := zap.NewProduction()
	statsService := storage.NewStatsService(bundle.ClickHouseConn(), bundle.PgPool, bundle.Payloads)
	purgeExecutor := storage.NewPurgeExecutor(bundle.ClickHouseConn(), bundle.PgPool, bundle.Payloads, bundle.PurgeJobs, bundle.Runs, zapLogger)
	hub := alert.NewHub(bundle.Projects) // separate hub if indie didn't start one — harmless

	providerKeys := eval.ProviderKeys{
		Anthropic: cfg.AnthropicAPIKey,
		OpenAI:    cfg.OpenAIAPIKey,
		Google:    cfg.GoogleAIAPIKey,
	}

	modelPricingPath := os.Getenv("MODEL_PRICING_PATH")
	if modelPricingPath == "" {
		modelPricingPath = "config/model_pricing.yaml"
	}
	var pricingTable *pricing.Table
	var llmClient llmclient.Client
	if pt, err := pricing.Load(modelPricingPath); err == nil {
		pricingTable = pt
		providerMap := make(map[string]string, len(pt.Models))
		for id, m := range pt.Models {
			providerMap[id] = m.Provider
		}
		llmClient = llmclient.New(llmclient.ProviderKeys{
			Anthropic: cfg.AnthropicAPIKey,
			OpenAI:    cfg.OpenAIAPIKey,
			Google:    cfg.GoogleAIAPIKey,
		}, providerMap)
	} else {
		slog.Warn("pricing table not loaded — Playground disabled", "error", err)
	}

	router := api.NewRouter(
		bundle.Projects, bundle.Runs, bundle.Spans, bundle.Topology, bundle.Budget,
		bundle.Evals, bundle.EvalConfigs, bundle.AlertRules, bundle.Analytics,
		bundle.Loops, bundle.Sessions, bundle.Users, bundle.Search, bundle.PIIConfigs,
		bundle.SpanFeedback, bundle.Payloads, bundle.Playground, bundle.Exports,
		bundle.RunTags, bundle.RunAnnotations, bundle.PushSubs, bundle.EmailDigests,
		bundle.IngestTokens, bundle.Retention, bundle.PurgeJobs,
		statsService, purgeExecutor,
		cfg.WebPush.VAPIDPublicKey, bundle.PgPool, hub,
		cfg.CORS.AllowedOrigins, cfg.CORS.DevMode,
		providerKeys, llmClient, pricingTable, auditWriter,
	)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if cfg.TLSEnabled() {
			slog.Info("starting AgentPulse API (TLS)", "addr", cfg.HTTPAddr())
			if err := srv.ListenAndServeTLS(cfg.HTTP.TLSCert, cfg.HTTP.TLSKey); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Info("starting AgentPulse API", "addr", cfg.HTTPAddr())
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		}
	}()

	// Indie-mode: also stand up the embedded OTLP listener.
	var otlpSrv *http.Server
	if indieOTLP != nil {
		otlpSrv = &http.Server{
			Addr:         cfg.Indie.OTLPAddr,
			Handler:      indieOTLP,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		go func() {
			slog.Info("starting embedded OTLP receiver", "addr", cfg.Indie.OTLPAddr)
			if err := otlpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("otlp receiver error", "error", err)
				os.Exit(1)
			}
		}()
	}

	<-ctx.Done()

	slog.Info("shutting down server...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	if otlpSrv != nil {
		_ = otlpSrv.Shutdown(shutCtx)
	}
	slog.Info("server stopped")
}
