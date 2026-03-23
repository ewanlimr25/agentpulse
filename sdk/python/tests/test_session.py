"""Tests for session context helpers and span attribute propagation."""

from __future__ import annotations

import uuid

import pytest
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

import agentpulse.attributes as attrs
from agentpulse import (
    generate_session_id,
    get_session_id,
    llm_call,
    llm_call_ctx,
    reset_session,
    set_session_id,
)
from agentpulse._context import reset_run


# ── Helpers ───────────────────────────────────────────────────────────────────

def span_attr(span, key: str):
    return span.attributes.get(key)


# ── generate_session_id ───────────────────────────────────────────────────────

def test_generate_session_id_returns_valid_uuid():
    sid = generate_session_id()
    parsed = uuid.UUID(sid)  # raises if invalid
    assert str(parsed) == sid


def test_generate_session_id_returns_unique_values():
    ids = {generate_session_id() for _ in range(100)}
    assert len(ids) == 100


# ── set_session_id / get_session_id ──────────────────────────────────────────

def test_get_session_id_returns_none_when_not_set(reset_otel):
    reset_session()
    assert get_session_id() is None


def test_set_get_session_id_round_trip(reset_otel):
    set_session_id("my-session-123")
    assert get_session_id() == "my-session-123"
    reset_session()


def test_set_session_id_with_generated_id(reset_otel):
    sid = generate_session_id()
    set_session_id(sid)
    assert get_session_id() == sid
    reset_session()


# ── reset_session ─────────────────────────────────────────────────────────────

def test_reset_session_clears_session_id(reset_otel):
    set_session_id("to-be-cleared")
    reset_session()
    assert get_session_id() is None


# ── Span attribute propagation ────────────────────────────────────────────────

def test_session_id_stamped_on_span_when_set(reset_otel: InMemorySpanExporter):
    reset_run()
    set_session_id("session-abc")

    @llm_call(model="gpt-4o")
    def my_fn() -> str:
        return "ok"

    my_fn()
    reset_session()

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert span_attr(spans[0], attrs.SESSION_ID) == "session-abc"


def test_session_id_not_stamped_when_not_set(reset_otel: InMemorySpanExporter):
    reset_run()
    reset_session()

    @llm_call(model="gpt-4o")
    def my_fn() -> str:
        return "ok"

    my_fn()

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert span_attr(spans[0], attrs.SESSION_ID) is None


def test_session_id_stamped_on_ctx_manager_span(reset_otel: InMemorySpanExporter):
    reset_run()
    set_session_id("session-ctx-456")

    with llm_call_ctx(model="claude-sonnet-4-6"):
        pass

    reset_session()

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert span_attr(spans[0], attrs.SESSION_ID) == "session-ctx-456"


def test_multiple_spans_share_same_session_id(reset_otel: InMemorySpanExporter):
    reset_run()
    set_session_id("shared-session")

    @llm_call(model="gpt-4o")
    def fn1() -> str:
        return "a"

    @llm_call(model="gpt-4o")
    def fn2() -> str:
        return "b"

    fn1()
    fn2()
    reset_session()

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 2
    for span in spans:
        assert span_attr(span, attrs.SESSION_ID) == "shared-session"


def test_reset_session_stops_stamping(reset_otel: InMemorySpanExporter):
    reset_run()
    set_session_id("before-reset")

    @llm_call(model="gpt-4o")
    def fn_before() -> str:
        return "a"

    fn_before()
    reset_session()

    reset_run()

    @llm_call(model="gpt-4o")
    def fn_after() -> str:
        return "b"

    fn_after()

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 2
    assert span_attr(spans[0], attrs.SESSION_ID) == "before-reset"
    assert span_attr(spans[1], attrs.SESSION_ID) is None
