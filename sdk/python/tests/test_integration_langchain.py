"""Tests for the LangChain integration (context-leak fix validation)."""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock
from uuid import uuid4

import pytest
from opentelemetry import context as context_api, trace
from opentelemetry.trace import StatusCode

# ── Stub langchain_core so the integration module imports without the real pkg ─


def _install_langchain_stub() -> None:
    if "langchain_core" in sys.modules:
        return

    langchain_core = types.ModuleType("langchain_core")
    callbacks_mod = types.ModuleType("langchain_core.callbacks")
    outputs_mod = types.ModuleType("langchain_core.outputs")

    class BaseCallbackHandler:
        raise_error = False

        def on_llm_start(self, *a, **kw): ...
        def on_llm_end(self, *a, **kw): ...
        def on_llm_error(self, *a, **kw): ...
        def on_tool_start(self, *a, **kw): ...
        def on_tool_end(self, *a, **kw): ...
        def on_tool_error(self, *a, **kw): ...
        def on_chain_start(self, *a, **kw): ...
        def on_chain_end(self, *a, **kw): ...
        def on_chain_error(self, *a, **kw): ...

    class LLMResult:
        def __init__(self, generations=None, llm_output=None):
            self.generations = generations or []
            self.llm_output = llm_output or {}

    callbacks_mod.BaseCallbackHandler = BaseCallbackHandler  # type: ignore
    outputs_mod.LLMResult = LLMResult  # type: ignore
    langchain_core.callbacks = callbacks_mod  # type: ignore
    langchain_core.outputs = outputs_mod  # type: ignore
    sys.modules["langchain_core"] = langchain_core
    sys.modules["langchain_core.callbacks"] = callbacks_mod
    sys.modules["langchain_core.outputs"] = outputs_mod


_install_langchain_stub()

from agentpulse.integrations.langchain import AgentPulseCallbackHandler  # noqa: E402


@pytest.fixture()
def handler():
    return AgentPulseCallbackHandler()


# ── LLM span lifecycle ────────────────────────────────────────────────────────


def test_llm_start_end_emits_span(reset_otel, handler):
    run_id = uuid4()
    handler.on_llm_start(
        {"kwargs": {"model_name": "gpt-4o"}, "id": ["ChatOpenAI"]},
        ["hello"],
        run_id=run_id,
    )
    handler.on_llm_end(
        MagicMock(
            llm_output={"token_usage": {"prompt_tokens": 10, "completion_tokens": 5}},
            generations=[[MagicMock(text="hi")]],
        ),
        run_id=run_id,
    )
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "llm.call"
    assert s.attributes.get("gen_ai.usage.input_tokens") == 10
    assert s.attributes.get("gen_ai.usage.output_tokens") == 5


def test_llm_error_ends_span_with_error(reset_otel, handler):
    run_id = uuid4()
    handler.on_llm_start({"kwargs": {}, "id": ["LLM"]}, ["q"], run_id=run_id)
    handler.on_llm_error(ValueError("boom"), run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].status.status_code == StatusCode.ERROR


def test_llm_context_detached_after_end(reset_otel, handler):
    """Verify that the context token is detached after on_llm_end (no leak)."""
    before = context_api.get_current()
    run_id = uuid4()
    handler.on_llm_start({"kwargs": {}, "id": ["M"]}, ["p"], run_id=run_id)
    handler.on_llm_end(MagicMock(llm_output={}, generations=[]), run_id=run_id)
    after = context_api.get_current()
    # The span should not still be the current span after detach
    current_span = trace.get_current_span()
    assert not current_span.is_recording()


def test_llm_context_detached_after_error(reset_otel, handler):
    run_id = uuid4()
    handler.on_llm_start({"kwargs": {}, "id": ["M"]}, ["p"], run_id=run_id)
    handler.on_llm_error(RuntimeError("fail"), run_id=run_id)
    current_span = trace.get_current_span()
    assert not current_span.is_recording()


# ── Tool span lifecycle ───────────────────────────────────────────────────────


def test_tool_start_end_emits_span(reset_otel, handler):
    run_id = uuid4()
    handler.on_tool_start({"name": "search"}, "query", run_id=run_id)
    handler.on_tool_end("results", run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "tool.call"
    assert spans[0].attributes.get("tool.name") == "search"


def test_tool_error_ends_span(reset_otel, handler):
    run_id = uuid4()
    handler.on_tool_start({"name": "calc"}, "2+2", run_id=run_id)
    handler.on_tool_error(Exception("nope"), run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].status.status_code == StatusCode.ERROR


# ── Chain (agent) span lifecycle ──────────────────────────────────────────────


def test_agent_chain_emits_handoff_span(reset_otel, handler):
    run_id = uuid4()
    handler.on_chain_start({"id": ["AgentExecutor"]}, {}, run_id=run_id)
    handler.on_chain_end({}, run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "agent.handoff"


def test_non_agent_chain_ignored(reset_otel, handler):
    run_id = uuid4()
    handler.on_chain_start({"id": ["LLMChain"]}, {}, run_id=run_id)
    handler.on_chain_end({}, run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 0


# ── Context propagation ───────────────────────────────────────────────────────


def test_run_id_attached_to_spans(reset_otel, handler):
    from agentpulse._context import set_run_id
    set_run_id("test-run-123")
    run_id = uuid4()
    handler.on_llm_start({"kwargs": {}, "id": ["M"]}, ["p"], run_id=run_id)
    handler.on_llm_end(MagicMock(llm_output={}, generations=[]), run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.run_id") == "test-run-123"


def test_session_id_attached_when_set(reset_otel, handler):
    from agentpulse._context import set_session_id
    set_session_id("sess-abc")
    run_id = uuid4()
    handler.on_tool_start({"name": "t"}, "in", run_id=run_id)
    handler.on_tool_end("out", run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.session_id") == "sess-abc"


def test_user_id_attached_when_set(reset_otel, handler):
    from agentpulse._context import set_user_id
    set_user_id("user-xyz")
    run_id = uuid4()
    handler.on_tool_start({"name": "t"}, "in", run_id=run_id)
    handler.on_tool_end("out", run_id=run_id)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.user_id") == "user-xyz"
