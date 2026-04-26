package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// LoopStore implements store.LoopStore against SQLite.
type LoopStore struct {
	db *sql.DB
}

func NewLoopStore(db *sql.DB) *LoopStore {
	return &LoopStore{db: db}
}

// Upsert inserts or updates a RunLoop record, deduplicating on
// (run_id, detection_type, span_name, input_hash, output_hash).
func (s *LoopStore) Upsert(ctx context.Context, loop *domain.RunLoop) error {
	if loop.ID == "" {
		loop.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_loops
		  (id, run_id, project_id, detection_type, span_name, input_hash, output_hash,
		   confidence, occurrence_count, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, detection_type, span_name, input_hash, output_hash)
		DO UPDATE SET
		  occurrence_count = excluded.occurrence_count,
		  detected_at      = excluded.detected_at
	`,
		loop.ID, loop.RunID, loop.ProjectID, loop.DetectionType,
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, project_id, detection_type, span_name,
		       input_hash, output_hash, confidence, occurrence_count, detected_at
		FROM run_loops
		WHERE run_id = ?
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
// Builds an IN (?,?,?...) clause dynamically since SQLite has no array param type.
func (s *LoopStore) HasLoops(ctx context.Context, runIDs []string) (map[string]bool, error) {
	if len(runIDs) == 0 {
		return map[string]bool{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, len(runIDs))
	for i, id := range runIDs {
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT DISTINCT run_id FROM run_loops WHERE run_id IN (%s)`, placeholders)
	rows, err := s.db.QueryContext(ctx, q, args...)
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
// within the given window (windowSeconds). The cutoff is computed in Go
// and passed as a time parameter (cleaner than SQLite's datetime() math).
func (s *LoopStore) CountByProject(ctx context.Context, projectID string, windowSeconds int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowSeconds) * time.Second)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(DISTINCT run_id)
		FROM run_loops
		WHERE project_id = ?
		  AND detected_at >= ?
	`, projectID, cutoff).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("loop_store count_by_project: %w", err)
	}
	return count, nil
}
