package topologyexporter

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// topologyStore abstracts Postgres writes for testability.
type topologyStore interface {
	// UpsertNodes inserts or updates topology nodes and returns a map of
	// spanID → postgres UUID for use when creating edges.
	UpsertNodes(ctx context.Context, projectID string, nodes []topologyNode) (map[string]string, error)

	// UpsertEdges inserts topology edges using the resolved node UUIDs.
	// nodeIDs maps spanID → postgres UUID (returned from UpsertNodes).
	UpsertEdges(ctx context.Context, projectID string, edges []topologyEdge, nodeIDs map[string]string) error

	Close()
}

// postgresStore is the production Postgres-backed topology store.
type postgresStore struct {
	pool *pgxpool.Pool
}

func newPostgresStore(ctx context.Context, dsn string) (*postgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return &postgresStore{pool: pool}, nil
}

func (s *postgresStore) Close() { s.pool.Close() }

// UpsertNodes batch-upserts topology nodes and returns spanID → nodeID mapping.
func (s *postgresStore) UpsertNodes(ctx context.Context, projectID string, nodes []topologyNode) (map[string]string, error) {
	if len(nodes) == 0 {
		return map[string]string{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		INSERT INTO topology_nodes
			(project_id, run_id, trace_id, span_id, node_type, node_name, status,
			 start_time, end_time, cost_usd, token_count, metadata)
		SELECT
			$1::uuid, v.run_id, ''::text, v.span_id, v.node_type, v.node_name, v.status,
			v.start_time, v.end_time, v.cost_usd, v.token_count, v.metadata::jsonb
		FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[],
		            $7::timestamptz[], $8::timestamptz[], $9::float8[], $10::int4[], $11::text[])
		AS v(run_id, span_id, node_type, node_name, status,
		     start_time, end_time, cost_usd, token_count, metadata)
		ON CONFLICT (project_id, run_id, span_id)
		DO UPDATE SET
			node_type  = EXCLUDED.node_type,
			node_name  = EXCLUDED.node_name,
			status     = EXCLUDED.status,
			end_time   = EXCLUDED.end_time,
			cost_usd   = EXCLUDED.cost_usd,
			token_count = EXCLUDED.token_count
		RETURNING id::text, span_id`,
		projectID,
		nodeField(nodes, func(n topologyNode) string { return n.RunID }),
		nodeField(nodes, func(n topologyNode) string { return n.SpanID }),
		nodeField(nodes, func(n topologyNode) string { return n.NodeType }),
		nodeField(nodes, func(n topologyNode) string { return n.NodeName }),
		nodeField(nodes, func(n topologyNode) string { return n.Status }),
		nodeTimeField(nodes, func(n topologyNode) *string { return timePtr(n.StartTime) }),
		nodeTimeField(nodes, func(n topologyNode) *string { return timePtr(n.EndTime) }),
		nodeFloat(nodes, func(n topologyNode) float64 { return n.CostUSD }),
		nodeInt(nodes, func(n topologyNode) int { return n.TokenCount }),
		nodeMetadata(nodes),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting topology nodes: %w", err)
	}
	defer rows.Close()

	nodeIDs := make(map[string]string, len(nodes))
	for rows.Next() {
		var nodeID, spanID string
		if err := rows.Scan(&nodeID, &spanID); err != nil {
			return nil, fmt.Errorf("scanning node id: %w", err)
		}
		nodeIDs[spanID] = nodeID
	}
	return nodeIDs, rows.Err()
}

// UpsertEdges batch-inserts topology edges, resolving span IDs to node UUIDs.
func (s *postgresStore) UpsertEdges(ctx context.Context, projectID string, edges []topologyEdge, nodeIDs map[string]string) error {
	if len(edges) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range edges {
		srcID, srcOK := nodeIDs[e.SourceSpanID]
		dstID, dstOK := nodeIDs[e.TargetSpanID]
		if !srcOK || !dstOK {
			continue // node wasn't upserted (should not happen, but defensive)
		}
		batch.Queue(`
			INSERT INTO topology_edges
				(project_id, run_id, source_node_id, target_node_id, edge_type)
			VALUES ($1::uuid, $2, $3::uuid, $4::uuid, $5)
			ON CONFLICT (project_id, run_id, source_node_id, target_node_id, edge_type)
			DO NOTHING`,
			projectID, e.RunID, srcID, dstID, e.EdgeType,
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	for range edges {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting topology edge: %w", err)
		}
	}
	return nil
}

// ── Field extraction helpers ──────────────────────────────────────────────────

func nodeField(nodes []topologyNode, f func(topologyNode) string) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = f(n)
	}
	return out
}

func nodeTimeField(nodes []topologyNode, f func(topologyNode) *string) []*string {
	out := make([]*string, len(nodes))
	for i, n := range nodes {
		out[i] = f(n)
	}
	return out
}

func nodeFloat(nodes []topologyNode, f func(topologyNode) float64) []float64 {
	out := make([]float64, len(nodes))
	for i, n := range nodes {
		out[i] = f(n)
	}
	return out
}

func nodeInt(nodes []topologyNode, f func(topologyNode) int) []int32 {
	out := make([]int32, len(nodes))
	for i, n := range nodes {
		out[i] = int32(f(n))
	}
	return out
}

func nodeMetadata(nodes []topologyNode) []string {
	out := make([]string, len(nodes))
	for i := range nodes {
		out[i] = "{}"
	}
	return out
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02T15:04:05.999999999Z")
	return &s
}
