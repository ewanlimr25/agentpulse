"""
Agent Replay / Sandbox Debugging — SDK side.

Loads a replay bundle (from a backend run, or a local JSON file) and
re-executes the user's instrumented code with mocked LLM/tool outputs
sourced from the recorded spans. See docs/roadmap.md §H.

Usage::

    from agentpulse import replay

    bundle = replay.load_bundle("a1b2c3...run_id")
    with replay.ReplayEngine(bundle, overrides={"web_search": "FAKE"}):
        run_pipeline(task)  # user's existing code, unchanged
"""

from __future__ import annotations

import json
import logging
import os
import urllib.request
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Optional

from opentelemetry import trace

from agentpulse import attributes as attrs
from agentpulse import spans as _spans_module

logger = logging.getLogger(__name__)


# ── Wire format ───────────────────────────────────────────────────────────────


@dataclass
class ReplaySpan:
    SpanID: str = ""
    ParentSpanID: str = ""
    AgentSpanKind: str = ""
    AgentName: str = ""
    SpanName: str = ""
    ModelID: str = ""
    ToolName: str = ""
    CallIndex: int = 0
    StatusCode: str = ""
    StatusMessage: str = ""
    Inputs: dict[str, str] = field(default_factory=dict)
    Outputs: dict[str, str] = field(default_factory=dict)
    InputTokens: int = 0
    OutputTokens: int = 0

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "ReplaySpan":
        return cls(
            SpanID=data.get("SpanID", ""),
            ParentSpanID=data.get("ParentSpanID", ""),
            AgentSpanKind=data.get("AgentSpanKind", ""),
            AgentName=data.get("AgentName", ""),
            SpanName=data.get("SpanName", ""),
            ModelID=data.get("ModelID", ""),
            ToolName=data.get("ToolName", ""),
            CallIndex=int(data.get("CallIndex", 0) or 0),
            StatusCode=data.get("StatusCode", ""),
            StatusMessage=data.get("StatusMessage", ""),
            Inputs=dict(data.get("Inputs") or {}),
            Outputs=dict(data.get("Outputs") or {}),
            InputTokens=int(data.get("InputTokens", 0) or 0),
            OutputTokens=int(data.get("OutputTokens", 0) or 0),
        )


@dataclass
class ReplayBundle:
    SchemaVersion: int = 1
    Run: dict[str, Any] = field(default_factory=dict)
    Topology: dict[str, Any] = field(default_factory=dict)
    Spans: list[ReplaySpan] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "ReplayBundle":
        # Accept both bare and {"data": {...}} envelopes.
        if "data" in data and isinstance(data["data"], dict):
            data = data["data"]
        return cls(
            SchemaVersion=int(data.get("SchemaVersion", 1) or 1),
            Run=dict(data.get("Run") or {}),
            Topology=dict(data.get("Topology") or {}),
            Spans=[ReplaySpan.from_dict(s) for s in (data.get("Spans") or [])],
        )


# ── Loader ────────────────────────────────────────────────────────────────────


def load_bundle(
    source: str | Path,
    *,
    api_url: Optional[str] = None,
    api_key: Optional[str] = None,
) -> ReplayBundle:
    """Load a replay bundle from a JSON file or the backend.

    If ``source`` is a path that exists or ends in .json it is read from disk.
    Otherwise it is treated as a run id and fetched from the backend.

    The backend URL and Bearer token default to environment variables
    ``AGENTPULSE_API_URL`` and ``AGENTPULSE_API_KEY``.
    """
    src_str = str(source)
    is_path = isinstance(source, Path) or os.path.exists(src_str) or src_str.endswith(".json")
    if is_path:
        with open(src_str, "r", encoding="utf-8") as fh:
            return ReplayBundle.from_dict(json.load(fh))

    base_url = api_url or os.environ.get("AGENTPULSE_API_URL")
    token = api_key or os.environ.get("AGENTPULSE_API_KEY")
    if not base_url:
        raise ValueError("api_url or AGENTPULSE_API_URL env var must be set to fetch a bundle")

    url = f"{base_url.rstrip('/')}/api/v1/runs/{src_str}/replay-bundle"
    req = urllib.request.Request(url)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req) as resp:  # noqa: S310 — trusted backend URL
        payload = json.loads(resp.read().decode("utf-8"))
    return ReplayBundle.from_dict(payload)


# ── Replay engine ─────────────────────────────────────────────────────────────


SpanKey = tuple[str, str, int]


class ReplayEngine:
    """Context manager that intercepts span wrappers and replays recorded outputs.

    On enter, it monkey-patches :func:`agentpulse.spans._make_wrapper` so that
    every wrapper produced afterwards consults the engine before invoking the
    user's function. The matching key is ``(agent_name, span_name, call_index)``
    where ``call_index`` is incremented per ``(agent_name, span_name)`` pair.
    """

    def __init__(
        self,
        bundle: ReplayBundle,
        overrides: Optional[dict[str, Any]] = None,
    ) -> None:
        self.bundle = bundle
        self.overrides = dict(overrides or {})
        self._call_counts: dict[tuple[str, str], int] = defaultdict(int)
        self._index: dict[SpanKey, ReplaySpan] = {}
        for span in bundle.Spans:
            self._index[(span.AgentName, span.SpanName, span.CallIndex)] = span
        self._original_make_wrapper: Optional[Callable[..., Any]] = None

    # -- Matching ----------------------------------------------------------------

    def _next_call_index(self, agent_name: str, span_name: str) -> int:
        idx = self._call_counts[(agent_name, span_name)]
        self._call_counts[(agent_name, span_name)] += 1
        return idx

    def lookup(self, agent_name: str, span_name: str, call_index: int) -> Optional[ReplaySpan]:
        return self._index.get((agent_name, span_name, call_index))

    def intercept(
        self,
        span_kind: str,
        agent_name: str,
        span_name: str,
        actual_input: Optional[str],
    ) -> tuple[bool, Any]:
        """Look up the recorded span and apply replay semantics.

        Returns ``(matched, value)``. When matched, ``value`` is the replay
        output (override-aware). When unmatched, ``value`` is ``None`` and the
        caller should run the real function.

        Side-effects: writes attributes onto the current OTel span (replay
        provenance, divergence markers, recorded prompt/completion or tool
        input/output) so the new run carries the full diff context.
        """
        call_index = self._next_call_index(agent_name, span_name)
        recorded = self.lookup(agent_name, span_name, call_index)
        current_span = trace.get_current_span()

        if recorded is None:
            try:
                current_span.set_attribute("agentpulse.replay.unmatched", True)
            except Exception:  # pragma: no cover — best-effort attr
                logger.debug("could not set replay.unmatched on current span", exc_info=True)
            return False, None

        # Replay provenance.
        try:
            current_span.set_attribute("agentpulse.replay_source_run_id", str(self.bundle.Run.get("ID", "")))
            current_span.set_attribute("agentpulse.replay_source_span_id", recorded.SpanID)
        except Exception:  # pragma: no cover
            logger.debug("could not set replay provenance attrs", exc_info=True)

        # Divergence check (string compare on recorded input field).
        recorded_input = recorded.Inputs.get("gen_ai.prompt") or recorded.Inputs.get("tool.input")
        if actual_input is not None and recorded_input is not None and actual_input != recorded_input:
            try:
                current_span.set_attribute("agentpulse.replay.diverged", True)
                current_span.set_attribute("agentpulse.replay.input.actual", actual_input)
                current_span.set_attribute("agentpulse.replay.input.recorded", recorded_input)
            except Exception:  # pragma: no cover
                logger.debug("could not set divergence attrs", exc_info=True)

        # Re-record token counts and payloads onto the new span so the
        # replay run mirrors the original's data shape.
        if span_kind == attrs.LLM_CALL:
            _spans_module.record_llm_usage(
                current_span,
                input_tokens=recorded.InputTokens,
                output_tokens=recorded.OutputTokens,
                prompt=recorded.Inputs.get("gen_ai.prompt"),
                completion=recorded.Outputs.get("gen_ai.completion"),
            )
        elif span_kind in (attrs.TOOL_CALL, attrs.MCP_TOOL_CALL):
            _spans_module.record_mcp_tool_result(
                current_span,
                tool_input=recorded.Inputs.get("tool.input"),
                tool_output=recorded.Outputs.get("tool.output"),
            )

        # Override resolution: tool name first, then span name.
        override_key = recorded.ToolName or span_name
        if recorded.ToolName and recorded.ToolName in self.overrides:
            return True, self.overrides[recorded.ToolName]
        if span_name in self.overrides:
            return True, self.overrides[span_name]
        if override_key in self.overrides:
            return True, self.overrides[override_key]

        # Default: return recorded output.
        if span_kind == attrs.LLM_CALL:
            return True, recorded.Outputs.get("gen_ai.completion", "")
        return True, recorded.Outputs.get("tool.output", "")

    # -- Context manager ---------------------------------------------------------

    def __enter__(self) -> "ReplayEngine":
        engine = self
        original = _spans_module._make_wrapper
        self._original_make_wrapper = original

        def patched_make_wrapper(fn, span_name, span_kind, extra_fn):  # type: ignore[no-untyped-def]
            wrapper = original(fn, span_name, span_kind, extra_fn)

            import functools
            import inspect

            agent_name_holder: dict[str, str] = {"name": ""}

            def _capture_agent(span):  # type: ignore[no-untyped-def]
                # Run the real attribute setter, then snoop agent.name.
                extra_fn(span)
                try:
                    val = span.attributes.get(attrs.AGENT_NAME) if span.attributes else None
                    if val:
                        agent_name_holder["name"] = str(val)
                except Exception:  # pragma: no cover
                    pass

            # Build a fresh wrapper that calls intercept before fn.
            if inspect.iscoroutinefunction(fn):
                @functools.wraps(fn)
                async def async_replay_wrapper(*args, **kwargs):  # type: ignore[no-untyped-def]
                    with _spans_module._get_tracer().start_as_current_span(span_name) as span:
                        _capture_agent(span)
                        actual_input = _stringify_input(args, kwargs)
                        matched, value = engine.intercept(
                            span_kind, agent_name_holder["name"], span_name, actual_input
                        )
                        if matched:
                            return value
                        try:
                            return await fn(*args, **kwargs)
                        except Exception as exc:
                            from opentelemetry.trace import StatusCode
                            span.set_status(StatusCode.ERROR, str(exc))
                            raise
                return async_replay_wrapper

            @functools.wraps(fn)
            def sync_replay_wrapper(*args, **kwargs):  # type: ignore[no-untyped-def]
                with _spans_module._get_tracer().start_as_current_span(span_name) as span:
                    _capture_agent(span)
                    actual_input = _stringify_input(args, kwargs)
                    matched, value = engine.intercept(
                        span_kind, agent_name_holder["name"], span_name, actual_input
                    )
                    if matched:
                        return value
                    try:
                        return fn(*args, **kwargs)
                    except Exception as exc:
                        from opentelemetry.trace import StatusCode
                        span.set_status(StatusCode.ERROR, str(exc))
                        raise
            return sync_replay_wrapper

        _spans_module._make_wrapper = patched_make_wrapper  # type: ignore[assignment]
        return self

    def __exit__(self, exc_type, exc, tb) -> None:  # type: ignore[no-untyped-def]
        if self._original_make_wrapper is not None:
            _spans_module._make_wrapper = self._original_make_wrapper  # type: ignore[assignment]
        # Best-effort flush.
        try:
            provider = trace.get_tracer_provider()
            force_flush = getattr(provider, "force_flush", None)
            if callable(force_flush):
                force_flush()
        except Exception:  # pragma: no cover
            logger.debug("force_flush failed", exc_info=True)


def _stringify_input(args: tuple, kwargs: dict) -> Optional[str]:
    """Best-effort stringification of a wrapped function's input for divergence checks."""
    if not args and not kwargs:
        return None
    try:
        if len(args) == 1 and not kwargs and isinstance(args[0], str):
            return args[0]
        return json.dumps({"args": list(args), "kwargs": kwargs}, default=str, sort_keys=True)
    except Exception:
        return None


__all__ = ["ReplaySpan", "ReplayBundle", "load_bundle", "ReplayEngine"]
