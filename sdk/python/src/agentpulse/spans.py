"""
Span kind decorators and context managers.

Each of the five AgentPulse span kinds has:
  - A decorator form:       @llm_call(model="gpt-4o")
  - A context manager form: with llm_call_ctx(model="gpt-4o") as span:

Both forms automatically:
  - Set agentpulse.span_kind to the correct value
  - Set agentpulse.project_id from context (evaluated at call time)
  - Set agentpulse.run_id (auto-generated per execution if not set, at call time)
  - Propagate parent_span_id via OTel context (this is how the DAG is built)
  - Set span status to ERROR if the wrapped function raises

Usage example::

    @llm_call(model="claude-sonnet-4-6", agent_name="ResearchAgent")
    def call_llm(prompt: str) -> str:
        result = my_llm_client.chat(prompt)
        record_llm_usage(trace.get_current_span(),
                         input_tokens=100, output_tokens=200,
                         prompt=prompt, completion=result)
        return result
"""

from __future__ import annotations

import functools
import inspect
import logging
from contextlib import contextmanager
from typing import Any, Callable, Generator, Optional, TypeVar

from opentelemetry import trace
from opentelemetry.trace import Span, StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import get_project_id, get_run_id, get_session_id, get_user_id

logger = logging.getLogger(__name__)

F = TypeVar("F", bound=Callable[..., Any])


# ── Internal helpers ───────────────────────────────────────────────────────────

def _get_tracer() -> trace.Tracer:
    return trace.get_tracer("agentpulse")


def _set_common_attrs(span: Span, span_kind: attrs.AgentSpanKind, agent_name: Optional[str]) -> None:
    """Set attributes that every span kind shares. Evaluated at call time."""
    span.set_attribute(attrs.SPAN_KIND, span_kind)
    span.set_attribute(attrs.RUN_ID, get_run_id())
    project_id = get_project_id()
    if project_id:
        span.set_attribute(attrs.PROJECT_ID, project_id)
    session_id = get_session_id()
    if session_id:
        span.set_attribute(attrs.SESSION_ID, session_id)
    user_id = get_user_id()
    if user_id:
        span.set_attribute(attrs.USER_ID, user_id)
    if agent_name:
        span.set_attribute(attrs.AGENT_NAME, agent_name)


def _make_wrapper(
    fn: F,
    span_name: str,
    span_kind: attrs.AgentSpanKind,
    extra_fn: Callable[[Span], None],
) -> F:
    """Build a sync or async wrapper that opens a span and applies extra_fn."""
    if inspect.iscoroutinefunction(fn):
        @functools.wraps(fn)
        async def async_wrapper(*args: Any, **kwargs: Any) -> Any:
            with _get_tracer().start_as_current_span(span_name) as span:
                extra_fn(span)
                try:
                    return await fn(*args, **kwargs)
                except Exception as exc:
                    span.set_status(StatusCode.ERROR, str(exc))
                    raise
        return async_wrapper  # type: ignore[return-value]

    @functools.wraps(fn)
    def sync_wrapper(*args: Any, **kwargs: Any) -> Any:
        with _get_tracer().start_as_current_span(span_name) as span:
            extra_fn(span)
            try:
                return fn(*args, **kwargs)
            except Exception as exc:
                span.set_status(StatusCode.ERROR, str(exc))
                raise
    return sync_wrapper  # type: ignore[return-value]


# ── LLM call ──────────────────────────────────────────────────────────────────

def llm_call(
    model: str,
    agent_name: Optional[str] = None,
    span_name: Optional[str] = None,
) -> Callable[[F], F]:
    """Decorator for LLM inference spans.

    Sets agentpulse.span_kind = "llm.call" and gen_ai.request.model.
    Use record_llm_usage() inside the decorated function to attach
    token counts, prompt, and completion text.

    Args:
        model: Model identifier, e.g. "claude-sonnet-4-6", "gpt-4o".
        agent_name: Name of the agent making this call.
        span_name: Override the span name (defaults to function name).
    """
    def decorator(fn: F) -> F:
        name = span_name or fn.__name__

        def apply_attrs(span: Span) -> None:
            _set_common_attrs(span, attrs.LLM_CALL, agent_name)
            span.set_attribute(attrs.MODEL_ID, model)

        return _make_wrapper(fn, name, attrs.LLM_CALL, apply_attrs)
    return decorator


@contextmanager
def llm_call_ctx(
    model: str,
    agent_name: Optional[str] = None,
    span_name: str = "llm.call",
) -> Generator[Span, None, None]:
    """Context manager form of llm_call.

    Example::

        with llm_call_ctx(model="gpt-4o", agent_name="Assistant") as span:
            result = client.chat(prompt)
            record_llm_usage(span, input_tokens=50, output_tokens=100,
                             prompt=prompt, completion=result)
    """
    with _get_tracer().start_as_current_span(span_name) as span:
        _set_common_attrs(span, attrs.LLM_CALL, agent_name)
        span.set_attribute(attrs.MODEL_ID, model)
        try:
            yield span
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise


# ── Tool call ─────────────────────────────────────────────────────────────────

def tool_call(
    tool_name: str,
    agent_name: Optional[str] = None,
    span_name: Optional[str] = None,
) -> Callable[[F], F]:
    """Decorator for tool invocation spans.

    Sets agentpulse.span_kind = "tool.call" and tool.name.

    Args:
        tool_name: Name of the tool being called, e.g. "web_search", "code_exec".
        agent_name: Name of the agent invoking this tool.
        span_name: Override the span name (defaults to function name).
    """
    def decorator(fn: F) -> F:
        name = span_name or fn.__name__

        def apply_attrs(span: Span) -> None:
            _set_common_attrs(span, attrs.TOOL_CALL, agent_name)
            span.set_attribute(attrs.TOOL_NAME, tool_name)

        return _make_wrapper(fn, name, attrs.TOOL_CALL, apply_attrs)
    return decorator


@contextmanager
def tool_call_ctx(
    tool_name: str,
    agent_name: Optional[str] = None,
    span_name: str = "tool.call",
) -> Generator[Span, None, None]:
    """Context manager form of tool_call."""
    with _get_tracer().start_as_current_span(span_name) as span:
        _set_common_attrs(span, attrs.TOOL_CALL, agent_name)
        span.set_attribute(attrs.TOOL_NAME, tool_name)
        try:
            yield span
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise


# ── Agent handoff ─────────────────────────────────────────────────────────────

def handoff(
    target_agent: str,
    agent_name: Optional[str] = None,
    span_name: Optional[str] = None,
) -> Callable[[F], F]:
    """Decorator for agent-to-agent handoff spans.

    Sets agentpulse.span_kind = "agent.handoff". Child spans created inside
    the decorated function will have this span as their parent — this is how
    the topology DAG builds the handoff edge A → B.

    Args:
        target_agent: Name of the agent receiving control.
        agent_name: Name of the agent handing off.
        span_name: Override the span name (defaults to function name).
    """
    def decorator(fn: F) -> F:
        name = span_name or fn.__name__

        def apply_attrs(span: Span) -> None:
            _set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name)
            span.set_attribute(attrs.HANDOFF_TARGET, target_agent)

        return _make_wrapper(fn, name, attrs.AGENT_HANDOFF, apply_attrs)
    return decorator


@contextmanager
def handoff_ctx(
    target_agent: str,
    agent_name: Optional[str] = None,
    span_name: str = "agent.handoff",
) -> Generator[Span, None, None]:
    """Context manager form of handoff."""
    with _get_tracer().start_as_current_span(span_name) as span:
        _set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name)
        span.set_attribute(attrs.HANDOFF_TARGET, target_agent)
        try:
            yield span
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise


# ── Memory read ───────────────────────────────────────────────────────────────

def memory_read(
    key: Optional[str] = None,
    agent_name: Optional[str] = None,
    span_name: Optional[str] = None,
) -> Callable[[F], F]:
    """Decorator for memory read spans."""
    def decorator(fn: F) -> F:
        name = span_name or fn.__name__

        def apply_attrs(span: Span) -> None:
            _set_common_attrs(span, attrs.MEMORY_READ, agent_name)
            if key:
                span.set_attribute(attrs.MEMORY_KEY, key)

        return _make_wrapper(fn, name, attrs.MEMORY_READ, apply_attrs)
    return decorator


@contextmanager
def memory_read_ctx(
    key: Optional[str] = None,
    agent_name: Optional[str] = None,
    span_name: str = "memory.read",
) -> Generator[Span, None, None]:
    """Context manager form of memory_read."""
    with _get_tracer().start_as_current_span(span_name) as span:
        _set_common_attrs(span, attrs.MEMORY_READ, agent_name)
        if key:
            span.set_attribute(attrs.MEMORY_KEY, key)
        try:
            yield span
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise


# ── Memory write ──────────────────────────────────────────────────────────────

def memory_write(
    key: Optional[str] = None,
    agent_name: Optional[str] = None,
    span_name: Optional[str] = None,
) -> Callable[[F], F]:
    """Decorator for memory write spans."""
    def decorator(fn: F) -> F:
        name = span_name or fn.__name__

        def apply_attrs(span: Span) -> None:
            _set_common_attrs(span, attrs.MEMORY_WRITE, agent_name)
            if key:
                span.set_attribute(attrs.MEMORY_KEY, key)

        return _make_wrapper(fn, name, attrs.MEMORY_WRITE, apply_attrs)
    return decorator


@contextmanager
def memory_write_ctx(
    key: Optional[str] = None,
    agent_name: Optional[str] = None,
    span_name: str = "memory.write",
) -> Generator[Span, None, None]:
    """Context manager form of memory_write."""
    with _get_tracer().start_as_current_span(span_name) as span:
        _set_common_attrs(span, attrs.MEMORY_WRITE, agent_name)
        if key:
            span.set_attribute(attrs.MEMORY_KEY, key)
        try:
            yield span
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise


# ── Usage helper ──────────────────────────────────────────────────────────────

def record_llm_usage(
    span: Span,
    input_tokens: int,
    output_tokens: int,
    prompt: Optional[str] = None,
    completion: Optional[str] = None,
    cost_usd: Optional[float] = None,
) -> None:
    """Attach LLM usage data to an active span.

    Call this inside an @llm_call decorated function (or llm_call_ctx) after
    the LLM returns to record token counts and optionally the prompt/completion
    text. Token counts are required for cost computation in the collector.

    Args:
        span: The active span (use trace.get_current_span() or the ctx manager yield).
        input_tokens: Number of tokens in the prompt/input.
        output_tokens: Number of tokens in the completion/output.
        prompt: Full prompt text (used for eval quality scoring).
        completion: Full completion text (used for eval quality scoring).
        cost_usd: Explicit cost in USD. If omitted, the collector computes it
                  from token counts and the model pricing table.
    """
    span.set_attribute(attrs.INPUT_TOKENS, input_tokens)
    span.set_attribute(attrs.OUTPUT_TOKENS, output_tokens)
    if prompt is not None:
        span.set_attribute(attrs.PROMPT, prompt)
    if completion is not None:
        span.set_attribute(attrs.COMPLETION, completion)
    if cost_usd is not None:
        span.set_attribute(attrs.COST_USD, cost_usd)
