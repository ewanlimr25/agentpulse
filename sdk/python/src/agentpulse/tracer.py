"""
Tracer initialisation.

Call init_tracer() once at startup — it configures the global OTel TracerProvider
and returns a Tracer instance ready to use.
"""

from __future__ import annotations

import logging
from typing import Optional

from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SimpleSpanProcessor

from agentpulse import attributes as attrs
from agentpulse.config import AgentPulseConfig, load_config

logger = logging.getLogger(__name__)

# Module-level reference so shutdown() can reach the provider.
_provider: Optional[TracerProvider] = None


def init_tracer(config: Optional[AgentPulseConfig] = None) -> trace.Tracer:
    """Configure the global OTel TracerProvider and return an AgentPulse tracer.

    Call this once at application startup, before any instrumented code runs.
    Subsequent calls replace the existing provider — log a warning if this
    happens unintentionally.

    Args:
        config: Optional AgentPulseConfig. If None, settings are loaded from
                environment variables (AGENTPULSE_PROJECT_ID, etc.).

    Returns:
        An opentelemetry Tracer scoped to "agentpulse".

    Example::

        tracer = init_tracer()           # reads env vars
        tracer = init_tracer(AgentPulseConfig(project_id="my-project-uuid"))
    """
    global _provider

    if config is None:
        config = load_config()

    if _provider is not None:
        logger.warning(
            "agentpulse.init_tracer() called again — replacing existing TracerProvider. "
            "Call shutdown() first if this is intentional."
        )

    exporter = _build_exporter(config)

    resource = Resource.create(
        {
            "service.name": config.service_name,
            attrs.PROJECT_ID: config.project_id,
        }
    )

    provider = TracerProvider(resource=resource)

    processor_cls = BatchSpanProcessor if config.batch_export else SimpleSpanProcessor
    provider.add_span_processor(processor_cls(exporter))  # type: ignore[call-arg]

    trace.set_tracer_provider(provider)
    _provider = provider

    logger.debug(
        "AgentPulse tracer initialised (project=%s endpoint=%s protocol=%s)",
        config.project_id,
        config.resolved_endpoint,
        config.protocol,
    )

    return trace.get_tracer("agentpulse")


def shutdown() -> None:
    """Flush pending spans and shut down the TracerProvider.

    Call this at application exit to ensure all buffered spans are exported.
    """
    global _provider
    if _provider is not None:
        _provider.shutdown()
        _provider = None


def _build_exporter(config: AgentPulseConfig) -> object:
    endpoint = config.resolved_endpoint

    if config.protocol == "grpc":
        try:
            from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
        except ImportError as exc:
            raise ImportError(
                "gRPC exporter not installed. "
                "Run: pip install opentelemetry-exporter-otlp-proto-grpc"
            ) from exc
        return OTLPSpanExporter(
            endpoint=endpoint,
            insecure=config.insecure,
        )

    # HTTP
    try:
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter as HTTPExporter
    except ImportError as exc:
        raise ImportError(
            "HTTP exporter not installed. "
            "Run: pip install 'agentpulse[http]'"
        ) from exc
    return HTTPExporter(endpoint=endpoint)
