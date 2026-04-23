package storage

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const defaultEnforcerInterval = 6 * time.Hour

// gracePeriod is subtracted from the retention boundary to avoid purging
// data that just crossed the retention threshold.
const gracePeriod = 2 * time.Hour

// RetentionEnforcer is a background worker that enforces per-project retention policies.
// It follows the same ticker-based pattern as loopdetect.Detector.
type RetentionEnforcer struct {
	retention store.RetentionStore
	purgeJobs store.PurgeJobStore
	executor  *PurgeExecutor
	interval  time.Duration
	logger    *zap.Logger
	done      chan struct{}
}

// NewRetentionEnforcer creates a RetentionEnforcer with the given interval (use 0 for default 6h).
func NewRetentionEnforcer(
	retention store.RetentionStore,
	purgeJobs store.PurgeJobStore,
	executor *PurgeExecutor,
	interval time.Duration,
	logger *zap.Logger,
) *RetentionEnforcer {
	if interval <= 0 {
		interval = defaultEnforcerInterval
	}
	return &RetentionEnforcer{
		retention: retention,
		purgeJobs: purgeJobs,
		executor:  executor,
		interval:  interval,
		logger:    logger,
		done:      make(chan struct{}),
	}
}

// Start launches the background enforcement goroutine.
// It returns when ctx is cancelled.
func (e *RetentionEnforcer) Start(ctx context.Context) {
	go e.run(ctx)
}

func (e *RetentionEnforcer) run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.enforceAll(ctx)
		}
	}
}

func (e *RetentionEnforcer) enforceAll(ctx context.Context) {
	configs, err := e.retention.ListAll(ctx)
	if err != nil {
		e.logger.Error("retention enforcer: list all configs", zap.Error(err))
		return
	}

	for _, cfg := range configs {
		if err := e.enforceProject(ctx, cfg); err != nil {
			e.logger.Error("retention enforcer: enforce project",
				zap.String("project_id", cfg.ProjectID),
				zap.Error(err),
			)
		}
	}
}

func (e *RetentionEnforcer) enforceProject(ctx context.Context, cfg *domain.RetentionConfig) error {
	cutoff := time.Now().
		Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour).
		Add(-gracePeriod)

	job := &domain.PurgeJob{
		ProjectID:    cfg.ProjectID,
		CutoffDate:   &cutoff,
		IncludeEvals: false,
		Status:       "pending",
	}
	if err := e.purgeJobs.Create(ctx, job); err != nil {
		return fmt.Errorf("retention enforcer: create purge job: %w", err)
	}

	// Run the purge synchronously in this goroutine (the goroutine is already background).
	// ExecuteAgePurge issues the CH mutations and returns immediately — mutations are async on CH side.
	if err := e.executor.ExecuteAgePurge(ctx, job); err != nil {
		return fmt.Errorf("retention enforcer: execute age purge: %w", err)
	}
	return nil
}
