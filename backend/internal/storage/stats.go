package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const statsCacheTTL = 5 * time.Minute

type cachedStats struct {
	stats  *domain.StorageStats
	expiry time.Time
}

// StatsService computes storage stats across ClickHouse, Postgres, and S3.
// Results are cached per project for 5 minutes to avoid expensive repeated scans.
type StatsService struct {
	ch       driver.Conn
	pg       *pgxpool.Pool
	payloads store.PayloadStore
	cache    sync.Map // projectID → cachedStats
}

// NewStatsService creates a StatsService.
func NewStatsService(ch driver.Conn, pg *pgxpool.Pool, payloads store.PayloadStore) *StatsService {
	return &StatsService{ch: ch, pg: pg, payloads: payloads}
}

// GetStats returns storage stats for a project, using a 5-minute in-memory cache.
func (s *StatsService) GetStats(ctx context.Context, projectID string) (*domain.StorageStats, error) {
	// Check cache.
	if v, ok := s.cache.Load(projectID); ok {
		entry := v.(cachedStats)
		if time.Now().Before(entry.expiry) {
			return entry.stats, nil
		}
	}

	stats, err := s.compute(ctx, projectID)
	if err != nil {
		return nil, err
	}

	s.cache.Store(projectID, cachedStats{stats: stats, expiry: time.Now().Add(statsCacheTTL)})
	return stats, nil
}

func (s *StatsService) compute(ctx context.Context, projectID string) (*domain.StorageStats, error) {
	result := &domain.StorageStats{
		ProjectID:   projectID,
		StatsApprox: true,
		ComputedAt:  time.Now(),
	}

	type chResult struct {
		spanCount    int64
		spanBytes    int64
		oldestSpanAt *time.Time
		newestSpanAt *time.Time
	}
	type pgResult struct {
		topologyRows int64
	}
	type s3Result struct {
		objectCount int64
		bytes       int64
	}

	chCh := make(chan chResult, 1)
	pgCh := make(chan pgResult, 1)
	s3Ch := make(chan s3Result, 1)
	errCh := make(chan error, 3)

	// Fan out queries in parallel.
	go func() {
		r, err := s.fetchCHStats(ctx, projectID)
		if err != nil {
			errCh <- fmt.Errorf("ch stats: %w", err)
			return
		}
		chCh <- r
	}()

	go func() {
		r, err := s.fetchPGStats(ctx, projectID)
		if err != nil {
			errCh <- fmt.Errorf("pg stats: %w", err)
			return
		}
		pgCh <- r
	}()

	go func() {
		if s.payloads == nil {
			s3Ch <- s3Result{}
			return
		}
		count, bytes, err := s.payloads.StatsByPrefix(ctx, projectID+"/")
		if err != nil {
			errCh <- fmt.Errorf("s3 stats: %w", err)
			return
		}
		s3Ch <- s3Result{objectCount: count, bytes: bytes}
	}()

	// Collect results — all three goroutines must complete.
	var firstErr error
	for range 3 {
		select {
		case r := <-chCh:
			result.SpanRowCount = r.spanCount
			result.SpanBytesEst = r.spanBytes
			result.OldestSpanAt = r.oldestSpanAt
			result.NewestSpanAt = r.newestSpanAt
		case r := <-pgCh:
			result.TopologyRows = r.topologyRows
		case r := <-s3Ch:
			result.S3ObjectCount = r.objectCount
			result.S3Bytes = r.bytes
		case err := <-errCh:
			if firstErr == nil {
				firstErr = err
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

func (s *StatsService) fetchCHStats(ctx context.Context, projectID string) (chResult struct {
	spanCount    int64
	spanBytes    int64
	oldestSpanAt *time.Time
	newestSpanAt *time.Time
}, err error) {
	// Row count and time bounds.
	var count uint64
	var oldest, newest time.Time
	errScan := s.ch.QueryRow(ctx,
		"SELECT count(), min(start_time), max(start_time) FROM spans WHERE project_id = ?",
		projectID,
	).Scan(&count, &oldest, &newest)
	if errScan != nil {
		err = fmt.Errorf("ch span stats: %w", errScan)
		return
	}
	chResult.spanCount = int64(count)
	if !oldest.IsZero() {
		t := oldest
		chResult.oldestSpanAt = &t
	}
	if !newest.IsZero() {
		t := newest
		chResult.newestSpanAt = &t
	}

	// Estimated bytes from system.parts — project share approximated by row ratio.
	var totalBytes uint64
	var totalRows uint64
	_ = s.ch.QueryRow(ctx,
		"SELECT sum(data_compressed_bytes), sum(rows) FROM system.parts WHERE table = 'spans' AND active = 1",
	).Scan(&totalBytes, &totalRows)
	if totalRows > 0 && count > 0 {
		chResult.spanBytes = int64(float64(totalBytes) * float64(count) / float64(totalRows))
	}
	return
}

func (s *StatsService) fetchPGStats(ctx context.Context, projectID string) (pgResult struct {
	topologyRows int64
}, err error) {
	var count int64
	err = s.pg.QueryRow(ctx,
		"SELECT count(*) FROM topology_nodes WHERE project_id = $1",
		projectID,
	).Scan(&count)
	if err != nil {
		err = fmt.Errorf("pg topology_nodes count: %w", err)
		return
	}
	pgResult.topologyRows = count
	return
}
