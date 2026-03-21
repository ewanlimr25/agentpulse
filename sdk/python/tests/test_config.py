"""Tests for AgentPulseConfig and load_config."""

import os
import pytest

from agentpulse.config import AgentPulseConfig, load_config


def test_config_requires_project_id():
    with pytest.raises(ValueError, match="project_id is required"):
        AgentPulseConfig(project_id="")


def test_config_invalid_protocol():
    with pytest.raises(ValueError, match="protocol must be"):
        AgentPulseConfig(project_id="test", protocol="websocket")  # type: ignore


def test_config_defaults():
    cfg = AgentPulseConfig(project_id="abc-123")
    assert cfg.project_id == "abc-123"
    assert cfg.protocol == "grpc"
    assert cfg.insecure is True
    assert cfg.batch_export is True
    assert cfg.service_name == "agentpulse-agent"


def test_config_resolved_endpoint_grpc_default():
    cfg = AgentPulseConfig(project_id="x", endpoint="")
    assert cfg.resolved_endpoint == "localhost:4317"


def test_config_resolved_endpoint_http_default():
    cfg = AgentPulseConfig(project_id="x", endpoint="", protocol="http")
    assert cfg.resolved_endpoint == "http://localhost:4318"


def test_config_explicit_endpoint_overrides():
    cfg = AgentPulseConfig(project_id="x", endpoint="collector:4317")
    assert cfg.resolved_endpoint == "collector:4317"


def test_load_config_from_env(monkeypatch):
    monkeypatch.setenv("AGENTPULSE_PROJECT_ID", "env-project-id")
    monkeypatch.setenv("AGENTPULSE_SERVICE", "my-agent")
    monkeypatch.setenv("AGENTPULSE_PROTOCOL", "http")

    cfg = load_config()

    assert cfg.project_id == "env-project-id"
    assert cfg.service_name == "my-agent"
    assert cfg.protocol == "http"


def test_load_config_override_takes_precedence(monkeypatch):
    monkeypatch.setenv("AGENTPULSE_PROJECT_ID", "from-env")
    cfg = load_config(project_id="from-arg")
    assert cfg.project_id == "from-arg"


def test_load_config_missing_project_id(monkeypatch):
    monkeypatch.delenv("AGENTPULSE_PROJECT_ID", raising=False)
    with pytest.raises(ValueError):
        load_config()
