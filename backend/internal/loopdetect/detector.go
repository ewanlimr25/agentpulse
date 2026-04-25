package loopdetect

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const detectorInterval = 60 * time.Second
const defaultLookback = 10 * time.Minute

// Detector is a background worker that scans for agent loops.
type Detector struct {
	ch           driver.Conn
	pg           *pgxpool.Pool
	loopStore    store.LoopStore
	topoStore    store.TopologyStore
	projectStore store.ProjectStore
	mu           sync.Mutex
	watermarks   map[string]time.Time // project_id -> last scanned at
}

func NewDetector(ch driver.Conn, pg *pgxpool.Pool, loopStore store.LoopStore, topoStore store.TopologyStore, projectStore store.ProjectStore) *Detector {
	return &Detector{
		ch:           ch,
		pg:           pg,
		loopStore:    loopStore,
		topoStore:    topoStore,
		projectStore: projectStore,
		watermarks:   make(map[string]time.Time),
	}
}

func (d *Detector) Run(ctx context.Context) {
	// Ensure watermarks table exists
	if err := d.ensureWatermarksTable(ctx); err != nil {
		slog.Error("loopdetect watermarks table", "error", err)
	}
	// Load persisted watermarks
	if err := d.loadWatermarks(ctx); err != nil {
		slog.Warn("loopdetect load watermarks", "error", err)
	}

	ticker := time.NewTicker(detectorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.detectAll(ctx)
		}
	}
}

func (d *Detector) detectAll(ctx context.Context) {
	projects, err := d.projectStore.List(ctx)
	if err != nil {
		slog.Error("loopdetect list projects", "error", err)
		return
	}
	for _, p := range projects {
		if err := d.detectProject(ctx, p); err != nil {
			slog.Error("loopdetect project", "project_id", p.ID, "error", err)
		}
	}
}

func (d *Detector) detectProject(ctx context.Context, p *domain.Project) error {
	since := d.getWatermark(p.ID)

	cfg, err := d.projectStore.GetLoopConfig(ctx, p.ID)
	if err != nil {
		slog.Warn("loopdetect get loop config, using defaults", "project_id", p.ID, "error", err)
		def := domain.DefaultLoopConfig
		cfg = &def
	}

	results, err := QueryRepeatedToolCalls(ctx, d.ch, p.ID, since, *cfg)
	if err != nil {
		return fmt.Errorf("query repeated calls: %w", err)
	}

	seenRuns := make(map[string]bool)
	for _, r := range results {
		loop := &domain.RunLoop{
			RunID:           r.RunID,
			ProjectID:       p.ID,
			DetectionType:   "repeated_call",
			SpanName:        r.SpanName,
			InputHash:       r.InputHash,
			OutputHash:      r.OutputHash,
			Confidence:      r.Confidence,
			OccurrenceCount: r.Count,
			DetectedAt:      time.Now(),
		}
		if err := d.loopStore.Upsert(ctx, loop); err != nil {
			slog.Warn("loopdetect upsert", "run_id", r.RunID, "error", err)
		}
		seenRuns[r.RunID] = true
	}

	// Topology cycle detection for flagged runs
	for runID := range seenRuns {
		topo, err := d.topoStore.GetByRun(ctx, runID)
		if err != nil || topo == nil {
			continue
		}
		cycles := DetectCycles(topo)
		for _, c := range cycles {
			loop := &domain.RunLoop{
				RunID:         runID,
				ProjectID:     p.ID,
				DetectionType: "topology_cycle",
				Confidence:    "high",
				SpanName:      fmt.Sprintf("cycle:%v", c.NodeIDs),
				DetectedAt:    time.Now(),
			}
			if err := d.loopStore.Upsert(ctx, loop); err != nil {
				slog.Warn("loopdetect cycle upsert", "run_id", runID, "error", err)
			}
		}
	}

	d.saveWatermark(ctx, p.ID, time.Now())
	return nil
}

func (d *Detector) getWatermark(projectID string) time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.watermarks[projectID]; ok {
		return t
	}
	return time.Now().Add(-defaultLookback)
}

func (d *Detector) ensureWatermarksTable(ctx context.Context) error {
	_, err := d.pg.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS loop_detector_watermarks (
			project_id      TEXT PRIMARY KEY,
			last_scanned_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func (d *Detector) loadWatermarks(ctx context.Context) error {
	rows, err := d.pg.Query(ctx, `SELECT project_id, last_scanned_at FROM loop_detector_watermarks`)
	if err != nil {
		return err
	}
	defer rows.Close()
	d.mu.Lock()
	defer d.mu.Unlock()
	for rows.Next() {
		var pid string
		var t time.Time
		if err := rows.Scan(&pid, &t); err == nil {
			d.watermarks[pid] = t
		}
	}
	return rows.Err()
}

func (d *Detector) saveWatermark(ctx context.Context, projectID string, t time.Time) {
	d.mu.Lock()
	d.watermarks[projectID] = t
	d.mu.Unlock()
	_, err := d.pg.Exec(ctx, `
		INSERT INTO loop_detector_watermarks (project_id, last_scanned_at)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET last_scanned_at = EXCLUDED.last_scanned_at
	`, projectID, t)
	if err != nil {
		slog.Warn("loopdetect save watermark", "project_id", projectID, "error", err)
	}
}
