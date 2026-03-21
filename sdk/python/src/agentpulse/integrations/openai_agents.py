"""
OpenAI Agents SDK integration for AgentPulse.

Wraps OpenAI's agents.Runner to automatically emit AgentPulse spans
for each agent invocation, tool call, and handoff.

Usage::

    from agents import Agent, Runner
    from agentpulse import init_tracer
    from agentpulse.integrations.openai_agents import instrument_runner

    init_tracer()
    runner = instrument_runner(Runner())

    result = await runner.run(agent, "your task here")

Requires: pip install 'agentpulse[openai]'

Note: The OpenAI Agents SDK tracing API is still evolving. This integration
targets openai>=1.30 with the agents module. Pin your openai version if you
observe attribute mapping issues after upgrades.
"""

from __future__ import annotations

import logging
from typing import Any

from opentelemetry import trace
from opentelemetry.trace import StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import get_project_id, get_run_id, set_run_id

logger = logging.getLogger(__name__)

try:
    import openai  # noqa: F401
except ImportError as _exc:
    raise ImportError(
        "OpenAI Agents integration requires openai>=1.30. "
        "Install with: pip install 'agentpulse[openai]'"
    ) from _exc


def instrument_runner(runner: Any) -> Any:
    """Wrap an OpenAI Agents Runner to emit AgentPulse spans.

    Returns the same runner instance with tracing hooks applied.
    Uses monkey-patching to intercept the run() and run_sync() methods.

    Args:
        runner: An openai agents Runner instance.

    Returns:
        The same runner with AgentPulse tracing applied.
    """
    tracer = trace.get_tracer("agentpulse.openai_agents")
    original_run = runner.run
    original_run_sync = getattr(runner, "run_sync", None)

    async def traced_run(agent: Any, task: str, **kwargs: Any) -> Any:
        agent_name = getattr(agent, "name", type(agent).__name__)
        with tracer.start_as_current_span(f"agent.{agent_name}") as span:
            span.set_attribute(attrs.SPAN_KIND, attrs.AGENT_HANDOFF)
            span.set_attribute(attrs.AGENT_NAME, agent_name)
            span.set_attribute(attrs.RUN_ID, get_run_id())
            project_id = get_project_id()
            if project_id:
                span.set_attribute(attrs.PROJECT_ID, project_id)
            try:
                return await original_run(agent, task, **kwargs)
            except Exception as exc:
                span.set_status(StatusCode.ERROR, str(exc))
                raise

    runner.run = traced_run

    if original_run_sync is not None:
        def traced_run_sync(agent: Any, task: str, **kwargs: Any) -> Any:
            agent_name = getattr(agent, "name", type(agent).__name__)
            with tracer.start_as_current_span(f"agent.{agent_name}") as span:
                span.set_attribute(attrs.SPAN_KIND, attrs.AGENT_HANDOFF)
                span.set_attribute(attrs.AGENT_NAME, agent_name)
                span.set_attribute(attrs.RUN_ID, get_run_id())
                project_id = get_project_id()
                if project_id:
                    span.set_attribute(attrs.PROJECT_ID, project_id)
                try:
                    return original_run_sync(agent, task, **kwargs)
                except Exception as exc:
                    span.set_status(StatusCode.ERROR, str(exc))
                    raise
        runner.run_sync = traced_run_sync

    return runner
