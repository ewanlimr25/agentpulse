"""
AgentPulse SDK configuration.

All settings can be supplied programmatically or via environment variables.
Environment variables take precedence over dataclass defaults.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import Literal


@dataclass(frozen=True)
class AgentPulseConfig:
    """Immutable configuration for the AgentPulse SDK.

    Args:
        project_id: AgentPulse project UUID. Required. Used to scope all spans
            to the correct tenant in the collector.
        endpoint: OTLP exporter endpoint. Defaults to localhost:4317 (gRPC) or
            http://localhost:4318 (HTTP). Set AGENTPULSE_ENDPOINT to override.
        service_name: OTel resource service.name attribute. Appears in traces.
        protocol: Transport protocol — "grpc" (default) or "http".
            Set AGENTPULSE_PROTOCOL to override.
        insecure: Disable TLS. True by default for local development.
        batch_export: Use BatchSpanProcessor (True) or SimpleSpanProcessor (False).
            Batch is recommended for production; Simple is useful for testing.
        export_timeout_ms: Max time to wait for a batch export to complete.
    """

    project_id: str
    endpoint: str = field(default_factory=lambda: os.getenv("AGENTPULSE_ENDPOINT", ""))
    service_name: str = field(default_factory=lambda: os.getenv("AGENTPULSE_SERVICE", "agentpulse-agent"))
    protocol: Literal["grpc", "http"] = field(
        default_factory=lambda: os.getenv("AGENTPULSE_PROTOCOL", "grpc")  # type: ignore[return-value]
    )
    insecure: bool = True
    batch_export: bool = True
    export_timeout_ms: int = 30_000

    def __post_init__(self) -> None:
        if not self.project_id:
            raise ValueError(
                "AgentPulse project_id is required. "
                "Pass it to AgentPulseConfig(project_id=...) or set AGENTPULSE_PROJECT_ID."
            )
        if self.protocol not in ("grpc", "http"):
            raise ValueError(f"protocol must be 'grpc' or 'http', got {self.protocol!r}")

    @property
    def resolved_endpoint(self) -> str:
        if self.endpoint:
            return self.endpoint
        if self.protocol == "grpc":
            return "localhost:4317"
        return "http://localhost:4318"


def load_config(**overrides: object) -> AgentPulseConfig:
    """Build an AgentPulseConfig from environment variables with optional overrides.

    Environment variables:
        AGENTPULSE_PROJECT_ID   Required
        AGENTPULSE_ENDPOINT     Optional
        AGENTPULSE_SERVICE      Optional (default: "agentpulse-agent")
        AGENTPULSE_PROTOCOL     Optional ("grpc" or "http", default: "grpc")
    """
    project_id = overrides.pop("project_id", None) or os.getenv("AGENTPULSE_PROJECT_ID", "")
    return AgentPulseConfig(project_id=str(project_id), **overrides)  # type: ignore[arg-type]
