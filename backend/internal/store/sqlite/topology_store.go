package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// TopologyStore implements store.TopologyStore against SQLite.
type TopologyStore struct {
	db *sql.DB
}

func NewTopologyStore(db *sql.DB) *TopologyStore {
	return &TopologyStore{db: db}
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, run_id, trace_id, span_id,
		       node_type, node_name, status,
		       start_time, end_time, cost_usd, token_count, metadata
		FROM topology_nodes
		WHERE run_id = ?
		ORDER BY start_time ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("topology_store nodes query: %w", err)
	}
	defer rows.Close()

	var out []*domain.TopologyNode
	for rows.Next() {
		n := &domain.TopologyNode{}
		var (
			startTime sql.NullTime
			endTime   sql.NullTime
			metaRaw   sql.NullString
			nodeType  string
			status    string
		)
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.RunID, &n.TraceID, &n.SpanID,
			&nodeType, &n.NodeName, &status,
			&startTime, &endTime, &n.CostUSD, &n.TokenCount, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("topology_store node scan: %w", err)
		}
		n.NodeType = domain.NodeType(nodeType)
		n.Status = domain.NodeStatus(status)
		if startTime.Valid {
			t := startTime.Time
			n.StartTime = &t
		}
		if endTime.Valid {
			t := endTime.Time
			n.EndTime = &t
		}
		if metaRaw.Valid && metaRaw.String != "" {
			if err := json.Unmarshal([]byte(metaRaw.String), &n.Metadata); err != nil {
				return nil, fmt.Errorf("topology_store node metadata: %w", err)
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *TopologyStore) listEdges(ctx context.Context, runID string) ([]*domain.TopologyEdge, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, run_id,
		       source_node_id, target_node_id, edge_type, metadata
		FROM topology_edges
		WHERE run_id = ?
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("topology_store edges query: %w", err)
	}
	defer rows.Close()

	var out []*domain.TopologyEdge
	for rows.Next() {
		e := &domain.TopologyEdge{}
		var (
			edgeType string
			metaRaw  sql.NullString
		)
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.RunID,
			&e.SourceNodeID, &e.TargetNodeID, &edgeType, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("topology_store edge scan: %w", err)
		}
		e.EdgeType = domain.EdgeType(edgeType)
		if metaRaw.Valid && metaRaw.String != "" {
			if err := json.Unmarshal([]byte(metaRaw.String), &e.Metadata); err != nil {
				return nil, fmt.Errorf("topology_store edge metadata: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
