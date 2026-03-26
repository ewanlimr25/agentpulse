package agentsemanticproc

import (
	"context"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const (
	attrAgentSpanKind = "agentpulse.span_kind"
	attrAgentName     = "agentpulse.agent.name"
	attrModelID       = "agentpulse.model_id"
	attrInputTokens   = "agentpulse.input_tokens"
	attrOutputTokens  = "agentpulse.output_tokens"
	attrCostUSD       = "agentpulse.cost_usd"
	attrRunID         = "agentpulse.run_id"
	attrProjectID     = "agentpulse.project_id"
	attrUserID        = "agentpulse.user_id"
	attrSessionID     = "agentpulse.session_id"
	attrTtftMs        = "agentpulse.ttft_ms"
)

// semanticProcessor enriches OTel spans with agent semantic fields.
type semanticProcessor struct {
	logger  *zap.Logger
	attrs   *attributeRegistry
	pricing *pricingRegistry
}

func newSemanticProcessor(logger *zap.Logger, attrs *attributeRegistry, pricing *pricingRegistry) *semanticProcessor {
	return &semanticProcessor{logger: logger, attrs: attrs, pricing: pricing}
}

func (p *semanticProcessor) Start(_ context.Context, _ component.Host) error { return nil }
func (p *semanticProcessor) Shutdown(_ context.Context) error                { return nil }

// ProcessTraces enriches every span in the batch with agent semantic attributes.
func (p *semanticProcessor) ProcessTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	for i := range td.ResourceSpans().Len() {
		rs := td.ResourceSpans().At(i)
		for j := range rs.ScopeSpans().Len() {
			ss := rs.ScopeSpans().At(j)
			for k := range ss.Spans().Len() {
				p.enrichSpan(ss.Spans().At(k))
			}
		}
	}
	return td, nil
}

// enrichSpan classifies a single span and writes computed attributes onto it.
func (p *semanticProcessor) enrichSpan(span ptrace.Span) {
	attrs := span.Attributes()

	// 1. Classify span kind
	attrs.PutStr(attrAgentSpanKind, p.classifySpanKind(span))

	// 2. Extract agent name
	if name := p.extractField("agent_name", attrs); name != "" {
		attrs.PutStr(attrAgentName, name)
	}

	// 3. Extract model ID
	modelID := p.extractField("model_id", attrs)
	if modelID != "" {
		attrs.PutStr(attrModelID, modelID)
	}

	// 4. Extract token counts
	inputTokens := uint32(getAttrInt(attrs, p.firstAttr("input_tokens")))
	outputTokens := uint32(getAttrInt(attrs, p.firstAttr("output_tokens")))
	if inputTokens > 0 {
		attrs.PutInt(attrInputTokens, int64(inputTokens))
	}
	if outputTokens > 0 {
		attrs.PutInt(attrOutputTokens, int64(outputTokens))
	}

	// 5. Compute cost (skip if explicitly provided)
	if _, exists := attrs.Get(p.attrs.Cost.ExplicitAttribute); !exists && p.pricing != nil {
		if cost := p.pricing.costUSD(modelID, inputTokens, outputTokens); cost > 0 {
			attrs.PutDouble(attrCostUSD, cost)
		}
	}

	// 6. Propagate run_id if present
	if runID := p.extractField("run_id", attrs); runID != "" {
		attrs.PutStr(attrRunID, runID)
	}

	// 7. Propagate project_id if present
	if projectID := p.extractField("project_id", attrs); projectID != "" {
		attrs.PutStr(attrProjectID, projectID)
	}

	// 8. Propagate session_id if present
	if sessionID := p.extractField("session_id", attrs); sessionID != "" {
		attrs.PutStr(attrSessionID, sessionID)
	}

	// 9. Propagate user_id if present; sanitize by rejecting values > 128 chars
	//    or containing '@' (likely PII email addresses).
	if userID := p.extractField("user_id", attrs); userID != "" {
		if len(userID) <= 128 && !strings.Contains(userID, "@") {
			attrs.PutStr(attrUserID, userID)
		} else {
			prefix := userID
			if len(prefix) > 20 {
				prefix = prefix[:20]
			}
			p.logger.Warn("user_id rejected: too long or contains '@' (use an opaque identifier, not an email)",
				zap.String("user_id_prefix", prefix),
			)
		}
	}

	// 10. Compute TTFT from stream.first_token event or SDK-stamped attribute.
	if ttft := p.computeTTFT(span); ttft > 0 {
		attrs.PutDouble(attrTtftMs, ttft)
	}
}

// computeTTFT computes time-to-first-token in milliseconds for a streaming span.
// It first checks for an SDK-computed attribute (handles serverless clock skew),
// then falls back to the stream.first_token SpanEvent timestamp.
// Returns 0 if no streaming data is present.
func (p *semanticProcessor) computeTTFT(span ptrace.Span) float64 {
	attrs := span.Attributes()

	// Prefer SDK-computed value (handles serverless clock issues)
	if v, ok := attrs.Get(attrTtftMs); ok {
		if ms := v.Double(); ms > 0 {
			return ms
		}
	}

	// Early exit if no events
	events := span.Events()
	if events.Len() == 0 {
		return 0
	}

	startNano := span.StartTimestamp().AsTime().UnixNano()
	spanDurNano := span.EndTimestamp().AsTime().UnixNano() - startNano

	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if e.Name() != "stream.first_token" {
			continue
		}
		eventNano := e.Timestamp().AsTime().UnixNano()
		if eventNano == 0 {
			// Zero timestamp is invalid — skip
			break
		}
		deltaNano := eventNano - startNano
		if deltaNano <= 0 {
			// NTP skew or clock issue — clamp to 0
			break
		}
		// Cap at span duration
		if deltaNano > spanDurNano && spanDurNano > 0 {
			deltaNano = spanDurNano
		}
		return float64(deltaNano) / 1e6 // ns → ms
	}
	return 0
}

// classifySpanKind determines the agent_span_kind for a span.
// Rules are evaluated in order; first match wins.
func (p *semanticProcessor) classifySpanKind(span ptrace.Span) string {
	attrs := span.Attributes()

	for _, rule := range p.attrs.SpanKindDetection {
		switch {
		case rule.Attribute == "_span_name":
			name := span.Name()
			for prefix, kind := range rule.PrefixMap {
				if strings.HasPrefix(name, prefix) {
					return kind
				}
			}

		case rule.Passthrough:
			if v, ok := attrs.Get(rule.Attribute); ok && v.Str() != "" {
				return v.Str()
			}

		case len(rule.ValueMap) > 0:
			if v, ok := attrs.Get(rule.Attribute); ok {
				if kind, mapped := rule.ValueMap[v.Str()]; mapped {
					return kind
				}
			}
		}
	}

	return "unknown"
}

// extractField returns the first non-empty string value for a named field.
func (p *semanticProcessor) extractField(fieldName string, attrs pcommon.Map) string {
	candidates, ok := p.attrs.FieldExtraction[fieldName]
	if !ok {
		return ""
	}
	for _, key := range candidates {
		if v, ok := attrs.Get(key); ok && v.Str() != "" {
			return v.Str()
		}
	}
	return ""
}

// firstAttr returns the first configured attribute key for a field.
func (p *semanticProcessor) firstAttr(fieldName string) string {
	if candidates, ok := p.attrs.FieldExtraction[fieldName]; ok && len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

// getAttrInt reads an integer/double attribute value, returning 0 if absent.
func getAttrInt(attrs pcommon.Map, key string) int64 {
	if key == "" {
		return 0
	}
	v, ok := attrs.Get(key)
	if !ok {
		return 0
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return int64(v.Double())
	default:
		return 0
	}
}
