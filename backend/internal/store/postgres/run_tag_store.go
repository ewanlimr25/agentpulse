package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunTagStore implements store.RunTagStore against Postgres.
type RunTagStore struct {
	pool *pgxpool.Pool
}

func NewRunTagStore(pool *pgxpool.Pool) *RunTagStore {
	return &RunTagStore{pool: pool}
}

const maxListRunsResult = 500

// List returns all tag strings for a specific run, ordered alphabetically.
func (s *RunTagStore) List(ctx context.Context, projectID, runID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tag FROM run_tags
		WHERE project_id = $1 AND run_id = $2
		ORDER BY tag ASC
	`, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("run_tag_store list: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("run_tag_store list scan: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// ListByRuns returns a map of runID → []string for multiple runs in one query.
// Runs that have no tags will not appear in the returned map.
func (s *RunTagStore) ListByRuns(ctx context.Context, projectID string, runIDs []string) (map[string][]string, error) {
	if len(runIDs) == 0 {
		return map[string][]string{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT run_id, tag FROM run_tags
		WHERE project_id = $1 AND run_id = ANY($2::text[])
		ORDER BY run_id, tag ASC
	`, projectID, runIDs)
	if err != nil {
		return nil, fmt.Errorf("run_tag_store list_by_runs: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var runID, tag string
		if err := rows.Scan(&runID, &tag); err != nil {
			return nil, fmt.Errorf("run_tag_store list_by_runs scan: %w", err)
		}
		result[runID] = append(result[runID], tag)
	}
	return result, rows.Err()
}

// Add attaches a tag to a run. Silently ignores duplicate (project_id, run_id, tag) combinations.
func (s *RunTagStore) Add(ctx context.Context, projectID, runID, tag string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO run_tags (project_id, run_id, tag)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, run_id, tag) DO NOTHING
	`, projectID, runID, tag)
	if err != nil {
		return fmt.Errorf("run_tag_store add: %w", err)
	}
	return nil
}

// Delete removes a tag from a run. No-op if the tag does not exist.
func (s *RunTagStore) Delete(ctx context.Context, projectID, runID, tag string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM run_tags WHERE project_id = $1 AND run_id = $2 AND tag = $3
	`, projectID, runID, tag)
	if err != nil {
		return fmt.Errorf("run_tag_store delete: %w", err)
	}
	return nil
}

// ListRuns returns up to limit run IDs (paginated by offset) that carry the given tag.
// Results are capped at maxListRunsResult (500) regardless of the limit parameter.
func (s *RunTagStore) ListRuns(ctx context.Context, projectID, tag string, limit, offset int) ([]string, error) {
	if limit <= 0 || limit > maxListRunsResult {
		limit = maxListRunsResult
	}
	rows, err := s.pool.Query(ctx, `
		SELECT run_id FROM run_tags
		WHERE project_id = $1 AND tag = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, projectID, tag, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("run_tag_store list_runs: %w", err)
	}
	defer rows.Close()

	var runIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("run_tag_store list_runs scan: %w", err)
		}
		runIDs = append(runIDs, id)
	}
	return runIDs, rows.Err()
}

// ListAllTags returns all distinct tags used within a project, ordered alphabetically.
func (s *RunTagStore) ListAllTags(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT tag FROM run_tags
		WHERE project_id = $1
		ORDER BY tag ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("run_tag_store list_all_tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("run_tag_store list_all_tags scan: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}
