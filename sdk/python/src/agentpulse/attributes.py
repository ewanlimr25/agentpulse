"""
Attribute key constants mirroring config/agent_attributes.yaml.

All values here must match what the AgentPulse collector expects.
Update this file (not collector YAML) when adding new attribute conventions.
"""

from typing import Literal

# ── Span kind ─────────────────────────────────────────────────────────────────

SPAN_KIND = "agentpulse.span_kind"

AgentSpanKind = Literal[
    "llm.call",
    "tool.call",
    "agent.handoff",
    "memory.read",
    "memory.write",
]

LLM_CALL: AgentSpanKind = "llm.call"
TOOL_CALL: AgentSpanKind = "tool.call"
AGENT_HANDOFF: AgentSpanKind = "agent.handoff"
MEMORY_READ: AgentSpanKind = "memory.read"
MEMORY_WRITE: AgentSpanKind = "memory.write"

# ── Identity ───────────────────────────────────────────────────────────────────

PROJECT_ID = "agentpulse.project_id"   # Checked by collector field_extraction
RUN_ID = "agentpulse.run_id"           # Groups spans into a single execution
SESSION_ID = "agentpulse.session_id"   # Groups multiple runs into a conversation (opt-in)
AGENT_NAME = "agent.name"              # Collector resolves via field_extraction.agent_name

# ── LLM ───────────────────────────────────────────────────────────────────────

MODEL_ID = "gen_ai.request.model"
INPUT_TOKENS = "gen_ai.usage.input_tokens"
OUTPUT_TOKENS = "gen_ai.usage.output_tokens"
PROMPT = "gen_ai.prompt"
COMPLETION = "gen_ai.completion"
COST_USD = "agentpulse.cost_usd"

# ── Tool ──────────────────────────────────────────────────────────────────────

TOOL_NAME = "tool.name"

# ── Handoff ───────────────────────────────────────────────────────────────────

HANDOFF_TARGET = "agentpulse.handoff.target_agent"

# ── Memory ────────────────────────────────────────────────────────────────────

MEMORY_KEY = "agentpulse.memory.key"
