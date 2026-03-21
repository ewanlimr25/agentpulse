"""Shared pytest fixtures for AgentPulse SDK tests."""

from __future__ import annotations

import pytest
import opentelemetry.trace as _trace_api
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.sdk.trace.export import SimpleSpanProcessor

from agentpulse._context import reset_run, set_project_id


def _force_set_tracer_provider(provider: TracerProvider) -> None:
    """Force-install a TracerProvider, bypassing OTel's one-time guard.

    OTel uses a sync.Once-style guard (_TRACER_PROVIDER_SET_ONCE) to prevent
    accidental replacement of the global provider at runtime. Tests need to
    swap it per-test. We reset both the guard and the global variable directly.
    This mirrors the approach used by OTel's own test suite.
    """
    # Reset the Once guard so set_tracer_provider will accept a new value
    _trace_api._TRACER_PROVIDER_SET_ONCE._done = False  # type: ignore[attr-defined]
    _trace_api._TRACER_PROVIDER = None  # type: ignore[attr-defined]
    _trace_api.set_tracer_provider(provider)


@pytest.fixture(autouse=True)
def reset_otel():
    """Provide a fresh InMemorySpanExporter and reset OTel + agentpulse context between tests."""
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    _force_set_tracer_provider(provider)

    # Inject a stable project_id into context for all tests
    set_project_id("test-project-id")
    reset_run()

    yield exporter

    exporter.clear()
