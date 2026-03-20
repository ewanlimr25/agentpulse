package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// TopologyStore implements store.TopologyStore against Postgres.
type TopologyStore struct {
	pool *pgxpool.Pool
}

func NewTopologyStore(pool *pgxpool.Pool) *TopologyStore {
	return &TopologyStore{pool: pool}
}

func (s *TopologyStore) GetByRun(ctx context.Context, runID string) (*domain.Topology, error) {
	nodes, err := s.listNodes(ctx, runID)
	if err != nil {
		return nil, err
	}
	edges, err := s.listEdges(ctx, runID)
	if err != nil {
		return nil, err
	}
	return &domain.Topology{RunID: runID, Nodes: nodes, Edges: edges}, nil
}

func (s *TopologyStore) listNodes(ctx context.Context, runID string) ([]*domain.TopologyNode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, run_id, trace_id, span_id,
		       node_type, node_name, status,
		       start_time, end_time, cost_usd, token_count, metadata
		FROM topology_nodes
		WHERE run_id = $1
		ORDER BY start_time ASC NULLS LAST
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("topology_store nodes query: %w", err)
	}
	defer rows.Close()

	var out []*domain.TopologyNode
	for rows.Next() {
		n := &domain.TopologyNode{}
		var metaRaw []byte
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.RunID, &n.TraceID, &n.SpanID,
			&n.NodeType, &n.NodeName, &n.Status,
			&n.StartTime, &n.EndTime, &n.CostUSD, &n.TokenCount, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("topology_store node scan: %w", err)
		}
		if len(metaRaw) > 0 {
			if err := json.Unmarshal(metaRaw, &n.Metadata); err != nil {
				return nil, fmt.Errorf("topology_store node metadata: %w", err)
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *TopologyStore) listEdges(ctx context.Context, runID string) ([]*domain.TopologyEdge, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, run_id,
		       source_node_id, target_node_id, edge_type, metadata
		FROM topology_edges
		WHERE run_id = $1
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("topology_store edges query: %w", err)
	}
	defer rows.Close()

	var out []*domain.TopologyEdge
	for rows.Next() {
		e := &domain.TopologyEdge{}
		var metaRaw []byte
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.RunID,
			&e.SourceNodeID, &e.TargetNodeID, &e.EdgeType, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("topology_store edge scan: %w", err)
		}
		if len(metaRaw) > 0 {
			if err := json.Unmarshal(metaRaw, &e.Metadata); err != nil {
				return nil, fmt.Errorf("topology_store edge metadata: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
