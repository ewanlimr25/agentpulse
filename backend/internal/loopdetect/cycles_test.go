package loopdetect

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

func TestDetectCycles(t *testing.T) {
	tests := []struct {
		name        string
		topo        *domain.Topology
		wantCycles  int
	}{
		{
			name:       "nil topology returns no cycles",
			topo:       nil,
			wantCycles: 0,
		},
		{
			name: "simple DAG A->B->C has no cycles",
			topo: &domain.Topology{
				RunID: "run-1",
				Nodes: []*domain.TopologyNode{
					{ID: "A"}, {ID: "B"}, {ID: "C"},
				},
				Edges: []*domain.TopologyEdge{
					{SourceNodeID: "A", TargetNodeID: "B"},
					{SourceNodeID: "B", TargetNodeID: "C"},
				},
			},
			wantCycles: 0,
		},
		{
			name: "self-loop A->A produces one cycle",
			topo: &domain.Topology{
				RunID: "run-2",
				Nodes: []*domain.TopologyNode{
					{ID: "A"},
				},
				Edges: []*domain.TopologyEdge{
					{SourceNodeID: "A", TargetNodeID: "A"},
				},
			},
			wantCycles: 1,
		},
		{
			name: "simple cycle A->B->A produces one cycle",
			topo: &domain.Topology{
				RunID: "run-3",
				Nodes: []*domain.TopologyNode{
					{ID: "A"}, {ID: "B"},
				},
				Edges: []*domain.TopologyEdge{
					{SourceNodeID: "A", TargetNodeID: "B"},
					{SourceNodeID: "B", TargetNodeID: "A"},
				},
			},
			wantCycles: 1,
		},
		{
			name: "diamond DAG A->B, A->C, B->D, C->D has no cycles",
			topo: &domain.Topology{
				RunID: "run-4",
				Nodes: []*domain.TopologyNode{
					{ID: "A"}, {ID: "B"}, {ID: "C"}, {ID: "D"},
				},
				Edges: []*domain.TopologyEdge{
					{SourceNodeID: "A", TargetNodeID: "B"},
					{SourceNodeID: "A", TargetNodeID: "C"},
					{SourceNodeID: "B", TargetNodeID: "D"},
					{SourceNodeID: "C", TargetNodeID: "D"},
				},
			},
			wantCycles: 0,
		},
		{
			name: "complex cycle A->B->C->A produces one cycle",
			topo: &domain.Topology{
				RunID: "run-5",
				Nodes: []*domain.TopologyNode{
					{ID: "A"}, {ID: "B"}, {ID: "C"},
				},
				Edges: []*domain.TopologyEdge{
					{SourceNodeID: "A", TargetNodeID: "B"},
					{SourceNodeID: "B", TargetNodeID: "C"},
					{SourceNodeID: "C", TargetNodeID: "A"},
				},
			},
			wantCycles: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectCycles(tt.topo)
			if len(got) != tt.wantCycles {
				t.Errorf("DetectCycles() returned %d cycles, want %d", len(got), tt.wantCycles)
			}
		})
	}
}
