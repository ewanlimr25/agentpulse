package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// RunAnnotationStore implements store.RunAnnotationStore against SQLite.
type RunAnnotationStore struct {
	db *sql.DB
}

func NewRunAnnotationStore(db *sql.DB) *RunAnnotationStore {
	return &RunAnnotationStore{db: db}
}

const runAnnotationColumns = `id, project_id, run_id, note, created_at, updated_at`

type runAnnotationScanner interface {
	Scan(dest ...any) error
}

func scanRunAnnotation(s runAnnotationScanner) (*domain.RunAnnotation, error) {
	a := &domain.RunAnnotation{}
	if err := s.Scan(&a.ID, &a.ProjectID, &a.RunID, &a.Note, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *RunAnnotationStore) Upsert(ctx context.Context, a *domain.RunAnnotation) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_annotations (id, project_id, run_id, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, run_id) DO UPDATE SET
			note       = excluded.note,
			updated_at = excluded.updated_at
	`, a.ID, a.ProjectID, a.RunID, a.Note, now, now)
	if err != nil {
		return fmt.Errorf("run_annotation_store upsert: %w", err)
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

func (s *RunAnnotationStore) Get(ctx context.Context, projectID, runID string) (*domain.RunAnnotation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+runAnnotationColumns+`
		FROM run_annotations
		WHERE project_id = ? AND run_id = ?`, projectID, runID)
	a, err := scanRunAnnotation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("run_annotation_store get: %w", err)
	}
	return a, nil
}

func (s *RunAnnotationStore) GetByRuns(ctx context.Context, projectID string, runIDs []string) (map[string]*domain.RunAnnotation, error) {
	if len(runIDs) == 0 {
		return map[string]*domain.RunAnnotation{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, 0, len(runIDs)+1)
	args = append(args, projectID)
	for _, id := range runIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+runAnnotationColumns+`
		FROM run_annotations
		WHERE project_id = ? AND run_id IN (`+placeholders+`)`, args...)
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

func (s *RunAnnotationStore) Delete(ctx context.Context, projectID, runID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM run_annotations WHERE project_id = ? AND run_id = ?`, projectID, runID)
	if err != nil {
		return fmt.Errorf("run_annotation_store delete: %w", err)
	}
	return nil
}
