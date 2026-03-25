"""Tests for the LlamaIndex Dispatcher-based integration."""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock
from uuid import uuid4

import pytest


# ── Stub llama_index.core.instrumentation ────────────────────────────────────


def _install_llamaindex_stub() -> None:
    if "llama_index" in sys.modules:
        return

    def _make_mod(*parts):
        name = ".".join(parts)
        m = types.ModuleType(name)
        sys.modules[name] = m
        return m

    llama = _make_mod("llama_index")
    core = _make_mod("llama_index", "core")
    instr = _make_mod("llama_index", "core", "instrumentation")
    eh = _make_mod("llama_index", "core", "instrumentation", "event_handlers")
    ev = _make_mod("llama_index", "core", "instrumentation", "events")
    ev_llm = _make_mod("llama_index", "core", "instrumentation", "events", "llm")
    ev_query = _make_mod("llama_index", "core", "instrumentation", "events", "query")
    ev_ret = _make_mod("llama_index", "core", "instrumentation", "events", "retrieval")
    ev_agent = _make_mod("llama_index", "core", "instrumentation", "events", "agent")

    class BaseEvent:
        def __init__(self):
            self.id_ = str(uuid4())

    class BaseEventHandler:
        @classmethod
        def class_name(cls):
            return cls.__name__

        def handle(self, event, **kwargs):
            ...

    class _Dispatcher:
        def __init__(self):
            self.event_handlers = []

        def add_event_handler(self, h):
            self.event_handlers.append(h)

        def get_handlers(self):
            return self.event_handlers

    _dispatcher_instance = _Dispatcher()

    def get_dispatcher():
        return _dispatcher_instance

    instr.get_dispatcher = get_dispatcher  # type: ignore
    eh.BaseEventHandler = BaseEventHandler  # type: ignore
    ev.BaseEvent = BaseEvent  # type: ignore

    # LLM events
    class LLMChatStartEvent(BaseEvent):
        def __init__(self, messages=None, model_dict=None):
            super().__init__()
            self.messages = messages or []
            self.model_dict = model_dict or {}

    class LLMChatEndEvent(BaseEvent):
        def __init__(self, response=None, model_dict=None):
            super().__init__()
            self.response = response
            self.model_dict = model_dict or {}

    class LLMCompletionStartEvent(BaseEvent):
        def __init__(self, prompt="", model_dict=None):
            super().__init__()
            self.prompt = prompt
            self.model_dict = model_dict or {}

    class LLMCompletionEndEvent(BaseEvent):
        def __init__(self, response=None):
            super().__init__()
            self.response = response

    ev_llm.LLMChatStartEvent = LLMChatStartEvent  # type: ignore
    ev_llm.LLMChatEndEvent = LLMChatEndEvent  # type: ignore
    ev_llm.LLMCompletionStartEvent = LLMCompletionStartEvent  # type: ignore
    ev_llm.LLMCompletionEndEvent = LLMCompletionEndEvent  # type: ignore

    # Query events
    class QueryStartEvent(BaseEvent):
        def __init__(self, query=""):
            super().__init__()
            self.query = query

    class QueryEndEvent(BaseEvent):
        def __init__(self, response=None):
            super().__init__()
            self.response = response
            self.id_ = None  # will be matched by handler using str(type)

    ev_query.QueryStartEvent = QueryStartEvent  # type: ignore
    ev_query.QueryEndEvent = QueryEndEvent  # type: ignore

    # Retrieval events
    class RetrievalStartEvent(BaseEvent):
        def __init__(self, query=""):
            super().__init__()
            self.query = query

    class RetrievalEndEvent(BaseEvent):
        def __init__(self, nodes=None):
            super().__init__()
            self.nodes = nodes or []

    ev_ret.RetrievalStartEvent = RetrievalStartEvent  # type: ignore
    ev_ret.RetrievalEndEvent = RetrievalEndEvent  # type: ignore

    # Agent events
    class AgentRunStepStartEvent(BaseEvent):
        def __init__(self, task=None):
            super().__init__()
            self.task = task

    class AgentRunStepEndEvent(BaseEvent):
        pass

    ev_agent.AgentRunStepStartEvent = AgentRunStepStartEvent  # type: ignore
    ev_agent.AgentRunStepEndEvent = AgentRunStepEndEvent  # type: ignore

    llama.core = core  # type: ignore
    core.instrumentation = instr  # type: ignore


_install_llamaindex_stub()

from agentpulse.integrations.llamaindex import (  # noqa: E402
    AgentPulseEventHandler,
    instrument_llamaindex,
    uninstrument_llamaindex,
)
from llama_index.core.instrumentation.events.llm import (  # type: ignore[import]
    LLMChatStartEvent,
    LLMChatEndEvent,
    LLMCompletionStartEvent,
    LLMCompletionEndEvent,
)
from llama_index.core.instrumentation.events.query import QueryStartEvent, QueryEndEvent  # type: ignore[import]
from llama_index.core.instrumentation.events.retrieval import RetrievalStartEvent, RetrievalEndEvent  # type: ignore[import]
from llama_index.core.instrumentation.events.agent import AgentRunStepStartEvent, AgentRunStepEndEvent  # type: ignore[import]


@pytest.fixture(autouse=True)
def reset_llamaindex():
    """Reset the LlamaIndex handler between tests."""
    uninstrument_llamaindex()
    yield
    uninstrument_llamaindex()


# ── Registration ──────────────────────────────────────────────────────────────


def test_instrument_registers_handler(reset_otel):
    from llama_index.core.instrumentation import get_dispatcher
    handler = instrument_llamaindex()
    dispatcher = get_dispatcher()
    assert handler in dispatcher.event_handlers


def test_instrument_idempotent(reset_otel):
    from llama_index.core.instrumentation import get_dispatcher
    h1 = instrument_llamaindex()
    h2 = instrument_llamaindex()
    assert h1 is h2
    dispatcher = get_dispatcher()
    ap_handlers = [h for h in dispatcher.event_handlers if isinstance(h, AgentPulseEventHandler)]
    assert len(ap_handlers) == 1


def test_uninstrument_removes_handler(reset_otel):
    from llama_index.core.instrumentation import get_dispatcher
    instrument_llamaindex()
    uninstrument_llamaindex()
    dispatcher = get_dispatcher()
    ap_handlers = [h for h in dispatcher.event_handlers if isinstance(h, AgentPulseEventHandler)]
    assert len(ap_handlers) == 0


# ── LLM Chat events ───────────────────────────────────────────────────────────


def test_llm_chat_start_end_emits_span(reset_otel):
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = LLMChatStartEvent(model_dict={"model": "gpt-4o"})
    start.id_ = event_id
    handler.handle(start)

    response = MagicMock()
    response.raw = {"usage": {"input_tokens": 20, "output_tokens": 10}}
    response.message = MagicMock(content="Paris")
    end = LLMChatEndEvent(response=response)
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "llm.call"
    assert s.attributes.get("gen_ai.usage.input_tokens") == 20
    assert s.attributes.get("gen_ai.completion") == "Paris"


def test_llm_completion_start_end_emits_span(reset_otel):
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = LLMCompletionStartEvent(prompt="Say hello", model_dict={"model": "text-davinci"})
    start.id_ = event_id
    handler.handle(start)

    response = MagicMock(usage=MagicMock(prompt_tokens=5, completion_tokens=3), text="hello")
    end = LLMCompletionEndEvent(response=response)
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("gen_ai.prompt") == "Say hello"
    assert spans[0].attributes.get("gen_ai.completion") == "hello"


# ── Query events ──────────────────────────────────────────────────────────────


def test_query_start_end_emits_handoff_span(reset_otel):
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = QueryStartEvent(query="What is RAG?")
    start.id_ = event_id
    handler.handle(start)

    response = MagicMock(response="RAG is retrieval-augmented generation")
    end = QueryEndEvent(response=response)
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "agent.handoff"
    assert "RAG" in (s.attributes.get("gen_ai.prompt") or "")


# ── Retrieval events ──────────────────────────────────────────────────────────


def test_retrieval_emits_memory_read_span(reset_otel):
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = RetrievalStartEvent(query="Paris")
    start.id_ = event_id
    handler.handle(start)

    end = RetrievalEndEvent(nodes=[MagicMock(), MagicMock()])
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "memory.read"
    assert spans[0].attributes.get("memory.result_count") == 2


# ── Agent step events ─────────────────────────────────────────────────────────


def test_agent_step_emits_handoff_span(reset_otel):
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = AgentRunStepStartEvent()
    start.id_ = event_id
    handler.handle(start)

    end = AgentRunStepEndEvent()
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "agent.handoff"


# ── Context propagation ───────────────────────────────────────────────────────


def test_session_id_propagated_to_llm_span(reset_otel):
    from agentpulse._context import set_session_id
    set_session_id("lli-sess-1")
    handler = AgentPulseEventHandler()
    event_id = str(uuid4())

    start = LLMChatStartEvent(model_dict={})
    start.id_ = event_id
    handler.handle(start)

    end = LLMChatEndEvent(response=MagicMock(raw={}, message=None))
    end.id_ = event_id
    handler.handle(end)

    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.session_id") == "lli-sess-1"
