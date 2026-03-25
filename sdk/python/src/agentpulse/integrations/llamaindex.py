"""
LlamaIndex integration for AgentPulse.

Uses the llama_index.core.instrumentation.Dispatcher event handler system
(available in llama-index-core >= 0.10.68). This is a purely additive,
non-destructive approach — unlike replacing Settings.callback_manager which
silently drops any existing handlers (Arize, LangFuse, etc.).

Usage::

    from agentpulse import init_tracer
    from agentpulse.integrations.llamaindex import instrument_llamaindex

    init_tracer()
    instrument_llamaindex()   # registers globally; call once at startup

    # All subsequent LlamaIndex calls are automatically traced:
    response = query_engine.query("What is RAG?")

Requires: pip install 'agentpulse[llamaindex]'
"""

from __future__ import annotations

import logging
from typing import Any, Optional

from opentelemetry import context as context_api, trace
from opentelemetry.trace import StatusCode

from agentpulse import attributes as attrs
from agentpulse.integrations._base import (
    extract_usage,
    safe_truncate,
    set_common_attrs,
)

logger = logging.getLogger(__name__)

try:
    from llama_index.core.instrumentation import get_dispatcher  # type: ignore[import]
    from llama_index.core.instrumentation.event_handlers import BaseEventHandler  # type: ignore[import]
    from llama_index.core.instrumentation.events import BaseEvent  # type: ignore[import]
except ImportError as _exc:
    raise ImportError(
        "LlamaIndex integration requires llama-index-core>=0.10.68. "
        "Install with: pip install 'agentpulse[llamaindex]'"
    ) from _exc

# Import event types individually so missing ones don't break the whole module.
try:
    from llama_index.core.instrumentation.events.llm import (  # type: ignore[import]
        LLMChatStartEvent,
        LLMChatEndEvent,
        LLMCompletionStartEvent,
        LLMCompletionEndEvent,
    )
    _HAS_LLM_EVENTS = True
except ImportError:
    _HAS_LLM_EVENTS = False
    logger.debug("AgentPulse LlamaIndex: LLM events not available in this version")

try:
    from llama_index.core.instrumentation.events.query import (  # type: ignore[import]
        QueryStartEvent,
        QueryEndEvent,
    )
    _HAS_QUERY_EVENTS = True
except ImportError:
    _HAS_QUERY_EVENTS = False

try:
    from llama_index.core.instrumentation.events.retrieval import (  # type: ignore[import]
        RetrievalStartEvent,
        RetrievalEndEvent,
    )
    _HAS_RETRIEVAL_EVENTS = True
except ImportError:
    _HAS_RETRIEVAL_EVENTS = False

try:
    from llama_index.core.instrumentation.events.agent import (  # type: ignore[import]
        AgentRunStepStartEvent,
        AgentRunStepEndEvent,
    )
    _HAS_AGENT_EVENTS = True
except ImportError:
    _HAS_AGENT_EVENTS = False


def _get_tracer() -> trace.Tracer:
    return trace.get_tracer("agentpulse.llamaindex")
_handler_instance: Optional[AgentPulseEventHandler] = None


class AgentPulseEventHandler(BaseEventHandler):
    """LlamaIndex event handler that emits AgentPulse spans.

    Registered with the root Dispatcher via add_event_handler(). This is
    additive — existing handlers (e.g. Arize Phoenix, custom loggers) are
    unaffected.

    Span kinds emitted:
        LLMChat/LLMCompletion events  → llm.call
        Query events                  → agent.handoff (wraps the full query)
        Retrieval events              → memory.read
        AgentRunStep events           → agent.handoff
        Tool call events (via agent)  → tool.call (when available)
    """

    def __init__(self) -> None:
        super().__init__()
        # span_id → (span, context_token) for start/end event pairing
        self._spans: dict[str, tuple[Any, object]] = {}

    @classmethod
    def class_name(cls) -> str:
        return "AgentPulseEventHandler"

    def handle(self, event: BaseEvent, **kwargs: Any) -> None:  # type: ignore[override]
        """Route incoming events to the appropriate span handler."""
        try:
            self._dispatch(event)
        except Exception as exc:
            logger.debug("AgentPulse LlamaIndex handler error: %s", exc)

    def _dispatch(self, event: BaseEvent) -> None:
        event_type = type(event).__name__

        if _HAS_LLM_EVENTS:
            if isinstance(event, LLMChatStartEvent):
                self._on_llm_start(event)
                return
            if isinstance(event, LLMChatEndEvent):
                self._on_llm_end(event)
                return
            if isinstance(event, LLMCompletionStartEvent):
                self._on_completion_start(event)
                return
            if isinstance(event, LLMCompletionEndEvent):
                self._on_completion_end(event)
                return

        if _HAS_QUERY_EVENTS:
            if isinstance(event, QueryStartEvent):
                self._on_query_start(event)
                return
            if isinstance(event, QueryEndEvent):
                self._on_query_end(event)
                return

        if _HAS_RETRIEVAL_EVENTS:
            if isinstance(event, RetrievalStartEvent):
                self._on_retrieval_start(event)
                return
            if isinstance(event, RetrievalEndEvent):
                self._on_retrieval_end(event)
                return

        if _HAS_AGENT_EVENTS:
            if isinstance(event, AgentRunStepStartEvent):
                self._on_agent_step_start(event)
                return
            if isinstance(event, AgentRunStepEndEvent):
                self._on_agent_step_end(event)
                return

    # ── Span helpers ───────────────────────────────────────────────────────────

    def _open(self, key: str, span_name: str, span_kind: str, agent_name: Optional[str] = None, extra: Optional[dict] = None) -> None:
        span = _get_tracer().start_span(span_name)
        set_common_attrs(span, span_kind, agent_name=agent_name, extra=extra)
        token = context_api.attach(trace.set_span_in_context(span))
        self._spans[key] = (span, token)

    def _close(self, key: str, error: Optional[Exception] = None) -> Optional[Any]:
        entry = self._spans.pop(key, None)
        if entry is None:
            return None
        span, token = entry
        if error is not None:
            span.set_status(StatusCode.ERROR, str(error))
        context_api.detach(token)
        span.end()
        return span

    def _key(self, event: BaseEvent) -> str:
        return getattr(event, "id_", type(event).__name__)

    # ── LLM Chat ───────────────────────────────────────────────────────────────

    def _on_llm_start(self, event: Any) -> None:
        model = "unknown"
        messages = getattr(event, "messages", None)
        llm = getattr(event, "model_dict", {})
        model = llm.get("model", llm.get("model_name", "unknown"))
        self._open(self._key(event), f"llm.{model}", attrs.LLM_CALL, extra={attrs.MODEL_ID: model})
        span, _ = self._spans.get(self._key(event), (None, None))
        if span and messages:
            try:
                prompt_text = "\n".join(
                    getattr(m, "content", str(m)) for m in messages
                )
                span.set_attribute(attrs.PROMPT, safe_truncate(prompt_text))
            except Exception:
                pass

    def _on_llm_end(self, event: Any) -> None:
        key = self._key(event)
        span, token = self._spans.get(key, (None, None))
        if span:
            response = getattr(event, "response", None)
            if response:
                usage = getattr(response, "raw", {})
                if isinstance(usage, dict):
                    u = usage.get("usage", {})
                    input_t = int(u.get("input_tokens", u.get("prompt_tokens", 0)))
                    output_t = int(u.get("output_tokens", u.get("completion_tokens", 0)))
                else:
                    input_t, output_t = extract_usage(getattr(response, "usage", None))
                if input_t:
                    span.set_attribute(attrs.INPUT_TOKENS, input_t)
                if output_t:
                    span.set_attribute(attrs.OUTPUT_TOKENS, output_t)
                # Capture completion text
                msg = getattr(response, "message", None)
                if msg:
                    content = getattr(msg, "content", None)
                    if content:
                        span.set_attribute(attrs.COMPLETION, safe_truncate(str(content)))
        self._close(key)

    # ── LLM Completion (non-chat) ──────────────────────────────────────────────

    def _on_completion_start(self, event: Any) -> None:
        prompt = getattr(event, "prompt", "")
        model = getattr(event, "model_dict", {}).get("model", "unknown")
        self._open(self._key(event), f"llm.{model}", attrs.LLM_CALL, extra={attrs.MODEL_ID: model})
        span, _ = self._spans.get(self._key(event), (None, None))
        if span and prompt:
            span.set_attribute(attrs.PROMPT, safe_truncate(str(prompt)))

    def _on_completion_end(self, event: Any) -> None:
        key = self._key(event)
        span, _ = self._spans.get(key, (None, None))
        if span:
            response = getattr(event, "response", None)
            if response:
                input_t, output_t = extract_usage(getattr(response, "usage", None))
                if input_t:
                    span.set_attribute(attrs.INPUT_TOKENS, input_t)
                if output_t:
                    span.set_attribute(attrs.OUTPUT_TOKENS, output_t)
                text = getattr(response, "text", None)
                if text:
                    span.set_attribute(attrs.COMPLETION, safe_truncate(str(text)))
        self._close(key)

    # ── Query ──────────────────────────────────────────────────────────────────

    def _on_query_start(self, event: Any) -> None:
        query = getattr(event, "query", "")
        self._open(self._key(event), "query", attrs.AGENT_HANDOFF)
        span, _ = self._spans.get(self._key(event), (None, None))
        if span and query:
            span.set_attribute(attrs.PROMPT, safe_truncate(str(query)))

    def _on_query_end(self, event: Any) -> None:
        key = self._key(event)
        span, _ = self._spans.get(key, (None, None))
        if span:
            response = getattr(event, "response", None)
            if response:
                response_text = getattr(response, "response", None) or str(response)
                span.set_attribute(attrs.COMPLETION, safe_truncate(str(response_text)))
        self._close(key)

    # ── Retrieval ──────────────────────────────────────────────────────────────

    def _on_retrieval_start(self, event: Any) -> None:
        query = getattr(event, "query", "")
        self._open(self._key(event), "retrieval", attrs.MEMORY_READ)
        span, _ = self._spans.get(self._key(event), (None, None))
        if span and query:
            span.set_attribute("memory.query", safe_truncate(str(query)))

    def _on_retrieval_end(self, event: Any) -> None:
        key = self._key(event)
        span, _ = self._spans.get(key, (None, None))
        if span:
            nodes = getattr(event, "nodes", [])
            span.set_attribute("memory.result_count", len(nodes))
        self._close(key)

    # ── Agent step ─────────────────────────────────────────────────────────────

    def _on_agent_step_start(self, event: Any) -> None:
        task = getattr(event, "task", None)
        agent_name = getattr(task, "extra_state", {}).get("agent_name") if task else None
        self._open(self._key(event), "agent.step", attrs.AGENT_HANDOFF, agent_name=agent_name)

    def _on_agent_step_end(self, event: Any) -> None:
        self._close(self._key(event))


# ── Public API ────────────────────────────────────────────────────────────────


def instrument_llamaindex() -> AgentPulseEventHandler:
    """Register AgentPulse as a LlamaIndex event handler (global, non-destructive).

    Safe to call multiple times — subsequent calls are no-ops that return the
    existing handler. Does not replace or modify the Settings.callback_manager.

    Returns the registered handler instance.
    """
    global _handler_instance
    if _handler_instance is not None:
        logger.debug("AgentPulse LlamaIndex: already instrumented; returning existing handler")
        return _handler_instance

    dispatcher = get_dispatcher()

    # Guard against double-registration (e.g. module reload in notebooks)
    for h in getattr(dispatcher, "event_handlers", []):
        if isinstance(h, AgentPulseEventHandler):
            _handler_instance = h
            return h

    _handler_instance = AgentPulseEventHandler()
    dispatcher.add_event_handler(_handler_instance)
    logger.debug("AgentPulse LlamaIndex: registered event handler on root dispatcher")
    return _handler_instance


def uninstrument_llamaindex() -> None:
    """Remove the AgentPulse event handler from the LlamaIndex dispatcher."""
    global _handler_instance
    if _handler_instance is None:
        return
    dispatcher = get_dispatcher()
    handlers = getattr(dispatcher, "event_handlers", [])
    dispatcher.event_handlers = [h for h in handlers if not isinstance(h, AgentPulseEventHandler)]
    _handler_instance = None
    logger.debug("AgentPulse LlamaIndex: event handler removed")
