package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EvalConfigStore implements store.EvalConfigStore against Postgres.
type EvalConfigStore struct {
	pool *pgxpool.Pool
}

func NewEvalConfigStore(pool *pgxpool.Pool) *EvalConfigStore {
	return &EvalConfigStore{pool: pool}
}

const evalConfigColumns = `id, project_id, eval_name, enabled, span_kind,
	prompt_template, prompt_version, scope_filter, created_at, updated_at`

func scanEvalConfig(row interface {
	Scan(...any) error
}) (*domain.EvalConfig, error) {
	c := &domain.EvalConfig{}
	var createdAt, updatedAt time.Time
	var scopeFilterJSON []byte
	err := row.Scan(
		&c.ID, &c.ProjectID, &c.EvalName, &c.Enabled, &c.SpanKind,
		&c.PromptTemplate, &c.PromptVersion, &scopeFilterJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = createdAt.UTC()
	c.UpdatedAt = updatedAt.UTC()
	if len(scopeFilterJSON) > 0 && string(scopeFilterJSON) != "{}" {
		_ = json.Unmarshal(scopeFilterJSON, &c.ScopeFilter)
	}
	return c, nil
}

func (s *EvalConfigStore) List(ctx context.Context, projectID string) ([]*domain.EvalConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+evalConfigColumns+`
		FROM project_eval_configs
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("eval_config_store list: %w", err)
	}
	defer rows.Close()

	var out []*domain.EvalConfig
	for rows.Next() {
		c, err := scanEvalConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("eval_config_store list scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *EvalConfigStore) ListAllEnabled(ctx context.Context) ([]*domain.EvalConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+evalConfigColumns+`
		FROM project_eval_configs
		WHERE enabled = TRUE
		ORDER BY project_id, eval_name
	`)
	if err != nil {
		return nil, fmt.Errorf("eval_config_store list_all_enabled: %w", err)
	}
	defer rows.Close()

	var out []*domain.EvalConfig
	for rows.Next() {
		c, err := scanEvalConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("eval_config_store list_all_enabled scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *EvalConfigStore) Upsert(ctx context.Context, cfg *domain.EvalConfig) error {
	scopeFilterJSON, err := json.Marshal(cfg.ScopeFilter)
	if err != nil || cfg.ScopeFilter == nil {
		scopeFilterJSON = []byte("{}")
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO project_eval_configs
		  (id, project_id, eval_name, enabled, span_kind, prompt_template, prompt_version, scope_filter)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (project_id, eval_name) DO UPDATE SET
		  enabled         = EXCLUDED.enabled,
		  span_kind       = EXCLUDED.span_kind,
		  prompt_template = EXCLUDED.prompt_template,
		  prompt_version  = CASE
		    WHEN project_eval_configs.prompt_template IS DISTINCT FROM EXCLUDED.prompt_template
		    THEN project_eval_configs.prompt_version + 1
		    ELSE project_eval_configs.prompt_version
		  END,
		  scope_filter    = EXCLUDED.scope_filter,
		  updated_at      = now()
	`, cfg.ID, cfg.ProjectID, cfg.EvalName, cfg.Enabled, cfg.SpanKind,
		cfg.PromptTemplate, cfg.PromptVersion, scopeFilterJSON)
	if err != nil {
		return fmt.Errorf("eval_config_store upsert: %w", err)
	}
	return nil
}

func (s *EvalConfigStore) Delete(ctx context.Context, projectID, evalName string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM project_eval_configs
		WHERE project_id = $1 AND eval_name = $2
	`, projectID, evalName)
	if err != nil {
		return fmt.Errorf("eval_config_store delete: %w", err)
	}
	return nil
}
