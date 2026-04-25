package domain

// LoopConfig holds per-project loop-detection thresholds.
// Nil fields fall back to the global defaults.
type LoopConfig struct {
	Tier1MinCount      int `json:"tier1_min_count"`      // default 2
	Tier2MinCount      int `json:"tier2_min_count"`      // default 4
	Tier2MaxIntervalMs int `json:"tier2_max_interval_ms"` // default 3000
}

// DefaultLoopConfig is the global fallback used when a project has no custom config.
var DefaultLoopConfig = LoopConfig{
	Tier1MinCount:      2,
	Tier2MinCount:      4,
	Tier2MaxIntervalMs: 3000,
}
