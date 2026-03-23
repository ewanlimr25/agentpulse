"""
AgentPulse Python SDK

Thin OTel wrapper for instrumenting AI agents with AgentPulse observability.

Quick start::

    from agentpulse import init_tracer, llm_call, record_llm_usage
    from opentelemetry import trace

    init_tracer()  # reads AGENTPULSE_PROJECT_ID from env

    @llm_call(model="gpt-4o", agent_name="MyAgent")
    def call_llm(prompt: str) -> str:
        result = my_client.chat(prompt)
        record_llm_usage(
            trace.get_current_span(),
            input_tokens=150, output_tokens=300,
            prompt=prompt, completion=result,
        )
        return result
"""

from agentpulse._context import (
    generate_session_id,
    get_run_id,
    get_session_id,
    reset_run,
    reset_session,
    set_run_id,
    set_session_id,
)
from agentpulse._version import __version__
from agentpulse.config import AgentPulseConfig, load_config
from agentpulse.spans import (
    handoff,
    handoff_ctx,
    llm_call,
    llm_call_ctx,
    memory_read,
    memory_read_ctx,
    memory_write,
    memory_write_ctx,
    record_llm_usage,
    tool_call,
    tool_call_ctx,
)
from agentpulse.tracer import init_tracer, shutdown

__all__ = [
    # Core setup
    "init_tracer",
    "shutdown",
    "AgentPulseConfig",
    "load_config",
    # Span decorators
    "llm_call",
    "tool_call",
    "handoff",
    "memory_read",
    "memory_write",
    # Span context managers
    "llm_call_ctx",
    "tool_call_ctx",
    "handoff_ctx",
    "memory_read_ctx",
    "memory_write_ctx",
    # Helpers
    "record_llm_usage",
    "set_run_id",
    "get_run_id",
    "reset_run",
    "set_session_id",
    "get_session_id",
    "generate_session_id",
    "reset_session",
    # Meta
    "__version__",
]
