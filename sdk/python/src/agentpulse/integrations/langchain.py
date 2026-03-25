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
from typing import Any
from uuid import UUID

from opentelemetry import context as context_api, trace
from opentelemetry.trace import Span, StatusCode

from agentpulse import attributes as attrs
from agentpulse.integrations._base import safe_truncate, set_common_attrs

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
    Each agent chain invocation → agent.handoff span

    Token counts are extracted from LLMResult.llm_output when available.

    Context propagation uses context_api.attach/detach token pairs, which is
    the correct OTel pattern for callback-based integrations where span start
    and end occur in different call frames.
    """

    def __init__(self) -> None:
        super().__init__()
        self._tracer = trace.get_tracer("agentpulse.langchain")
        # Map run_id → (span, context_token) so we can close them in end callbacks.
        self._spans: dict[UUID, tuple[Span, object]] = {}

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
        set_common_attrs(span, attrs.LLM_CALL, extra={attrs.MODEL_ID: str(model)})
        if prompts:
            span.set_attribute(attrs.PROMPT, safe_truncate(prompts[0]))
        token = context_api.attach(trace.set_span_in_context(span))
        self._spans[run_id] = (span, token)

    def on_llm_end(self, response: LLMResult, *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is None:
            return
        span, token = entry

        usage: dict[str, Any] = {}
        if response.llm_output:
            usage = response.llm_output.get("token_usage") or response.llm_output.get("usage", {})

        input_tokens = usage.get("prompt_tokens") or usage.get("input_tokens", 0)
        output_tokens = usage.get("completion_tokens") or usage.get("output_tokens", 0)
        if input_tokens:
            span.set_attribute(attrs.INPUT_TOKENS, int(input_tokens))
        if output_tokens:
            span.set_attribute(attrs.OUTPUT_TOKENS, int(output_tokens))

        if response.generations and response.generations[0]:
            gen = response.generations[0][0]
            text = getattr(gen, "text", None) or str(gen)
            span.set_attribute(attrs.COMPLETION, safe_truncate(text))

        context_api.detach(token)
        span.end()

    def on_llm_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is not None:
            span, token = entry
            span.set_status(StatusCode.ERROR, str(error))
            context_api.detach(token)
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
        set_common_attrs(span, attrs.TOOL_CALL, extra={attrs.TOOL_NAME: str(tool_name)})
        token = context_api.attach(trace.set_span_in_context(span))
        self._spans[run_id] = (span, token)

    def on_tool_end(self, output: Any, *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is not None:
            span, token = entry
            context_api.detach(token)
            span.end()

    def on_tool_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is not None:
            span, token = entry
            span.set_status(StatusCode.ERROR, str(error))
            context_api.detach(token)
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
        if "agent" not in chain_name.lower():
            return

        span = self._tracer.start_span(f"agent.{chain_name}")
        set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=str(chain_name))
        token = context_api.attach(trace.set_span_in_context(span))
        self._spans[run_id] = (span, token)

    def on_chain_end(self, outputs: dict[str, Any], *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is not None:
            span, token = entry
            context_api.detach(token)
            span.end()

    def on_chain_error(self, error: BaseException, *, run_id: UUID, **kwargs: Any) -> None:
        entry = self._spans.pop(run_id, None)
        if entry is not None:
            span, token = entry
            span.set_status(StatusCode.ERROR, str(error))
            context_api.detach(token)
            span.end()
