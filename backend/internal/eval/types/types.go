// Package evaltypes defines the EvalType interface and all built-in + custom
// eval type implementations. This package has no imports from the parent eval
// package to avoid circular dependencies.
package evaltypes

import "strings"

// SpanContext holds the span attributes needed by an eval type to build its prompt.
type SpanContext struct {
	Input     string // gen_ai.prompt or tool.input
	Output    string // gen_ai.completion or tool.output
	Context   string // gen_ai.context (for faithfulness)
	Model     string // gen_ai.request.model
	AgentName string // agent.name
	ToolName  string // tool.name (for tool.call spans)
}

// EvalType is the strategy interface for building judge prompts.
// Each eval type knows its name, which span kind it applies to, and how to
// build a scoring prompt from span attributes.
type EvalType interface {
	Name() string
	SpanKind() string // "llm.call" or "tool.call"
	BuildPrompt(ctx SpanContext) string
}

// Registry maps eval_name → EvalType.
type Registry map[string]EvalType

// xmlEscape replaces characters that could break XML tag boundaries.
// Applied to all user-supplied content before it enters a judge prompt.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// judgeInstruction is the standard suffix requesting JSON output.
const judgeInstruction = `
Respond with valid JSON only, no other text:
{"score": <float 0.0–1.0>, "reasoning": "<one sentence>"}`
