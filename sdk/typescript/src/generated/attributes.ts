// @generated -- DO NOT EDIT. Source: config/agent_attributes.yaml
// Run `npm run codegen` or `npm run build` to regenerate.

// ── Span kind ──────────────────────────────────────────────────────────────

/** Attribute key used to classify span kind. Set this on all AgentPulse spans. */
export const SPAN_KIND = "agentpulse.span_kind" as const

export type AgentSpanKind =
  | "llm.call"
  | "tool.call"
  | "agent.handoff"
  | "memory.read"
  | "memory.write"

export const LLM_CALL: AgentSpanKind = "llm.call"
export const TOOL_CALL: AgentSpanKind = "tool.call"
export const AGENT_HANDOFF: AgentSpanKind = "agent.handoff"
export const MEMORY_READ: AgentSpanKind = "memory.read"
export const MEMORY_WRITE: AgentSpanKind = "memory.write"

// ── Identity ────────────────────────────────────────────────────────────────

export const PROJECT_ID = "agentpulse.project.id" as const
export const RUN_ID = "agentpulse.run.id" as const
export const SESSION_ID = "agentpulse.session_id" as const
export const USER_ID = "agentpulse.user_id" as const
export const AGENT_NAME = "agent.name" as const

// ── LLM ─────────────────────────────────────────────────────────────────────

export const MODEL_ID = "gen_ai.request.model" as const
export const INPUT_TOKENS = "gen_ai.usage.input_tokens" as const
export const OUTPUT_TOKENS = "gen_ai.usage.output_tokens" as const
export const PROMPT = "gen_ai.prompt" as const
export const COMPLETION = "gen_ai.completion" as const
export const COST_USD = "agentpulse.cost_usd" as const

// ── Tool ────────────────────────────────────────────────────────────────────

export const TOOL_NAME = "tool.name" as const

// ── Handoff ─────────────────────────────────────────────────────────────────

export const HANDOFF_TARGET = "agentpulse.handoff.target_agent" as const

// ── Memory ──────────────────────────────────────────────────────────────────

export const MEMORY_KEY = "agentpulse.memory.key" as const

// ── Streaming ────────────────────────────────────────────────────────────────

export const TTFT_MS = "agentpulse.ttft_ms" as const
