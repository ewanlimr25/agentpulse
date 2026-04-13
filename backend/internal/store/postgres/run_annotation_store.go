package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// RunAnnotationStore implements store.RunAnnotationStore against Postgres.
type RunAnnotationStore struct {
	pool *pgxpool.Pool
}

func NewRunAnnotationStore(pool *pgxpool.Pool) *RunAnnotationStore {
	return &RunAnnotationStore{pool: pool}
}

const runAnnotationColumns = `id, project_id, run_id, note, created_at, updated_at`

func scanRunAnnotation(row pgx.Row) (*domain.RunAnnotation, error) {
	a := &domain.RunAnnotation{}
	err := row.Scan(&a.ID, &a.ProjectID, &a.RunID, &a.Note, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Upsert creates or replaces the annotation for a run.
// Keyed on (project_id, run_id); updating always refreshes updated_at.
func (s *RunAnnotationStore) Upsert(ctx context.Context, a *domain.RunAnnotation) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO run_annotations (id, project_id, run_id, note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, run_id) DO UPDATE SET
			note       = EXCLUDED.note,
			updated_at = now()
	`, a.ID, a.ProjectID, a.RunID, a.Note, now, now)
	if err != nil {
		return fmt.Errorf("run_annotation_store upsert: %w", err)
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

// Get returns the annotation for a run, or nil if none exists.
func (s *RunAnnotationStore) Get(ctx context.Context, projectID, runID string) (*domain.RunAnnotation, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+runAnnotationColumns+`
		FROM run_annotations
		WHERE project_id = $1 AND run_id = $2
	`, projectID, runID)
	a, err := scanRunAnnotation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("run_annotation_store get: %w", err)
	}
	return a, nil
}

// GetByRuns returns a map of runID → *RunAnnotation for multiple runs in one query.
// Runs that have no annotation will not appear in the returned map.
func (s *RunAnnotationStore) GetByRuns(ctx context.Context, projectID string, runIDs []string) (map[string]*domain.RunAnnotation, error) {
	if len(runIDs) == 0 {
		return map[string]*domain.RunAnnotation{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runAnnotationColumns+`
		FROM run_annotations
		WHERE project_id = $1 AND run_id = ANY($2::text[])
	`, projectID, runIDs)
	if err != nil {
		return nil, fmt.Errorf("run_annotation_store get_by_runs: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*domain.RunAnnotation)
	for rows.Next() {
		a, err := scanRunAnnotation(rows)
		if err != nil {
			return nil, fmt.Errorf("run_annotation_store get_by_runs scan: %w", err)
		}
		result[a.RunID] = a
	}
	return result, rows.Err()
}

// Delete removes the annotation for a run. No-op if none exists.
func (s *RunAnnotationStore) Delete(ctx context.Context, projectID, runID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM run_annotations WHERE project_id = $1 AND run_id = $2
	`, projectID, runID)
	if err != nil {
		return fmt.Errorf("run_annotation_store delete: %w", err)
	}
	return nil
}
