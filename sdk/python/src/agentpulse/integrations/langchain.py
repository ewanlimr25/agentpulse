"""
LangChain integration for AgentPulse.

Provides a callback handler that automatically instruments LangChain chains,
LLM calls, and tool calls with AgentPulse spans.

Usage::

    from agentpulse import init_tracer
    from agentpulse.integrations.langchain import AgentPulseCallbackHandler

    init_tracer()
    handler = AgentPulseCallbackHandler()

    # Pass to any LangChain runnable
    chain.invoke({"input": "..."}, config={"callbacks": [handler]})

    # Or add as a global callback
    from langchain_core.callbacks import set_global_handler
    set_global_handler("agentpulse", handler=handler)

Requires: pip install 'agentpulse[langchain]'
"""

from __future__ import annotations

import logging
from typing import Any, Optional
from uuid import UUID

from opentelemetry import trace
from opentelemetry.trace import Span, StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import get_project_id, get_run_id

logger = logging.getLogger(__name__)

try:
    from langchain_core.callbacks import BaseCallbackHandler
    from langchain_core.outputs import LLMResult
except ImportError as _exc:
    raise ImportError(
        "LangChain integration requires langchain-core. "
        "Install with: pip install 'agentpulse[langchain]'"
    ) from _exc


class AgentPulseCallbackHandler(BaseCallbackHandler):
    """LangChain callback handler that emits AgentPulse spans.

    Each LLM invocation → llm.call span
    Each tool invocation → tool.call span
    Each chain invocation → agent.handoff span (if it represents an agent)

    Token counts are extracted from LLMResult.llm_output when available.
    """

    def __init__(self) -> None:
        super().__init__()
        self._tracer = trace.get_tracer("agentpulse.langchain")
        # Map run_id → open Span so we can close it in the end callbacks
        self._spans: dict[UUID, Span] = {}

    # ── LLM callbacks ─────────────────────────────────────────────────────────

    def on_llm_start(
        self,
        serialized: dict[str, Any],
        prompts: list[str],
        *,
        run_id: UUID,
        **kwargs: Any,
    ) -> None:
        model = (
            serialized.get("kwargs", {}).get("model_name")
            or serialized.get("kwargs", {}).get("model")
            or serialized.get("id", ["unknown"])[-1]
        )
        span = self._tracer.start_span(f"llm.{model}")
        span.set_attribute(attrs.SPAN_KIND, attrs.LLM_CALL)
        span.set_attribute(attrs.MODEL_ID, str(model))
        span.set_attribute(attrs.RUN_ID, get_run_id())
        project_id = get_project_id()
        if project_id:
            span.set_attribute(attrs.PROJECT_ID, project_id)
        if prompts:
            span.set_attribute(attrs.PROMPT, prompts[0][:2000])  # Truncate to 2KB

        # Activate span in OTel context so children nest correctly
        ctx = trace.use_span(span, end_on_exit=False)
        ctx.__enter__()
        self._spans[run_id] = span

    def on_llm_end(self, response: LLMResult, *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is None:
            return

        # Extract token usage from LLMResult metadata
        usage = {}
        if response.llm_output:
            usage = response.llm_output.get("token_usage") or response.llm_output.get("usage", {})

        input_tokens = usage.get("prompt_tokens") or usage.get("input_tokens", 0)
        output_tokens = usage.get("completion_tokens") or usage.get("output_tokens", 0)

        if input_tokens:
            span.set_attribute(attrs.INPUT_TOKENS, int(input_tokens))
        if output_tokens:
            span.set_attribute(attrs.OUTPUT_TOKENS, int(output_tokens))

        # Capture first completion text
        if response.generations and response.generations[0]:
            gen = response.generations[0][0]
            text = getattr(gen, "text", None) or str(gen)
            span.set_attribute(attrs.COMPLETION, text[:2000])

        span.end()

    def on_llm_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is not None:
            span.set_status(StatusCode.ERROR, str(error))
            span.end()

    # ── Tool callbacks ─────────────────────────────────────────────────────────

    def on_tool_start(
        self,
        serialized: dict[str, Any],
        input_str: str,
        *,
        run_id: UUID,
        **kwargs: Any,
    ) -> None:
        tool_name = serialized.get("name", "unknown_tool")
        span = self._tracer.start_span(f"tool.{tool_name}")
        span.set_attribute(attrs.SPAN_KIND, attrs.TOOL_CALL)
        span.set_attribute(attrs.TOOL_NAME, str(tool_name))
        span.set_attribute(attrs.RUN_ID, get_run_id())
        project_id = get_project_id()
        if project_id:
            span.set_attribute(attrs.PROJECT_ID, project_id)

        ctx = trace.use_span(span, end_on_exit=False)
        ctx.__enter__()
        self._spans[run_id] = span

    def on_tool_end(self, output: Any, *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is not None:
            span.end()

    def on_tool_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is not None:
            span.set_status(StatusCode.ERROR, str(error))
            span.end()

    # ── Chain callbacks ────────────────────────────────────────────────────────

    def on_chain_start(
        self,
        serialized: dict[str, Any],
        inputs: dict[str, Any],
        *,
        run_id: UUID,
        **kwargs: Any,
    ) -> None:
        chain_name = serialized.get("id", ["chain"])[-1]
        # Only emit handoff spans for AgentExecutor / agent-like chains
        if "agent" not in chain_name.lower():
            return

        span = self._tracer.start_span(f"agent.{chain_name}")
        span.set_attribute(attrs.SPAN_KIND, attrs.AGENT_HANDOFF)
        span.set_attribute(attrs.AGENT_NAME, str(chain_name))
        span.set_attribute(attrs.RUN_ID, get_run_id())
        project_id = get_project_id()
        if project_id:
            span.set_attribute(attrs.PROJECT_ID, project_id)

        ctx = trace.use_span(span, end_on_exit=False)
        ctx.__enter__()
        self._spans[run_id] = span

    def on_chain_end(self, outputs: dict[str, Any], *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is not None:
            span.end()

    def on_chain_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        span = self._spans.pop(run_id, None)
        if span is not None:
            span.set_status(StatusCode.ERROR, str(error))
            span.end()
