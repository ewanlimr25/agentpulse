package domain

// ReplayBundle is a self-contained snapshot of a single agent run that can be
// loaded by an SDK in "replay mode" to deterministically re-execute the run
// against mocked tool/LLM responses sourced from the recorded spans.
//
// SchemaVersion is incremented whenever the bundle format changes in a way
// that requires SDK updates to consume it.
type ReplayBundle struct {
	SchemaVersion int           `json:"schema_version"`
	Run           *Run          `json:"run"`
	Spans         []*ReplaySpan `json:"spans"`
	Topology      *Topology     `json:"topology,omitempty"`
}

// ReplaySpan is a span enriched with a CallIndex disambiguator so that the
// replay SDK can match repeated invocations of the same (agent_name, span_name)
// pair to the correct recorded response.
//
// CallIndex starts at 0 and increments for each repeated occurrence of a
// given (agent_name, span_name) within a run, walking spans in start_time
// order. The first call to tool "search" by agent "researcher" has
// CallIndex=0, the second has CallIndex=1, and so on.
type ReplaySpan struct {
	*Span
	CallIndex int `json:"call_index"`
}
