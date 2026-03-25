"""
CrewAI integration for AgentPulse.

Monkey-patches CrewAI at three levels:
  1. Crew.kickoff / Crew.kickoff_async  → root agent.handoff span + run_id pinning
  2. Agent.execute_task                 → per-agent agent.handoff span
  3. BaseTool._run / BaseTool._arun     → tool.call span (covers both sync and async tools)

ThreadPoolExecutor context propagation:
  CrewAI runs parallel tasks via ThreadPoolExecutor for Process.PARALLEL crews.
  Python's ThreadPoolExecutor does NOT inherit contextvars before Python 3.12.
  This integration captures copy_context() at kickoff time and wraps the
  executor submit calls so run_id, session_id, and user_id propagate into
  worker threads.

Usage::

    from agentpulse import init_tracer
    from agentpulse.integrations.crewai import instrument_crewai

    init_tracer()
    instrument_crewai()

    crew = Crew(agents=[...], tasks=[...])
    result = crew.kickoff()   # emits full span hierarchy automatically

Requires: pip install 'agentpulse[crewai]'
"""

from __future__ import annotations

import asyncio
import functools
import inspect
import logging
from contextvars import copy_context
from typing import Any, Optional

from opentelemetry import context as context_api, trace
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
    from crewai import Agent, Crew  # type: ignore[import]
    from crewai.tools import BaseTool  # type: ignore[import]
except ImportError as _exc:
    raise ImportError(
        "CrewAI integration requires crewai>=0.55. "
        "Install with: pip install 'agentpulse[crewai]'"
    ) from _exc

def _get_tracer() -> trace.Tracer:
    return trace.get_tracer("agentpulse.crewai")
_originals: dict[str, Any] = {}


# ── ContextVar-safe executor wrapper ─────────────────────────────────────────


def _ctx_run(fn: Any, *args: Any, **kwargs: Any) -> Any:
    """Run fn with the calling thread's contextvar state.

    Used to wrap tasks submitted to ThreadPoolExecutor so that run_id,
    session_id, and user_id inherited from the Crew.kickoff() thread are
    available inside each agent's worker thread.
    """
    ctx = copy_context()
    return ctx.run(fn, *args, **kwargs)


def _patch_executor(crew: Any) -> None:
    """Patch the crew's internal executor.submit to propagate contextvars.

    CrewAI uses a ThreadPoolExecutor internally for parallel task execution
    (Process.PARALLEL). We wrap submit() so each submitted callable runs
    inside a copy of the current context.
    """
    executor = getattr(crew, "_executor", None)
    if executor is None:
        return
    original_submit = executor.submit

    @functools.wraps(original_submit)
    def patched_submit(fn: Any, /, *args: Any, **kwargs: Any) -> Any:
        ctx = copy_context()
        return original_submit(ctx.run, fn, *args, **kwargs)

    executor.submit = patched_submit


# ── Crew.kickoff ──────────────────────────────────────────────────────────────


def _kickoff_wrapper(self: Any, original: Any, inputs: Optional[dict] = None) -> Any:
    """Sync kickoff wrapper — root span + run_id pinning."""
    crew_name = getattr(self, "name", None) or getattr(self, "id", "crew")
    run_id = get_run_id()  # auto-generate if not already set; pin it now so
    set_run_id(run_id)     # all child spans in sub-threads share the same run_id

    _patch_executor(self)

    span = _get_tracer().start_span(f"crew.{crew_name}")
    set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=str(crew_name))
    token = context_api.attach(trace.set_span_in_context(span))
    try:
        result = original(self) if inputs is None else original(self, inputs)
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


async def _kickoff_async_wrapper(self: Any, original: Any, inputs: Optional[dict] = None) -> Any:
    """Async kickoff wrapper — root span + run_id pinning."""
    crew_name = getattr(self, "name", None) or getattr(self, "id", "crew")
    run_id = get_run_id()
    set_run_id(run_id)

    span = _get_tracer().start_span(f"crew.{crew_name}")
    set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=str(crew_name))
    token = context_api.attach(trace.set_span_in_context(span))
    try:
        result = await original(self) if inputs is None else await original(self, inputs)
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


# ── Agent.execute_task ────────────────────────────────────────────────────────


def _execute_task_wrapper(self: Any, original: Any, task: Any, context: Any = None, tools: Any = None) -> Any:
    """Per-agent execution span with model info extracted from the agent's LLM."""
    agent_name = getattr(self, "role", getattr(self, "name", type(self).__name__))

    # Extract model from agent's LLM config
    llm = getattr(self, "llm", None)
    model = "unknown"
    if llm is not None:
        model = (
            getattr(llm, "model_name", None)
            or getattr(llm, "model", None)
            or getattr(llm, "deployment_name", None)
            or "unknown"
        )

    span = _get_tracer().start_span(f"agent.{agent_name}")
    set_common_attrs(
        span, attrs.AGENT_HANDOFF,
        agent_name=str(agent_name),
        extra={attrs.MODEL_ID: str(model)},
    )
    task_desc = getattr(task, "description", None)
    if task_desc:
        span.set_attribute(attrs.PROMPT, safe_truncate(str(task_desc)))

    token = context_api.attach(trace.set_span_in_context(span))
    try:
        kwargs: dict[str, Any] = {}
        if context is not None:
            kwargs["context"] = context
        if tools is not None:
            kwargs["tools"] = tools
        result = original(self, task, **kwargs)
        if result is not None:
            span.set_attribute(attrs.COMPLETION, safe_truncate(str(result)))
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


# ── BaseTool._run and BaseTool._arun ─────────────────────────────────────────


def _tool_run_wrapper(self: Any, original: Any, *args: Any, **kwargs: Any) -> Any:
    """Sync tool call span with LiteLLM-aware token extraction."""
    tool_name = getattr(self, "name", type(self).__name__)

    span = _get_tracer().start_span(f"tool.{tool_name}")
    set_common_attrs(
        span, attrs.TOOL_CALL,
        extra={attrs.TOOL_NAME: str(tool_name)},
    )
    # Capture tool input from first positional arg or kwargs
    tool_input = args[0] if args else kwargs.get("tool_input") or kwargs.get("input")
    if tool_input is not None:
        span.set_attribute("tool.input", safe_truncate(str(tool_input)))

    token = context_api.attach(trace.set_span_in_context(span))
    try:
        result = original(self, *args, **kwargs)
        if result is not None:
            span.set_attribute("tool.output", safe_truncate(str(result)))
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


async def _tool_arun_wrapper(self: Any, original: Any, *args: Any, **kwargs: Any) -> Any:
    """Async tool call span."""
    tool_name = getattr(self, "name", type(self).__name__)

    span = _get_tracer().start_span(f"tool.{tool_name}")
    set_common_attrs(
        span, attrs.TOOL_CALL,
        extra={attrs.TOOL_NAME: str(tool_name)},
    )
    tool_input = args[0] if args else kwargs.get("tool_input") or kwargs.get("input")
    if tool_input is not None:
        span.set_attribute("tool.input", safe_truncate(str(tool_input)))

    token = context_api.attach(trace.set_span_in_context(span))
    try:
        result = await original(self, *args, **kwargs)
        if result is not None:
            span.set_attribute("tool.output", safe_truncate(str(result)))
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


# ── Patch helpers ─────────────────────────────────────────────────────────────


def _make_sync_patch(original: Any, wrapper: Any) -> Any:
    @functools.wraps(original)
    def patched(self: Any, *args: Any, **kwargs: Any) -> Any:
        return wrapper(self, original, *args, **kwargs)
    return patched


def _make_async_patch(original: Any, wrapper: Any) -> Any:
    @functools.wraps(original)
    async def patched(self: Any, *args: Any, **kwargs: Any) -> Any:
        return await wrapper(self, original, *args, **kwargs)
    return patched


# ── Public API ────────────────────────────────────────────────────────────────


def instrument_crewai() -> None:
    """Instrument CrewAI by monkey-patching Crew, Agent, and BaseTool.

    Safe to call multiple times — second call is a no-op with a warning.

    Patches applied:
      - Crew.kickoff()       → root agent.handoff span + run_id pinning + executor ctx fix
      - Crew.kickoff_async() → async variant
      - Agent.execute_task() → per-agent agent.handoff span with model info
      - BaseTool._run()      → tool.call span (sync tools)
      - BaseTool._arun()     → tool.call span (async tools)
    """
    if is_instrumented(Crew):
        logger.warning("AgentPulse CrewAI: already instrumented; skipping")
        return

    # ── Crew.kickoff ──────────────────────────────────────────────────────────
    if hasattr(Crew, "kickoff"):
        _originals["Crew.kickoff"] = Crew.kickoff
        Crew.kickoff = _make_sync_patch(Crew.kickoff, _kickoff_wrapper)
    else:
        logger.warning("AgentPulse CrewAI: Crew.kickoff not found; check CrewAI version")

    if hasattr(Crew, "kickoff_async"):
        _originals["Crew.kickoff_async"] = Crew.kickoff_async
        Crew.kickoff_async = _make_async_patch(Crew.kickoff_async, _kickoff_async_wrapper)

    # ── Agent.execute_task ────────────────────────────────────────────────────
    if hasattr(Agent, "execute_task"):
        _originals["Agent.execute_task"] = Agent.execute_task
        Agent.execute_task = _make_sync_patch(Agent.execute_task, _execute_task_wrapper)
    else:
        logger.warning("AgentPulse CrewAI: Agent.execute_task not found; check CrewAI version")

    # ── BaseTool._run and _arun ───────────────────────────────────────────────
    if hasattr(BaseTool, "_run"):
        _originals["BaseTool._run"] = BaseTool._run
        BaseTool._run = _make_sync_patch(BaseTool._run, _tool_run_wrapper)
    else:
        logger.warning("AgentPulse CrewAI: BaseTool._run not found; tool spans unavailable")

    if hasattr(BaseTool, "_arun"):
        _originals["BaseTool._arun"] = BaseTool._arun
        BaseTool._arun = _make_async_patch(BaseTool._arun, _tool_arun_wrapper)

    mark_instrumented(Crew)
    logger.debug("AgentPulse CrewAI: instrumented Crew, Agent, BaseTool")


def uninstrument_crewai() -> None:
    """Remove AgentPulse patches from CrewAI classes.

    Restores Crew.kickoff, Crew.kickoff_async, Agent.execute_task,
    BaseTool._run, and BaseTool._arun to their original implementations.
    """
    restores = [
        (Crew, "kickoff", "Crew.kickoff"),
        (Crew, "kickoff_async", "Crew.kickoff_async"),
        (Agent, "execute_task", "Agent.execute_task"),
        (BaseTool, "_run", "BaseTool._run"),
        (BaseTool, "_arun", "BaseTool._arun"),
    ]
    for cls, attr, key in restores:
        if key in _originals:
            setattr(cls, attr, _originals.pop(key))

    # Remove the instrumented sentinel
    try:
        delattr(Crew, "_agentpulse_instrumented")
    except AttributeError:
        pass

    _originals.clear()
    logger.debug("AgentPulse CrewAI: uninstrumented")
