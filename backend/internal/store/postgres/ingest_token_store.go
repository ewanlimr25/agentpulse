package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// IngestTokenStore implements store.IngestTokenStore against Postgres.
type IngestTokenStore struct {
	pool *pgxpool.Pool
}

func NewIngestTokenStore(pool *pgxpool.Pool) *IngestTokenStore {
	return &IngestTokenStore{pool: pool}
}

// Create inserts a new ingest token row and returns the created record.
func (s *IngestTokenStore) Create(ctx context.Context, projectID, tokenHash, label string) (*domain.IngestToken, error) {
	t := &domain.IngestToken{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO project_ingest_tokens (project_id, token_hash, label)
		VALUES ($1, $2, $3)
		RETURNING id, project_id, label, created_at
	`, projectID, tokenHash, label).Scan(&t.ID, &t.ProjectID, &t.Label, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ingest_token_store create: %w", err)
	}
	return t, nil
}

// ListByProject returns all ingest tokens for a project, ordered by creation time descending.
func (s *IngestTokenStore) ListByProject(ctx context.Context, projectID string) ([]*domain.IngestToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, label, created_at
		FROM project_ingest_tokens
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("ingest_token_store list_by_project: %w", err)
	}
	defer rows.Close()

	var out []*domain.IngestToken
	for rows.Next() {
		t := &domain.IngestToken{}
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Label, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("ingest_token_store scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes an ingest token by id, scoped to the project to prevent cross-project deletion.
func (s *IngestTokenStore) Delete(ctx context.Context, id, projectID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM project_ingest_tokens
		WHERE id = $1 AND project_id = $2
	`, id, projectID)
	if err != nil {
		return fmt.Errorf("ingest_token_store delete %s: %w", id, err)
	}
	return nil
}

// GetByHash looks up an ingest token by its SHA-256 hex hash.
// Returns an error wrapping pgx.ErrNoRows if not found.
func (s *IngestTokenStore) GetByHash(ctx context.Context, tokenHash string) (*domain.IngestToken, error) {
	t := &domain.IngestToken{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, label, created_at
		FROM project_ingest_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&t.ID, &t.ProjectID, &t.Label, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ingest_token_store get_by_hash: %w", err)
	}
	return t, nil
}
