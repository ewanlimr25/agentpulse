package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ProjectPIIConfigStore implements store.ProjectPIIConfigStore against SQLite.
type ProjectPIIConfigStore struct {
	db *sql.DB
}

func NewProjectPIIConfigStore(db *sql.DB) *ProjectPIIConfigStore {
	return &ProjectPIIConfigStore{db: db}
}

func (s *ProjectPIIConfigStore) Get(ctx context.Context, projectID string) (*domain.ProjectPIIConfig, error) {
	var rulesRaw sql.NullString
	cfg := &domain.ProjectPIIConfig{
		ProjectID:      projectID,
		PIICustomRules: []domain.PIICustomRule{},
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT pii_redaction_enabled, pii_custom_rules, created_at, updated_at
		FROM project_pii_configs
		WHERE project_id = ?`, projectID).Scan(
		&cfg.PIIRedactionEnabled, &rulesRaw, &cfg.CreatedAt, &cfg.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cfg, nil
		}
		return nil, fmt.Errorf("pii_config_store get %s: %w", projectID, err)
	}

	if rulesRaw.Valid && rulesRaw.String != "" && rulesRaw.String != "[]" && rulesRaw.String != "null" {
		if err := json.Unmarshal([]byte(rulesRaw.String), &cfg.PIICustomRules); err != nil {
			return nil, fmt.Errorf("pii_config_store get unmarshal rules: %w", err)
		}
	}
	return cfg, nil
}

func (s *ProjectPIIConfigStore) Upsert(ctx context.Context, cfg *domain.ProjectPIIConfig) error {
	rulesJSON, err := json.Marshal(cfg.PIICustomRules)
	if err != nil {
		return fmt.Errorf("pii_config_store upsert marshal: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO project_pii_configs (project_id, pii_redaction_enabled, pii_custom_rules, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
		  pii_redaction_enabled = excluded.pii_redaction_enabled,
		  pii_custom_rules      = excluded.pii_custom_rules,
		  updated_at            = excluded.updated_at
	`, cfg.ProjectID, cfg.PIIRedactionEnabled, string(rulesJSON), now, now)
	if err != nil {
		return fmt.Errorf("pii_config_store upsert: %w", err)
	}
	return nil
}
