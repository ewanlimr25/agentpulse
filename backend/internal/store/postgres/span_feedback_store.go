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

// SpanFeedbackStore implements store.SpanFeedbackStore against Postgres.
type SpanFeedbackStore struct {
	pool *pgxpool.Pool
}

func NewSpanFeedbackStore(pool *pgxpool.Pool) *SpanFeedbackStore {
	return &SpanFeedbackStore{pool: pool}
}

const spanFeedbackColumns = `id, project_id, span_id, run_id, rating, corrected_output, created_at, updated_at`

func scanSpanFeedback(row pgx.Row) (*domain.SpanFeedback, error) {
	f := &domain.SpanFeedback{}
	err := row.Scan(
		&f.ID, &f.ProjectID, &f.SpanID, &f.RunID,
		&f.Rating, &f.CorrectedOutput,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Upsert creates or replaces feedback for a span.
// The unique constraint on (project_id, span_id) ensures one record per span.
func (s *SpanFeedbackStore) Upsert(ctx context.Context, f *domain.SpanFeedback) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO span_feedback (id, project_id, span_id, run_id, rating, corrected_output, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, span_id) DO UPDATE SET
			rating           = EXCLUDED.rating,
			corrected_output = EXCLUDED.corrected_output,
			updated_at       = EXCLUDED.updated_at
	`,
		f.ID, f.ProjectID, f.SpanID, f.RunID, f.Rating, f.CorrectedOutput, now, now,
	)
	if err != nil {
		return fmt.Errorf("span_feedback_store upsert: %w", err)
	}
	f.CreatedAt = now
	f.UpdatedAt = now
	return nil
}

// GetBySpan returns the current feedback for a span, or nil if none exists.
func (s *SpanFeedbackStore) GetBySpan(ctx context.Context, projectID, spanID string) (*domain.SpanFeedback, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+spanFeedbackColumns+`
		FROM span_feedback
		WHERE project_id = $1 AND span_id = $2
	`, projectID, spanID)
	f, err := scanSpanFeedback(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("span_feedback_store get_by_span: %w", err)
	}
	return f, nil
}

// ListByRun returns all feedback records for spans in a run, ordered by creation time.
func (s *SpanFeedbackStore) ListByRun(ctx context.Context, projectID, runID string) ([]*domain.SpanFeedback, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+spanFeedbackColumns+`
		FROM span_feedback
		WHERE project_id = $1 AND run_id = $2
		ORDER BY created_at ASC
	`, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("span_feedback_store list_by_run: %w", err)
	}
	defer rows.Close()

	var out []*domain.SpanFeedback
	for rows.Next() {
		f, err := scanSpanFeedback(rows)
		if err != nil {
			return nil, fmt.Errorf("span_feedback_store list_by_run scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Delete removes feedback for a span. Both projectID and spanID are required
// to prevent cross-project deletion.
func (s *SpanFeedbackStore) Delete(ctx context.Context, projectID, spanID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM span_feedback WHERE project_id = $1 AND span_id = $2
	`, projectID, spanID)
	if err != nil {
		return fmt.Errorf("span_feedback_store delete: %w", err)
	}
	return nil
}

// ListAllByProject returns all feedback records for a project, ordered by creation time.
func (s *SpanFeedbackStore) ListAllByProject(ctx context.Context, projectID string) ([]*domain.SpanFeedback, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+spanFeedbackColumns+`
		FROM span_feedback
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("span_feedback_store list_all_by_project: %w", err)
	}
	defer rows.Close()

	var out []*domain.SpanFeedback
	for rows.Next() {
		f, err := scanSpanFeedback(rows)
		if err != nil {
			return nil, fmt.Errorf("span_feedback_store list_all_by_project scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CountByProject returns the total number of feedback records for a project.
func (s *SpanFeedbackStore) CountByProject(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM span_feedback WHERE project_id = $1
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("span_feedback_store count_by_project: %w", err)
	}
	return count, nil
}
