package topologyexporter

import "time"

// enrichedSpan is the subset of span data needed for topology inference.
// All agentpulse.* fields are written by agentsemanticproc upstream.
type enrichedSpan struct {
	TraceID      string
	SpanID       string
	ParentSpanID string

	RunID     string
	ProjectID string
	SpanName  string

	AgentSpanKind string
	AgentName     string
	ModelID       string
	ToolName      string
	MCPServerName string

	CostUSD     float64
	InputTokens uint32
	OutputTokens uint32
	StatusCode  string

	StartTime time.Time
	EndTime   time.Time
}

// topologyNode is a vertex in the agent execution graph.
type topologyNode struct {
	SpanID    string
	RunID     string
	ProjectID string

	NodeType string // agent | tool | llm | memory
	NodeName string
	Status   string // ok | error | running | unset

	StartTime  *time.Time
	EndTime    *time.Time
	CostUSD    float64
	TokenCount int

	Metadata map[string]any
}

// topologyEdge is a directed edge between two nodes.
type topologyEdge struct {
	RunID     string
	ProjectID string

	SourceSpanID string // resolved to a node ID by the store
	TargetSpanID string

	EdgeType string // invocation | handoff | memory_access
}

const (
	nodeTypeAgent  = "agent"
	nodeTypeTool   = "tool"
	nodeTypeLLM    = "llm"
	nodeTypeMemory = "memory"
	nodeTypeMCP    = "mcp"

	edgeTypeInvocation   = "invocation"
	edgeTypeHandoff      = "handoff"
	edgeTypeMemoryAccess = "memory_access"

	statusOK    = "ok"
	statusError = "error"
	statusUnset = "unset"
)

// InferTopology converts a batch of enriched spans into topology nodes and edges.
//
// Rules:
//   - Every span with a recognised kind (or with an agent name) becomes a node.
//   - If a span's parent_span_id exists in the batch, an edge is created from
//     the parent node to the child node.
//   - Orphan spans (parent not in batch) become nodes without incoming edges.
//   - Unknown spans with no agent name are skipped.
func InferTopology(spans []enrichedSpan) ([]topologyNode, []topologyEdge) {
	if len(spans) == 0 {
		return nil, nil
	}

	// Build a span index for O(1) parent lookups.
	index := make(map[string]*enrichedSpan, len(spans))
	for i := range spans {
		index[spans[i].SpanID] = &spans[i]
	}

	// Track which spans become nodes (skip unknown spans with no agent name).
	nodeSpanIDs := make(map[string]bool, len(spans))
	nodes := make([]topologyNode, 0, len(spans))

	for i := range spans {
		s := &spans[i]
		node, ok := spanToNode(s)
		if !ok {
			continue
		}
		nodes = append(nodes, node)
		nodeSpanIDs[s.SpanID] = true
	}

	// Build edges: walk each span that became a node and link to its parent.
	edges := make([]topologyEdge, 0)
	for i := range spans {
		s := &spans[i]
		if !nodeSpanIDs[s.SpanID] {
			continue // this span was skipped
		}
		if s.ParentSpanID == "" {
			continue // root span — no incoming edge
		}
		if _, parentExists := index[s.ParentSpanID]; !parentExists {
			continue // orphan — parent not in batch
		}
		if !nodeSpanIDs[s.ParentSpanID] {
			continue // parent span was skipped (unknown kind, no agent name)
		}

		edges = append(edges, topologyEdge{
			RunID:        s.RunID,
			ProjectID:    s.ProjectID,
			SourceSpanID: s.ParentSpanID,
			TargetSpanID: s.SpanID,
			EdgeType:     edgeTypeForKind(s.AgentSpanKind),
		})
	}

	return nodes, edges
}

// spanToNode converts a single span to a topology node.
// Returns (node, true) if the span should become a node, (zero, false) to skip.
func spanToNode(s *enrichedSpan) (topologyNode, bool) {
	nodeType, nodeName, ok := classifyNode(s)
	if !ok {
		return topologyNode{}, false
	}

	tokenCount := int(s.InputTokens) + int(s.OutputTokens)

	meta := map[string]any{}
	if s.ModelID != "" {
		meta["model_id"] = s.ModelID
	}
	if s.AgentName != "" {
		meta["agent_name"] = s.AgentName
	}
	if s.MCPServerName != "" {
		meta["mcp_server_name"] = s.MCPServerName
	}

	node := topologyNode{
		SpanID:     s.SpanID,
		RunID:      s.RunID,
		ProjectID:  s.ProjectID,
		NodeType:   nodeType,
		NodeName:   nodeName,
		Status:     statusFromCode(s.StatusCode),
		CostUSD:    s.CostUSD,
		TokenCount: tokenCount,
		Metadata:   meta,
	}

	if !s.StartTime.IsZero() {
		t := s.StartTime
		node.StartTime = &t
	}
	if !s.EndTime.IsZero() {
		t := s.EndTime
		node.EndTime = &t
	}

	return node, true
}

// classifyNode determines the node type and display name for a span.
// Returns (type, name, true) on success or ("", "", false) to skip the span.
func classifyNode(s *enrichedSpan) (nodeType, nodeName string, ok bool) {
	switch s.AgentSpanKind {
	case "llm.call":
		// Prefer agent name — it answers "who is making this call?"
		// Fall back to model ID, then span name.
		name := s.AgentName
		if name == "" {
			name = s.ModelID
		}
		if name == "" {
			name = s.SpanName
		}
		return nodeTypeLLM, name, true

	case "tool.call":
		// Prefer explicit tool name; strip "tool/" prefix from span names.
		name := s.ToolName
		if name == "" {
			name = stripToolPrefix(s.SpanName)
		}
		return nodeTypeTool, name, true

	case "agent.handoff":
		name := s.AgentName
		if name == "" {
			name = s.SpanName
		}
		return nodeTypeAgent, name, true

	case "memory.read", "memory.write":
		return nodeTypeMemory, "memory", true

	case "mcp.tool_call":
		name := s.ToolName
		if name == "" {
			name = s.SpanName
		}
		return nodeTypeMCP, name, true

	case "mcp.list_tools":
		name := s.SpanName
		return nodeTypeMCP, name, true

	default:
		if s.AgentName != "" {
			return nodeTypeAgent, s.AgentName, true
		}
		return "", "", false
	}
}

// stripToolPrefix removes a leading "tool/" from span names.
func stripToolPrefix(name string) string {
	if len(name) > 5 && name[:5] == "tool/" {
		return name[5:]
	}
	return name
}

// edgeTypeForKind maps a child span's agent_span_kind to a topology edge type.
func edgeTypeForKind(kind string) string {
	switch kind {
	case "agent.handoff":
		return edgeTypeHandoff
	case "memory.read", "memory.write":
		return edgeTypeMemoryAccess
	default:
		return edgeTypeInvocation
	}
}

// statusFromCode converts an OTel status code string to a topology node status.
func statusFromCode(code string) string {
	switch code {
	case "STATUS_CODE_OK", "Ok", "OK":
		return statusOK
	case "STATUS_CODE_ERROR", "Error", "ERROR":
		return statusError
	default:
		return statusUnset
	}
}
