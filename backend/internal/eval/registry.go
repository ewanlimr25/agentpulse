package eval

import (
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	evaltypes "github.com/agentpulse/agentpulse/backend/internal/eval/types"
)

// NewRegistry builds an EvalType registry from the given per-project configs.
// Built-in types are always registered. Custom eval types (PromptTemplate != nil)
// are added alongside them.
func NewRegistry(configs []*domain.EvalConfig) evaltypes.Registry {
	r := evaltypes.Registry{
		"relevance":        &evaltypes.RelevanceEval{},
		"hallucination":    &evaltypes.HallucinationEval{},
		"faithfulness":     &evaltypes.FaithfulnessEval{},
		"toxicity":         &evaltypes.ToxicityEval{},
		"tool_correctness": &evaltypes.ToolCorrectnessEval{},
	}
	for _, cfg := range configs {
		if cfg.PromptTemplate != nil && *cfg.PromptTemplate != "" {
			custom := evaltypes.NewCustomEval(cfg.EvalName, cfg.SpanKind, *cfg.PromptTemplate)
			if custom != nil {
				r[cfg.EvalName] = custom
			}
		}
	}
	return r
}
