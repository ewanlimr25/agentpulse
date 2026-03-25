"""
Shared helpers for all AgentPulse framework integrations.

All integration modules should use these helpers instead of calling span
attribute setters directly. This ensures:
  - session_id and user_id are always propagated (the original langchain/openai
    integrations were missing both)
  - The context_api.attach/detach pattern is used consistently — the old
    trace.use_span().__enter__() pattern leaks context if __exit__ is never called
  - Token extraction works across LiteLLM provider field name aliases
"""

from __future__ import annotations

import logging
from typing import Any, Optional

from opentelemetry import context as context_api, trace
from opentelemetry.trace import Span, StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import (
    get_project_id,
    get_run_id,
    get_session_id,
    get_user_id,
)

logger = logging.getLogger(__name__)

# Max characters to store for prompt/completion text in span attributes.
# Matches the collector-side truncation limit.
_TEXT_MAX = 4000

_SENTINEL = "_agentpulse_instrumented"


# ── Text helpers ──────────────────────────────────────────────────────────────


def safe_truncate(text: str, max_len: int = _TEXT_MAX) -> str:
    """Truncate text to max_len characters. Safe to call on non-string values."""
    if not isinstance(text, str):
        try:
            text = str(text)
        except Exception:
            return ""
    return text if len(text) <= max_len else text[:max_len]


# ── Span lifecycle ────────────────────────────────────────────────────────────


def set_common_attrs(
    span: Span,
    span_kind: str,
    agent_name: Optional[str] = None,
    extra: Optional[dict[str, Any]] = None,
) -> None:
    """Set the standard attributes that every AgentPulse span carries.

    This is the integration-facing version of spans._set_common_attrs and
    is kept separate so integrations don't import private SDK internals.
    """
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
    if extra:
        for k, v in extra.items():
            if v is not None:
                span.set_attribute(k, str(v) if not isinstance(v, (bool, int, float, str)) else v)


def start_span(
    tracer: trace.Tracer,
    span_name: str,
    span_kind: str,
    agent_name: Optional[str] = None,
    extra: Optional[dict[str, Any]] = None,
) -> tuple[Span, object]:
    """Start a span, set common attributes, and attach it to the OTel context.

    Returns (span, context_token). The caller MUST call::

        context_api.detach(token)
        span.end()

    in a ``finally`` block to guarantee cleanup. This is the correct pattern
    for callback-based integrations where start and end happen in different
    call frames (unlike start_as_current_span() which only works in a single
    with-block).

    Example::

        span, token = start_span(tracer, "llm.call", attrs.LLM_CALL, agent_name="bot")
        try:
            result = original_fn(...)
            span.set_attribute(attrs.COMPLETION, safe_truncate(result))
        except Exception as exc:
            span.set_status(StatusCode.ERROR, str(exc))
            raise
        finally:
            context_api.detach(token)
            span.end()
    """
    span = tracer.start_span(span_name)
    set_common_attrs(span, span_kind, agent_name, extra)
    token = context_api.attach(trace.set_span_in_context(span))
    return span, token


# ── Double-instrumentation guard ──────────────────────────────────────────────


def is_instrumented(obj: Any) -> bool:
    """Return True if obj has already been instrumented by AgentPulse."""
    return bool(getattr(obj, _SENTINEL, False))


def mark_instrumented(obj: Any) -> None:
    """Mark obj as instrumented so a second call is a no-op with a warning."""
    try:
        setattr(obj, _SENTINEL, True)
    except (AttributeError, TypeError):
        # Some objects (e.g. frozen dataclasses) don't support setattr.
        pass


# ── Token extraction ──────────────────────────────────────────────────────────


def extract_usage(usage: Any) -> tuple[int, int]:
    """Extract (input_tokens, output_tokens) from a usage object.

    Handles LiteLLM/OpenAI field name aliases:
      - OpenAI:    prompt_tokens    / completion_tokens
      - Anthropic: input_tokens     / output_tokens
    Returns (0, 0) when usage is None or fields are absent.
    """
    if usage is None:
        return 0, 0
    input_tokens = int(
        getattr(usage, "prompt_tokens", None)
        or getattr(usage, "input_tokens", None)
        or 0
    )
    output_tokens = int(
        getattr(usage, "completion_tokens", None)
        or getattr(usage, "output_tokens", None)
        or 0
    )
    return input_tokens, output_tokens


def record_usage_from_response(span: Span, response: Any, warn_on_zero: bool = True) -> None:
    """Extract and attach token usage from an LLM response to the span.

    Warns when both token counts are zero and it looks like the response
    was streaming (where usage may be unavailable in the returned object).
    """
    usage = getattr(response, "usage", None)
    input_tokens, output_tokens = extract_usage(usage)

    if input_tokens:
        span.set_attribute(attrs.INPUT_TOKENS, input_tokens)
    if output_tokens:
        span.set_attribute(attrs.OUTPUT_TOKENS, output_tokens)

    if warn_on_zero and input_tokens == 0 and output_tokens == 0:
        if getattr(response, "stream", False):
            span.add_event(
                "token_usage_unavailable",
                attributes={"reason": "streaming_response"},
            )
            logger.debug(
                "AgentPulse: token usage unavailable for streaming response (%s)",
                type(response).__name__,
            )
        elif usage is not None:
            logger.warning(
                "AgentPulse: could not extract token usage from %s (got usage=%r)",
                type(response).__name__,
                usage,
            )
