"""
OpenAI Agents SDK integration for AgentPulse.

Uses the OpenAI Agents SDK's native RunHooks / AgentHooks system — the
explicitly documented stable observability surface — rather than monkey-patching
Runner.run(). This produces per-tool and per-handoff spans, not just one root span.

Usage::

    from agents import Agent, Runner
    from agentpulse import init_tracer, set_run_id
    from agentpulse.integrations.openai_agents import instrument_openai_agents

    init_tracer()
    set_run_id("my-run-id")  # optional; auto-generated if omitted
    hooks = instrument_openai_agents()

    result = await Runner.run(agent, "your task here", hooks=hooks)

Backward compatibility::

    # The old instrument_runner(runner) API still works but is deprecated.
    from agentpulse.integrations.openai_agents import instrument_runner
    runner = instrument_runner(Runner())
    result = await runner.run(agent, "task")

Requires: pip install 'agentpulse[openai]'

Note: The OpenAI Agents SDK is pre-1.0. RunHooks is the stable API surface;
pin your openai-agents version if you observe attribute changes after upgrades.
"""

from __future__ import annotations

import logging
from typing import Any

from opentelemetry import trace
from opentelemetry.trace import StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import get_run_id, set_run_id
from agentpulse.integrations._base import (
    is_instrumented,
    mark_instrumented,
    record_usage_from_response,
    safe_truncate,
    set_common_attrs,
)

logger = logging.getLogger(__name__)

try:
    from agents import RunHooks, RunContextWrapper  # type: ignore[import]
except ImportError as _exc:
    raise ImportError(
        "OpenAI Agents integration requires openai-agents. "
        "Install with: pip install 'agentpulse[openai]'"
    ) from _exc


def _get_tracer() -> trace.Tracer:
    return trace.get_tracer("agentpulse.openai_agents")

# span_key → (span, context_token); used for tools/handoffs started in hooks
_active: dict[str, tuple[Any, object]] = {}


def _agent_name(agent: Any) -> str:
    return getattr(agent, "name", type(agent).__name__)


class AgentPulseRunHooks(RunHooks):
    """RunHooks implementation that emits AgentPulse spans.

    Pass an instance to Runner.run(agent, task, hooks=AgentPulseRunHooks()).

    Span hierarchy per run:
        agent.handoff (root — on_agent_start / on_agent_end)
          └─ tool.call    (on_tool_start / on_tool_end)
          └─ agent.handoff (on_handoff, for each sub-agent transfer)
    """

    def __init__(self) -> None:
        # Keyed by agent id so concurrent agents in the same run don't collide.
        self._agent_spans: dict[int, tuple[Any, object]] = {}
        self._tool_spans: dict[str, tuple[Any, object]] = {}

    async def on_agent_start(
        self, context: RunContextWrapper, agent: Any
    ) -> None:
        from opentelemetry import context as context_api

        name = _agent_name(agent)
        # Ensure run_id is pinned before any child span fires.
        get_run_id()  # triggers auto-generate if not already set

        span = _get_tracer().start_span(f"agent.{name}")
        set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=name)
        token = context_api.attach(trace.set_span_in_context(span))
        self._agent_spans[id(agent)] = (span, token)

    async def on_agent_end(
        self, context: RunContextWrapper, agent: Any, output: Any
    ) -> None:
        from opentelemetry import context as context_api

        entry = self._agent_spans.pop(id(agent), None)
        if entry is None:
            return
        span, token = entry
        if output is not None:
            try:
                span.set_attribute(attrs.COMPLETION, safe_truncate(str(output)))
            except Exception:
                pass
        context_api.detach(token)
        span.end()

    async def on_tool_start(
        self, context: RunContextWrapper, agent: Any, tool: Any
    ) -> None:
        from opentelemetry import context as context_api

        tool_name = getattr(tool, "name", type(tool).__name__)
        key = f"{id(agent)}.{tool_name}"
        span = _get_tracer().start_span(f"tool.{tool_name}")
        set_common_attrs(
            span,
            attrs.TOOL_CALL,
            agent_name=_agent_name(agent),
            extra={attrs.TOOL_NAME: tool_name},
        )
        token = context_api.attach(trace.set_span_in_context(span))
        self._tool_spans[key] = (span, token)

    async def on_tool_end(
        self, context: RunContextWrapper, agent: Any, tool: Any, result: Any
    ) -> None:
        from opentelemetry import context as context_api

        tool_name = getattr(tool, "name", type(tool).__name__)
        key = f"{id(agent)}.{tool_name}"
        entry = self._tool_spans.pop(key, None)
        if entry is None:
            return
        span, token = entry
        if result is not None:
            try:
                span.set_attribute("tool.output", safe_truncate(str(result)))
            except Exception:
                pass
        context_api.detach(token)
        span.end()

    async def on_handoff(
        self, context: RunContextWrapper, from_agent: Any, to_agent: Any
    ) -> None:
        """Emit a point-in-time handoff event on the current agent's span."""
        from_name = _agent_name(from_agent)
        to_name = _agent_name(to_agent)
        entry = self._agent_spans.get(id(from_agent))
        if entry:
            span, _ = entry
            span.add_event(
                "agent.handoff",
                attributes={
                    attrs.AGENT_NAME: from_name,
                    attrs.HANDOFF_TARGET: to_name,
                },
            )
        logger.debug("AgentPulse: handoff %s → %s", from_name, to_name)


def instrument_openai_agents() -> AgentPulseRunHooks:
    """Create and return an AgentPulseRunHooks instance.

    Pass the returned hooks to Runner.run(agent, task, hooks=hooks).

    Example::

        hooks = instrument_openai_agents()
        result = await Runner.run(my_agent, "task", hooks=hooks)
    """
    return AgentPulseRunHooks()


# ── Backward-compatible monkey-patch API ──────────────────────────────────────


def instrument_runner(runner: Any) -> Any:
    """Deprecated: wrap an OpenAI Agents Runner with AgentPulse tracing.

    The RunHooks API is preferred. This function remains for backward
    compatibility but only emits a single root span per run() call,
    not per-tool or per-handoff spans.

    Use instrument_openai_agents() for the full span hierarchy.
    """
    if is_instrumented(runner):
        logger.warning(
            "AgentPulse: Runner is already instrumented; skipping. "
            "Use instrument_openai_agents() and pass hooks= to Runner.run() instead."
        )
        return runner

    original_run = runner.run
    original_run_sync = getattr(runner, "run_sync", None)
    root_tracer = _get_tracer()

    async def traced_run(agent: Any, task: str, **kwargs: Any) -> Any:
        from opentelemetry import context as context_api

        name = _agent_name(agent)
        get_run_id()  # ensure run_id is pinned before span starts
        span = root_tracer.start_span(f"agent.{name}")
        set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=name)
        token = context_api.attach(trace.set_span_in_context(span))
        try:
            result = await original_run(agent, task, **kwargs)
            resp = getattr(result, "raw_responses", [None])[0] if result else None
            if resp:
                record_usage_from_response(span, resp)
            return result
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise
        finally:
            context_api.detach(token)
            span.end()

    runner.run = traced_run

    if original_run_sync is not None:
        import asyncio

        def traced_run_sync(agent: Any, task: str, **kwargs: Any) -> Any:
            return asyncio.get_event_loop().run_until_complete(
                traced_run(agent, task, **kwargs)
            )

        runner.run_sync = traced_run_sync

    mark_instrumented(runner)
    return runner


def uninstrument_runner(runner: Any) -> None:
    """Remove AgentPulse instrumentation from a Runner instance."""
    try:
        delattr(runner, "_agentpulse_instrumented")
        delattr(runner, "run")
        delattr(runner, "run_sync")
    except AttributeError:
        pass
