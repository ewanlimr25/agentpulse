package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ProjectPIIConfigStore implements store.ProjectPIIConfigStore against Postgres.
type ProjectPIIConfigStore struct {
	pool *pgxpool.Pool
}

func NewProjectPIIConfigStore(pool *pgxpool.Pool) *ProjectPIIConfigStore {
	return &ProjectPIIConfigStore{pool: pool}
}

// Get returns the PII config for a project.
// If no row exists, a default struct is returned — this is not an error.
func (s *ProjectPIIConfigStore) Get(ctx context.Context, projectID string) (*domain.ProjectPIIConfig, error) {
	var rulesJSON []byte
	cfg := &domain.ProjectPIIConfig{
		ProjectID:      projectID,
		PIICustomRules: []domain.PIICustomRule{},
	}

	err := s.pool.QueryRow(ctx, `
		SELECT pii_redaction_enabled, pii_custom_rules, created_at, updated_at
		FROM project_pii_configs
		WHERE project_id = $1
	`, projectID).Scan(&cfg.PIIRedactionEnabled, &rulesJSON, &cfg.CreatedAt, &cfg.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No config yet — return safe defaults, not an error.
			return cfg, nil
		}
		return nil, fmt.Errorf("pii_config_store get %s: %w", projectID, err)
	}

	if len(rulesJSON) > 0 && string(rulesJSON) != "[]" && string(rulesJSON) != "null" {
		if err := json.Unmarshal(rulesJSON, &cfg.PIICustomRules); err != nil {
			return nil, fmt.Errorf("pii_config_store get unmarshal rules: %w", err)
		}
	}
	return cfg, nil
}

// Upsert creates or updates the PII config for a project.
func (s *ProjectPIIConfigStore) Upsert(ctx context.Context, cfg *domain.ProjectPIIConfig) error {
	rulesJSON, err := json.Marshal(cfg.PIICustomRules)
	if err != nil {
		return fmt.Errorf("pii_config_store upsert marshal: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO project_pii_configs (project_id, pii_redaction_enabled, pii_custom_rules)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE SET
		  pii_redaction_enabled = EXCLUDED.pii_redaction_enabled,
		  pii_custom_rules      = EXCLUDED.pii_custom_rules,
		  updated_at            = NOW()
	`, cfg.ProjectID, cfg.PIIRedactionEnabled, rulesJSON)
	if err != nil {
		return fmt.Errorf("pii_config_store upsert: %w", err)
	}
	return nil
}
