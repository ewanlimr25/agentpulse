package loopdetect

import "github.com/agentpulse/agentpulse/backend/internal/domain"

// CycleResult holds the node IDs that form a detected cycle.
type CycleResult struct {
	NodeIDs []string
}

// DetectCycles runs DFS cycle detection on a topology graph.
// Returns one CycleResult per detected back-edge (cycle).
func DetectCycles(topo *domain.Topology) []CycleResult {
	if topo == nil {
		return nil
	}

	// Build adjacency list
	adj := make(map[string][]string)
	for _, e := range topo.Edges {
		adj[e.SourceNodeID] = append(adj[e.SourceNodeID], e.TargetNodeID)
	}

	// 3-colour DFS: 0=white, 1=gray, 2=black
	color := make(map[string]int)
	parent := make(map[string]string)
	var cycles []CycleResult

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = 1 // gray
		for _, next := range adj[node] {
			if color[next] == 1 {
				// Back edge found — reconstruct cycle path
				cycle := []string{next, node}
				cur := node
				for parent[cur] != "" && parent[cur] != next {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				cycles = append(cycles, CycleResult{NodeIDs: cycle})
			} else if color[next] == 0 {
				parent[next] = node
				dfs(next)
			}
		}
		color[node] = 2 // black
	}

	for _, node := range topo.Nodes {
		if color[node.ID] == 0 {
			dfs(node.ID)
		}
	}
	return cycles
}
