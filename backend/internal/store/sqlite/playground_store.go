package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// PlaygroundStore implements store.PlaygroundStore against SQLite.
type PlaygroundStore struct {
	db *sql.DB
}

func NewPlaygroundStore(db *sql.DB) *PlaygroundStore {
	return &PlaygroundStore{db: db}
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

const sessionColumns = `id, project_id, name, source_span_id, source_run_id, created_at, updated_at`

func scanSession(row interface {
	Scan(...any) error
}) (*domain.PlaygroundSession, error) {
	s := &domain.PlaygroundSession{}
	err := row.Scan(
		&s.ID, &s.ProjectID, &s.Name,
		&s.SourceSpanID, &s.SourceRunID,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// CreateSession inserts a session and its initial variants atomically.
func (s *PlaygroundStore) CreateSession(ctx context.Context, sess *domain.PlaygroundSession) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("playground_store create_session begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if sess.ID == "" {
		sess.ID = uuid.NewString()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO prompt_playground_sessions
		  (id, project_id, name, source_span_id, source_run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.ProjectID, sess.Name, sess.SourceSpanID, sess.SourceRunID, now, now,
	); err != nil {
		return fmt.Errorf("playground_store create_session insert: %w", err)
	}
	sess.CreatedAt = now
	sess.UpdatedAt = now

	for _, v := range sess.Variants {
		v.SessionID = sess.ID
		if err := insertVariantTx(ctx, tx, v, now); err != nil {
			return fmt.Errorf("playground_store create_session variant: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("playground_store create_session commit: %w", err)
	}
	return nil
}

// GetSession returns a session with its variants populated.
func (s *PlaygroundStore) GetSession(ctx context.Context, id string) (*domain.PlaygroundSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+sessionColumns+`
		FROM prompt_playground_sessions
		WHERE id = ?`, id)
	sess, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("playground_store get_session: %w", err)
	}

	variants, err := s.ListVariantsBySession(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("playground_store get_session variants: %w", err)
	}
	sess.Variants = variants
	return sess, nil
}

// ListSessionsByProject returns sessions for a project, newest first.
func (s *PlaygroundStore) ListSessionsByProject(ctx context.Context, projectID string, limit, offset int) ([]*domain.PlaygroundSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sessionColumns+`
		FROM prompt_playground_sessions
		WHERE project_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("playground_store list_sessions: %w", err)
	}
	defer rows.Close()

	var out []*domain.PlaygroundSession
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("playground_store list_sessions scan: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// CountSessionsByProject returns the total number of sessions for a project.
func (s *PlaygroundStore) CountSessionsByProject(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM prompt_playground_sessions WHERE project_id = ?`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("playground_store count_sessions: %w", err)
	}
	return count, nil
}

// DeleteSession removes a session and cascades to variants and executions.
func (s *PlaygroundStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM prompt_playground_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("playground_store delete_session: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

const variantColumns = `id, session_id, label, model_id, system_prompt, messages, temperature, max_tokens, updated_at`

func scanVariant(row interface {
	Scan(...any) error
}) (*domain.PlaygroundVariant, error) {
	v := &domain.PlaygroundVariant{}
	var messagesJSON sql.NullString
	err := row.Scan(
		&v.ID, &v.SessionID, &v.Label, &v.ModelID,
		&v.System, &messagesJSON,
		&v.Temperature, &v.MaxTokens,
		&v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if messagesJSON.Valid && messagesJSON.String != "" {
		if err := json.Unmarshal([]byte(messagesJSON.String), &v.Messages); err != nil {
			return nil, fmt.Errorf("playground_store scan_variant unmarshal: %w", err)
		}
	}
	return v, nil
}

// insertVariantTx inserts a variant within an existing transaction.
func insertVariantTx(ctx context.Context, tx *sql.Tx, v *domain.PlaygroundVariant, now time.Time) error {
	messagesJSON, err := json.Marshal(v.Messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}
	if v.ID == "" {
		v.ID = uuid.NewString()
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO prompt_playground_variants
		  (id, session_id, label, model_id, system_prompt, messages, temperature, max_tokens, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.SessionID, v.Label, v.ModelID, v.System, string(messagesJSON), v.Temperature, v.MaxTokens, now,
	)
	if err != nil {
		return err
	}
	v.UpdatedAt = now
	return nil
}

// UpsertVariant creates or updates a variant.
func (s *PlaygroundStore) UpsertVariant(ctx context.Context, v *domain.PlaygroundVariant) error {
	now := time.Now().UTC()
	messagesJSON, err := json.Marshal(v.Messages)
	if err != nil {
		return fmt.Errorf("playground_store upsert_variant marshal: %w", err)
	}

	// Mirror Postgres' COALESCE($1::uuid, gen_random_uuid()) — generate an
	// ID in Go when the caller did not supply one.
	if v.ID == "" {
		v.ID = uuid.NewString()
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO prompt_playground_variants
		  (id, session_id, label, model_id, system_prompt, messages, temperature, max_tokens, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			label         = excluded.label,
			model_id      = excluded.model_id,
			system_prompt = excluded.system_prompt,
			messages      = excluded.messages,
			temperature   = excluded.temperature,
			max_tokens    = excluded.max_tokens,
			updated_at    = excluded.updated_at`,
		v.ID, v.SessionID, v.Label, v.ModelID, v.System, string(messagesJSON), v.Temperature, v.MaxTokens, now,
	)
	if err != nil {
		return fmt.Errorf("playground_store upsert_variant: %w", err)
	}
	v.UpdatedAt = now
	return nil
}

// ListVariantsBySession returns all variants for a session.
func (s *PlaygroundStore) ListVariantsBySession(ctx context.Context, sessionID string) ([]*domain.PlaygroundVariant, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+variantColumns+`
		FROM prompt_playground_variants
		WHERE session_id = ?
		ORDER BY updated_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("playground_store list_variants: %w", err)
	}
	defer rows.Close()

	var out []*domain.PlaygroundVariant
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, fmt.Errorf("playground_store list_variants scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Executions
// ---------------------------------------------------------------------------

const executionColumns = `id, variant_id, output, input_tokens, output_tokens, cost_usd, latency_ms, error, created_at`

func scanExecution(row interface {
	Scan(...any) error
}) (*domain.PlaygroundExecution, error) {
	e := &domain.PlaygroundExecution{}
	err := row.Scan(
		&e.ID, &e.VariantID, &e.Output,
		&e.InputTokens, &e.OutputTokens,
		&e.CostUSD, &e.LatencyMS,
		&e.Error, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// RecordExecution inserts an execution record for a variant.
func (s *PlaygroundStore) RecordExecution(ctx context.Context, e *domain.PlaygroundExecution) error {
	now := time.Now().UTC()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO prompt_playground_executions
		  (id, variant_id, output, input_tokens, output_tokens, cost_usd, latency_ms, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.VariantID, e.Output, e.InputTokens, e.OutputTokens, e.CostUSD, e.LatencyMS, e.Error, now,
	)
	if err != nil {
		return fmt.Errorf("playground_store record_execution: %w", err)
	}
	e.CreatedAt = now
	return nil
}

// ListExecutionsByVariant returns the most recent executions for a variant.
func (s *PlaygroundStore) ListExecutionsByVariant(ctx context.Context, variantID string, limit int) ([]*domain.PlaygroundExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+executionColumns+`
		FROM prompt_playground_executions
		WHERE variant_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, variantID, limit)
	if err != nil {
		return nil, fmt.Errorf("playground_store list_executions: %w", err)
	}
	defer rows.Close()

	var out []*domain.PlaygroundExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("playground_store list_executions scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
