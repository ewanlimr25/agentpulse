package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// IngestTokenStore implements store.IngestTokenStore against SQLite.
type IngestTokenStore struct {
	db *sql.DB
}

func NewIngestTokenStore(db *sql.DB) *IngestTokenStore { return &IngestTokenStore{db: db} }

func (s *IngestTokenStore) Create(ctx context.Context, projectID, tokenHash, label string) (*domain.IngestToken, error) {
	id := uuid.NewString()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO project_ingest_tokens (id, project_id, token_hash, label)
		VALUES (?, ?, ?, ?)`, id, projectID, tokenHash, label); err != nil {
		return nil, fmt.Errorf("ingest_token_store create: %w", err)
	}
	t := &domain.IngestToken{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, label, created_at FROM project_ingest_tokens WHERE id = ?`, id).
		Scan(&t.ID, &t.ProjectID, &t.Label, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ingest_token_store readback: %w", err)
	}
	return t, nil
}

func (s *IngestTokenStore) ListByProject(ctx context.Context, projectID string) ([]*domain.IngestToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, label, created_at
		FROM project_ingest_tokens
		WHERE project_id = ?
		ORDER BY created_at DESC`, projectID)
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

func (s *IngestTokenStore) Delete(ctx context.Context, id, projectID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM project_ingest_tokens
		WHERE id = ? AND project_id = ?`, id, projectID)
	if err != nil {
		return fmt.Errorf("ingest_token_store delete %s: %w", id, err)
	}
	return nil
}

func (s *IngestTokenStore) GetByHash(ctx context.Context, tokenHash string) (*domain.IngestToken, error) {
	t := &domain.IngestToken{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, label, created_at
		FROM project_ingest_tokens
		WHERE token_hash = ?`, tokenHash).
		Scan(&t.ID, &t.ProjectID, &t.Label, &t.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("ingest_token_store get_by_hash: %w", err)
		}
		return nil, fmt.Errorf("ingest_token_store get_by_hash: %w", err)
	}
	return t, nil
}
