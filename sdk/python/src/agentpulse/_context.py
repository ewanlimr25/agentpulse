"""
Internal context propagation for AgentPulse-specific values.

OTel handles trace/span context automatically via contextvars. This module
layers run_id on top — a concept AgentPulse uses to group all spans in a
single agent execution into one "run" in the UI.
"""

from __future__ import annotations

import uuid
from contextvars import ContextVar
from typing import Optional

_run_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_run_id", default=None)
_project_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_project_id", default=None)


def set_run_id(run_id: str) -> None:
    """Pin a specific run_id for the current async context.

    Useful when you want multiple top-level agent invocations to share a run,
    or when you generate the run_id externally (e.g. from a request ID).
    """
    _run_id_var.set(run_id)


def get_run_id() -> str:
    """Return the current run_id, auto-generating one if not set."""
    run_id = _run_id_var.get()
    if run_id is None:
        run_id = str(uuid.uuid4())
        _run_id_var.set(run_id)
    return run_id


def set_project_id(project_id: str) -> None:
    """Store project_id in context so span helpers can access it without passing it around."""
    _project_id_var.set(project_id)


def get_project_id() -> Optional[str]:
    """Return the project_id from context, or None if not set."""
    return _project_id_var.get()


def reset_run() -> None:
    """Clear the current run_id, causing the next span to start a new run.

    Useful in tests or when running multiple agent executions sequentially.
    """
    _run_id_var.set(None)
