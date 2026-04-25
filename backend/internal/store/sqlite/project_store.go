package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ProjectStore implements store.ProjectStore against SQLite.
type ProjectStore struct {
	db *sql.DB
}

func NewProjectStore(db *sql.DB) *ProjectStore { return &ProjectStore{db: db} }

func (s *ProjectStore) List(ctx context.Context) ([]*domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, api_key_hash, admin_key_hash, created_at, updated_at
		FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("project_store list: %w", err)
	}
	defer rows.Close()

	var out []*domain.Project
	for rows.Next() {
		p := &domain.Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.APIKeyHash, &p.AdminKeyHash, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("project_store scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Get(ctx context.Context, id string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, api_key_hash, admin_key_hash, created_at, updated_at
		FROM projects WHERE id = ?`, id).Scan(
		&p.ID, &p.Name, &p.APIKeyHash, &p.AdminKeyHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project_store get %s: %w", id, err)
	}
	return p, nil
}

// Create inserts the project. If p.ID is empty a new UUID is generated.
func (s *ProjectStore) Create(ctx context.Context, p *domain.Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, api_key_hash, admin_key_hash)
		VALUES (?, ?, ?, ?)`, p.ID, p.Name, p.APIKeyHash, p.AdminKeyHash)
	if err != nil {
		return fmt.Errorf("project_store create: %w", err)
	}
	return nil
}

func (s *ProjectStore) GetByAPIKeyHash(ctx context.Context, hash string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, api_key_hash, admin_key_hash, created_at, updated_at
		FROM projects WHERE api_key_hash = ?`, hash).Scan(
		&p.ID, &p.Name, &p.APIKeyHash, &p.AdminKeyHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project_store get_by_key_hash: %w", err)
	}
	return p, nil
}

func (s *ProjectStore) GetByAdminKeyHash(ctx context.Context, hash string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, api_key_hash, admin_key_hash, created_at, updated_at
		FROM projects WHERE admin_key_hash = ?`, hash).Scan(
		&p.ID, &p.Name, &p.APIKeyHash, &p.AdminKeyHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project_store get_by_admin_key_hash: %w", err)
	}
	return p, nil
}

// GetLoopConfig returns the JSON-decoded loop_config column, or DefaultLoopConfig
// if the column is NULL (no custom config has been set).
func (s *ProjectStore) GetLoopConfig(ctx context.Context, projectID string) (*domain.LoopConfig, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT loop_config FROM projects WHERE id = ?`, projectID).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			def := domain.DefaultLoopConfig
			return &def, nil
		}
		return nil, fmt.Errorf("project_store get_loop_config %s: %w", projectID, err)
	}
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		def := domain.DefaultLoopConfig
		return &def, nil
	}
	var cfg domain.LoopConfig
	if err := json.Unmarshal([]byte(raw.String), &cfg); err != nil {
		return nil, fmt.Errorf("project_store get_loop_config unmarshal %s: %w", projectID, err)
	}
	return &cfg, nil
}

// PutLoopConfig serializes cfg as JSON into the loop_config column.
func (s *ProjectStore) PutLoopConfig(ctx context.Context, projectID string, cfg domain.LoopConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("project_store put_loop_config marshal: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE projects SET loop_config = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, string(raw), projectID)
	if err != nil {
		return fmt.Errorf("project_store put_loop_config %s: %w", projectID, err)
	}
	return nil
}
