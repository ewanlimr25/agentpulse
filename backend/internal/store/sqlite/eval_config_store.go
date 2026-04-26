package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EvalConfigStore implements store.EvalConfigStore against SQLite.
type EvalConfigStore struct {
	db *sql.DB
}

func NewEvalConfigStore(db *sql.DB) *EvalConfigStore {
	return &EvalConfigStore{db: db}
}

const evalConfigColumns = `id, project_id, eval_name, enabled, span_kind,
	prompt_template, prompt_version, scope_filter, judge_models, created_at, updated_at`

const defaultJudgeModel = "claude-haiku-4-5"

func scanEvalConfig(row interface {
	Scan(...any) error
}) (*domain.EvalConfig, error) {
	c := &domain.EvalConfig{}
	var createdAt, updatedAt time.Time
	var scopeFilterJSON, judgeModelsJSON sql.NullString
	err := row.Scan(
		&c.ID, &c.ProjectID, &c.EvalName, &c.Enabled, &c.SpanKind,
		&c.PromptTemplate, &c.PromptVersion, &scopeFilterJSON, &judgeModelsJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = createdAt.UTC()
	c.UpdatedAt = updatedAt.UTC()

	if scopeFilterJSON.Valid && scopeFilterJSON.String != "" && scopeFilterJSON.String != "{}" {
		_ = json.Unmarshal([]byte(scopeFilterJSON.String), &c.ScopeFilter)
	}

	if judgeModelsJSON.Valid && judgeModelsJSON.String != "" {
		var models []string
		if err := json.Unmarshal([]byte(judgeModelsJSON.String), &models); err != nil {
			return nil, fmt.Errorf("eval_config_store unmarshal judge_models: %w", err)
		}
		if len(models) == 0 {
			c.JudgeModels = []string{defaultJudgeModel}
		} else {
			c.JudgeModels = models
		}
	} else {
		c.JudgeModels = []string{defaultJudgeModel}
	}
	return c, nil
}

func (s *EvalConfigStore) List(ctx context.Context, projectID string) ([]*domain.EvalConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+evalConfigColumns+`
		FROM project_eval_configs
		WHERE project_id = ?
		ORDER BY created_at ASC`, projectID)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+evalConfigColumns+`
		FROM project_eval_configs
		WHERE enabled = 1
		ORDER BY project_id, eval_name`)
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

// Upsert creates or updates a config keyed on (project_id, eval_name).
//
// SQLite UPSERT preserves the prompt_version and only bumps it when the
// prompt_template actually changes — matching the Postgres CASE expression.
func (s *EvalConfigStore) Upsert(ctx context.Context, cfg *domain.EvalConfig) error {
	scopeFilterJSON, err := json.Marshal(cfg.ScopeFilter)
	if err != nil || cfg.ScopeFilter == nil {
		scopeFilterJSON = []byte("{}")
	}

	judgeModels := cfg.JudgeModels
	if len(judgeModels) == 0 {
		judgeModels = []string{defaultJudgeModel}
	}
	judgeModelsJSON, err := json.Marshal(judgeModels)
	if err != nil {
		return fmt.Errorf("eval_config_store upsert marshal judge_models: %w", err)
	}

	id := cfg.ID
	if id == "" {
		id = uuid.NewString()
		cfg.ID = id
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO project_eval_configs
		  (id, project_id, eval_name, enabled, span_kind, prompt_template, prompt_version, scope_filter, judge_models)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, eval_name) DO UPDATE SET
		  enabled         = excluded.enabled,
		  span_kind       = excluded.span_kind,
		  prompt_template = excluded.prompt_template,
		  prompt_version  = CASE
		    WHEN COALESCE(excluded.prompt_template, '') != COALESCE(project_eval_configs.prompt_template, '')
		    THEN project_eval_configs.prompt_version + 1
		    ELSE project_eval_configs.prompt_version
		  END,
		  scope_filter    = excluded.scope_filter,
		  judge_models    = excluded.judge_models,
		  updated_at      = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		id, cfg.ProjectID, cfg.EvalName, cfg.Enabled, cfg.SpanKind,
		cfg.PromptTemplate, cfg.PromptVersion, string(scopeFilterJSON), string(judgeModelsJSON),
	)
	if err != nil {
		return fmt.Errorf("eval_config_store upsert: %w", err)
	}
	return nil
}

func (s *EvalConfigStore) Delete(ctx context.Context, projectID, evalName string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM project_eval_configs
		WHERE project_id = ? AND eval_name = ?`, projectID, evalName)
	if err != nil {
		return fmt.Errorf("eval_config_store delete: %w", err)
	}
	return nil
}
