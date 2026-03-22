package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// LoopStore implements store.LoopStore against Postgres.
type LoopStore struct {
	pool *pgxpool.Pool
}

func NewLoopStore(pool *pgxpool.Pool) *LoopStore {
	return &LoopStore{pool: pool}
}

// Upsert inserts or updates a RunLoop record, deduplicating on
// (run_id, detection_type, span_name, input_hash, output_hash).
func (s *LoopStore) Upsert(ctx context.Context, loop *domain.RunLoop) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO run_loops
		  (run_id, project_id, detection_type, span_name, input_hash, output_hash,
		   confidence, occurrence_count, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (run_id, detection_type, span_name, input_hash, output_hash)
		DO UPDATE SET
		  occurrence_count = EXCLUDED.occurrence_count,
		  detected_at      = EXCLUDED.detected_at
	`,
		loop.RunID, loop.ProjectID, loop.DetectionType,
		loop.SpanName, loop.InputHash, loop.OutputHash,
		loop.Confidence, loop.OccurrenceCount, loop.DetectedAt,
	)
	if err != nil {
		return fmt.Errorf("loop_store upsert: %w", err)
	}
	return nil
}

// ListByRun returns all detected loops for a run, ordered by confidence then occurrence count.
func (s *LoopStore) ListByRun(ctx context.Context, runID string) ([]*domain.RunLoop, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, project_id, detection_type, span_name,
		       input_hash, output_hash, confidence, occurrence_count, detected_at
		FROM run_loops
		WHERE run_id = $1
		ORDER BY confidence DESC, occurrence_count DESC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("loop_store list_by_run: %w", err)
	}
	defer rows.Close()

	var out []*domain.RunLoop
	for rows.Next() {
		l := &domain.RunLoop{}
		if err := rows.Scan(
			&l.ID, &l.RunID, &l.ProjectID, &l.DetectionType, &l.SpanName,
			&l.InputHash, &l.OutputHash, &l.Confidence, &l.OccurrenceCount, &l.DetectedAt,
		); err != nil {
			return nil, fmt.Errorf("loop_store list_by_run scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// HasLoops returns a map of runID -> true for any run IDs that have detected loops.
func (s *LoopStore) HasLoops(ctx context.Context, runIDs []string) (map[string]bool, error) {
	if len(runIDs) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT run_id FROM run_loops WHERE run_id = ANY($1)
	`, runIDs)
	if err != nil {
		return nil, fmt.Errorf("loop_store has_loops: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("loop_store has_loops scan: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}

// CountByProject returns the number of distinct runs with detected loops
// within the given window (windowSeconds).
func (s *LoopStore) CountByProject(ctx context.Context, projectID string, windowSeconds int) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT run_id)
		FROM run_loops
		WHERE project_id = $1
		  AND detected_at >= now() - ($2 * interval '1 second')
	`, projectID, windowSeconds).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("loop_store count_by_project: %w", err)
	}
	return count, nil
}
