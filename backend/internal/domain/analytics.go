package domain

// ToolStats holds aggregated metrics for a single tool within a time window.
type ToolStats struct {
	ToolName     string
	CallCount    uint64
	ErrorCount   uint64
	ErrorRate    float64 // 0–100 percentage, computed server-side
	AvgLatencyMS float64
	P95LatencyMS float64
	TotalCostUSD float64
}

// AgentCostStats holds cost breakdown for a single agent within a time window.
type AgentCostStats struct {
	AgentName      string
	TotalCostUSD   float64
	CostPercent    float64 // 0–100, computed server-side (share of project total)
	CallCount      uint64
	AvgCostPerCall float64 // computed server-side
}
