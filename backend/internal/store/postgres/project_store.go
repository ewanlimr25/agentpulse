package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ProjectStore implements store.ProjectStore against Postgres.
type ProjectStore struct {
	pool *pgxpool.Pool
}

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore {
	return &ProjectStore{pool: pool}
}

func (s *ProjectStore) List(ctx context.Context) ([]*domain.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, api_key_hash, created_at, updated_at
		FROM projects ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("project_store list: %w", err)
	}
	defer rows.Close()

	var out []*domain.Project
	for rows.Next() {
		p := &domain.Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.APIKeyHash, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("project_store scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Get(ctx context.Context, id string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, api_key_hash, created_at, updated_at
		FROM projects WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.APIKeyHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project_store get %s: %w", id, err)
	}
	return p, nil
}

func (s *ProjectStore) Create(ctx context.Context, p *domain.Project) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO projects (id, name, api_key_hash)
		VALUES ($1, $2, $3)
	`, p.ID, p.Name, p.APIKeyHash)
	if err != nil {
		return fmt.Errorf("project_store create: %w", err)
	}
	return nil
}

func (s *ProjectStore) GetByAPIKeyHash(ctx context.Context, hash string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, api_key_hash, created_at, updated_at
		FROM projects WHERE api_key_hash = $1
	`, hash).Scan(&p.ID, &p.Name, &p.APIKeyHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project_store get_by_key_hash: %w", err)
	}
	return p, nil
}
