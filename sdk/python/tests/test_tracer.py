"""Tests for init_tracer() and shutdown()."""

from __future__ import annotations

import pytest
from unittest.mock import MagicMock, patch

from agentpulse.config import AgentPulseConfig
from agentpulse.tracer import init_tracer, shutdown, _build_exporter
from agentpulse import attributes as attrs


def test_init_tracer_returns_tracer(reset_otel):
    """init_tracer returns a functional Tracer instance."""
    cfg = AgentPulseConfig(project_id="proj-123", protocol="grpc")
    # Patch the gRPC exporter so we don't need a real collector
    with patch("agentpulse.tracer._build_exporter") as mock_build:
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        from opentelemetry.sdk.trace.export import SimpleSpanProcessor
        mock_exporter = InMemorySpanExporter()
        mock_build.return_value = mock_exporter

        tracer = init_tracer(cfg)
        assert tracer is not None


def test_init_tracer_sets_global_provider(reset_otel):
    """init_tracer sets the global OTel TracerProvider."""
    from opentelemetry import trace

    cfg = AgentPulseConfig(project_id="proj-xyz")
    with patch("agentpulse.tracer._build_exporter") as mock_build:
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        mock_build.return_value = InMemorySpanExporter()
        init_tracer(cfg)

    from opentelemetry.sdk.trace import TracerProvider
    assert isinstance(trace.get_tracer_provider(), TracerProvider)


def test_init_tracer_loads_config_from_env(monkeypatch, reset_otel):
    """init_tracer() with no args reads from env vars."""
    monkeypatch.setenv("AGENTPULSE_PROJECT_ID", "env-project")
    with patch("agentpulse.tracer._build_exporter") as mock_build:
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        mock_build.return_value = InMemorySpanExporter()
        tracer = init_tracer()
        assert tracer is not None


def test_init_tracer_without_project_id_raises(monkeypatch, reset_otel):
    """init_tracer() raises ValueError if AGENTPULSE_PROJECT_ID is not set."""
    monkeypatch.delenv("AGENTPULSE_PROJECT_ID", raising=False)
    with pytest.raises(ValueError, match="project_id is required"):
        init_tracer()


def test_build_exporter_grpc():
    """_build_exporter returns a gRPC exporter for protocol='grpc'."""
    cfg = AgentPulseConfig(project_id="x", protocol="grpc", endpoint="localhost:4317")
    grpc_path = "opentelemetry.exporter.otlp.proto.grpc.trace_exporter.OTLPSpanExporter"
    with patch(grpc_path) as mock_cls:
        mock_cls.return_value = MagicMock()
        exporter = _build_exporter(cfg)
        mock_cls.assert_called_once_with(endpoint="localhost:4317", insecure=True)
        assert exporter is mock_cls.return_value


def test_build_exporter_http():
    """_build_exporter returns an HTTP exporter for protocol='http'."""
    cfg = AgentPulseConfig(project_id="x", protocol="http", endpoint="http://localhost:4318")
    http_path = "opentelemetry.exporter.otlp.proto.http.trace_exporter.OTLPSpanExporter"
    with patch(http_path) as mock_cls:
        mock_cls.return_value = MagicMock()
        exporter = _build_exporter(cfg)
        mock_cls.assert_called_once_with(endpoint="http://localhost:4318")
        assert exporter is mock_cls.return_value


def test_shutdown_clears_provider(reset_otel):
    """shutdown() clears the internal provider reference."""
    from agentpulse import tracer as tracer_module

    cfg = AgentPulseConfig(project_id="proj-shutdown")
    with patch("agentpulse.tracer._build_exporter") as mock_build:
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        mock_build.return_value = InMemorySpanExporter()
        init_tracer(cfg)

    assert tracer_module._provider is not None
    shutdown()
    assert tracer_module._provider is None
