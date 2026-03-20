package domain

import "time"

// NodeType classifies the type of node in the agent execution graph.
type NodeType string

const (
	NodeTypeAgent  NodeType = "agent"
	NodeTypeTool   NodeType = "tool"
	NodeTypeLLM    NodeType = "llm"
	NodeTypeMemory NodeType = "memory"
)

// NodeStatus reflects the execution outcome of a node.
type NodeStatus string

const (
	NodeStatusOK      NodeStatus = "ok"
	NodeStatusError   NodeStatus = "error"
	NodeStatusRunning NodeStatus = "running"
	NodeStatusUnset   NodeStatus = "unset"
)

// EdgeType classifies the relationship between two nodes.
type EdgeType string

const (
	EdgeTypeInvocation   EdgeType = "invocation"
	EdgeTypeHandoff      EdgeType = "handoff"
	EdgeTypeMemoryAccess EdgeType = "memory_access"
)

// TopologyNode is a vertex in the agent execution graph.
type TopologyNode struct {
	ID        string
	ProjectID string
	RunID     string
	TraceID   string
	SpanID    string

	NodeType NodeType
	NodeName string
	Status   NodeStatus

	StartTime  *time.Time
	EndTime    *time.Time
	CostUSD    float64
	TokenCount int

	Metadata map[string]any
}

// TopologyEdge is a directed edge between two nodes in the execution graph.
type TopologyEdge struct {
	ID        string
	ProjectID string
	RunID     string

	SourceNodeID string
	TargetNodeID string
	EdgeType     EdgeType

	Metadata map[string]any
}

// Topology is the complete directed graph for a single agent run.
type Topology struct {
	RunID string
	Nodes []*TopologyNode
	Edges []*TopologyEdge
}
