"""Tests for span kind decorators, context managers, and record_llm_usage."""

from __future__ import annotations

import pytest
from opentelemetry import trace
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

import agentpulse.attributes as attrs
from agentpulse import (
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
from agentpulse._context import get_run_id, reset_run


# ── Helpers ───────────────────────────────────────────────────────────────────

def get_spans(exporter: InMemorySpanExporter) -> list:
    return exporter.get_finished_spans()


def span_attr(span, key: str) -> str | int | float | None:
    return span.attributes.get(key)


# ── LLM call ──────────────────────────────────────────────────────────────────

def test_llm_call_decorator_sets_span_kind(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    def my_llm(prompt: str) -> str:
        return "response"

    my_llm("hello")

    spans = get_spans(reset_otel)
    assert len(spans) == 1
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.LLM_CALL


def test_llm_call_decorator_sets_model(reset_otel: InMemorySpanExporter):
    @llm_call(model="claude-sonnet-4-6")
    def my_llm() -> str:
        return "ok"

    my_llm()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.MODEL_ID) == "claude-sonnet-4-6"


def test_llm_call_decorator_sets_agent_name(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o", agent_name="ResearchAgent")
    def my_llm() -> str:
        return "ok"

    my_llm()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.AGENT_NAME) == "ResearchAgent"


def test_llm_call_decorator_sets_run_id(reset_otel: InMemorySpanExporter):
    run_id = get_run_id()  # Establish run_id for this test

    @llm_call(model="gpt-4o")
    def my_llm() -> str:
        return "ok"

    my_llm()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.RUN_ID) == run_id


def test_llm_call_decorator_sets_project_id(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    def my_llm() -> str:
        return "ok"

    my_llm()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.PROJECT_ID) == "test-project-id"


def test_llm_call_span_name_defaults_to_function_name(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    def call_the_llm() -> str:
        return "ok"

    call_the_llm()
    spans = get_spans(reset_otel)
    assert spans[0].name == "call_the_llm"


def test_llm_call_custom_span_name(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o", span_name="my.custom.span")
    def my_llm() -> str:
        return "ok"

    my_llm()
    spans = get_spans(reset_otel)
    assert spans[0].name == "my.custom.span"


def test_llm_call_sets_error_on_exception(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    def my_llm() -> str:
        raise RuntimeError("API failure")

    with pytest.raises(RuntimeError):
        my_llm()

    spans = get_spans(reset_otel)
    from opentelemetry.trace import StatusCode
    assert spans[0].status.status_code == StatusCode.ERROR


@pytest.mark.asyncio
async def test_llm_call_async_decorator(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    async def async_llm(prompt: str) -> str:
        return "async response"

    result = await async_llm("test")

    assert result == "async response"
    spans = get_spans(reset_otel)
    assert len(spans) == 1
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.LLM_CALL


# ── Context manager form ──────────────────────────────────────────────────────

def test_llm_call_ctx_yields_span(reset_otel: InMemorySpanExporter):
    with llm_call_ctx(model="gpt-4o") as span:
        assert span is not None
        assert span_attr(span, attrs.SPAN_KIND) == attrs.LLM_CALL

    spans = get_spans(reset_otel)
    assert len(spans) == 1


# ── Tool call ─────────────────────────────────────────────────────────────────

def test_tool_call_decorator(reset_otel: InMemorySpanExporter):
    @tool_call(tool_name="web_search")
    def search(q: str) -> str:
        return "results"

    search("query")
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.TOOL_CALL
    assert span_attr(spans[0], attrs.TOOL_NAME) == "web_search"


def test_tool_call_ctx(reset_otel: InMemorySpanExporter):
    with tool_call_ctx(tool_name="calculator") as span:
        assert span_attr(span, attrs.TOOL_NAME) == "calculator"

    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.TOOL_CALL


# ── Handoff ───────────────────────────────────────────────────────────────────

def test_handoff_decorator(reset_otel: InMemorySpanExporter):
    @handoff(target_agent="ResearchAgent", agent_name="OrchestratorAgent")
    def delegate(task: str) -> str:
        return "delegated"

    delegate("research this")
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.AGENT_HANDOFF
    assert span_attr(spans[0], attrs.HANDOFF_TARGET) == "ResearchAgent"
    assert span_attr(spans[0], attrs.AGENT_NAME) == "OrchestratorAgent"


def test_handoff_ctx(reset_otel: InMemorySpanExporter):
    with handoff_ctx(target_agent="ChildAgent") as span:
        assert span_attr(span, attrs.HANDOFF_TARGET) == "ChildAgent"

    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.AGENT_HANDOFF


# ── Memory ────────────────────────────────────────────────────────────────────

def test_memory_read_decorator(reset_otel: InMemorySpanExporter):
    @memory_read(key="user_profile")
    def read_mem() -> dict:
        return {}

    read_mem()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.MEMORY_READ
    assert span_attr(spans[0], attrs.MEMORY_KEY) == "user_profile"


def test_memory_write_decorator(reset_otel: InMemorySpanExporter):
    @memory_write(key="conversation_history")
    def write_mem(data: str) -> None:
        pass

    write_mem("hello")
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.SPAN_KIND) == attrs.MEMORY_WRITE
    assert span_attr(spans[0], attrs.MEMORY_KEY) == "conversation_history"


def test_memory_read_ctx(reset_otel: InMemorySpanExporter):
    with memory_read_ctx(key="facts") as span:
        assert span_attr(span, attrs.MEMORY_KEY) == "facts"


def test_memory_write_ctx(reset_otel: InMemorySpanExporter):
    with memory_write_ctx(key="facts") as span:
        assert span_attr(span, attrs.SPAN_KIND) == attrs.MEMORY_WRITE


# ── record_llm_usage ──────────────────────────────────────────────────────────

def test_record_llm_usage_sets_tokens(reset_otel: InMemorySpanExporter):
    @llm_call(model="gpt-4o")
    def my_llm() -> str:
        record_llm_usage(
            trace.get_current_span(),
            input_tokens=100,
            output_tokens=200,
            prompt="hello",
            completion="world",
        )
        return "world"

    my_llm()
    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.INPUT_TOKENS) == 100
    assert span_attr(spans[0], attrs.OUTPUT_TOKENS) == 200
    assert span_attr(spans[0], attrs.PROMPT) == "hello"
    assert span_attr(spans[0], attrs.COMPLETION) == "world"


def test_record_llm_usage_optional_fields(reset_otel: InMemorySpanExporter):
    with llm_call_ctx(model="gpt-4o") as span:
        record_llm_usage(span, input_tokens=50, output_tokens=75)

    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.INPUT_TOKENS) == 50
    # prompt and completion should not be set
    assert attrs.PROMPT not in spans[0].attributes
    assert attrs.COMPLETION not in spans[0].attributes


def test_record_llm_usage_explicit_cost(reset_otel: InMemorySpanExporter):
    with llm_call_ctx(model="gpt-4o") as span:
        record_llm_usage(span, input_tokens=50, output_tokens=75, cost_usd=0.001234)

    spans = get_spans(reset_otel)
    assert span_attr(spans[0], attrs.COST_USD) == pytest.approx(0.001234)


# ── Parent-child relationship (DAG topology) ──────────────────────────────────

def test_parent_child_span_relationship(reset_otel: InMemorySpanExporter):
    """Handoff wrapping an llm_call must produce correct parent_span_id."""

    @llm_call(model="gpt-4o", agent_name="ResearchAgent")
    def inner_llm() -> str:
        return "result"

    @handoff(target_agent="ResearchAgent", agent_name="OrchestratorAgent")
    def orchestrate() -> str:
        return inner_llm()

    orchestrate()

    spans = get_spans(reset_otel)
    assert len(spans) == 2

    # Find parent and child
    handoff_span = next(s for s in spans if s.attributes.get(attrs.SPAN_KIND) == attrs.AGENT_HANDOFF)
    llm_span = next(s for s in spans if s.attributes.get(attrs.SPAN_KIND) == attrs.LLM_CALL)

    # The llm span's parent must be the handoff span
    assert llm_span.parent is not None
    assert llm_span.parent.span_id == handoff_span.context.span_id


def test_run_id_consistent_within_execution(reset_otel: InMemorySpanExporter):
    """All spans in one execution share the same run_id."""
    reset_run()  # Ensure fresh run_id

    @llm_call(model="gpt-4o")
    def llm_a() -> str:
        return "a"

    @llm_call(model="gpt-4o")
    def llm_b() -> str:
        return "b"

    llm_a()
    llm_b()

    spans = get_spans(reset_otel)
    assert len(spans) == 2
    run_ids = {span_attr(s, attrs.RUN_ID) for s in spans}
    assert len(run_ids) == 1  # All same run_id
