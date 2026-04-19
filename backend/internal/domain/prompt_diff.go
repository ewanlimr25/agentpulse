package domain

// PromptFieldDiff captures a before/after comparison of a single prompt field.
type PromptFieldDiff struct {
	FieldName string `json:"field_name"`
	A         string `json:"a"`
	B         string `json:"b"`
	Changed   bool   `json:"changed"`
}

// ModelParamDiff captures a before/after comparison of a single model parameter.
type ModelParamDiff struct {
	ParamName string `json:"param_name"`
	A         string `json:"a"`
	B         string `json:"b"`
	Changed   bool   `json:"changed"`
}

// SpanPromptDiff holds the diff for a single matched LLM span pair.
type SpanPromptDiff struct {
	SpanKey   string `json:"span_key"`
	AgentName string `json:"agent_name"`
	SpanName  string `json:"span_name"`
	CallIndex int    `json:"call_index"`
	// Status is one of: "changed", "only-a", "only-b", "unchanged"
	Status      string            `json:"status"`
	PromptDiffs []PromptFieldDiff `json:"prompt_diffs"`
	ParamDiffs  []ModelParamDiff  `json:"param_diffs"`
}

// RunPromptDiff is the top-level response for a prompt diff between two runs.
type RunPromptDiff struct {
	RunIDA         string           `json:"run_id_a"`
	RunIDB         string           `json:"run_id_b"`
	Spans          []SpanPromptDiff `json:"spans"`
	UnchangedCount int              `json:"unchanged_count"`
	Truncated      bool             `json:"truncated"`
}
