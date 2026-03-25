"""
Internal context propagation for AgentPulse-specific values.

OTel handles trace/span context automatically via contextvars. This module
layers run_id, session_id, and user_id on top — concepts AgentPulse uses to
group spans into runs (single executions), sessions (multi-turn conversations),
and attribute cost to individual users/customers.
"""

from __future__ import annotations

import uuid
from contextvars import ContextVar
from typing import Optional

_run_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_run_id", default=None)
_project_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_project_id", default=None)
_session_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_session_id", default=None)
_user_id_var: ContextVar[Optional[str]] = ContextVar("agentpulse_user_id", default=None)


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


def generate_session_id() -> str:
    """Generate a new random session ID.

    Convenience helper so callers don't need to import uuid directly.
    """
    return str(uuid.uuid4())


def set_session_id(session_id: str) -> None:
    """Pin a session_id for the current async context.

    All spans created after this call will carry ``agentpulse.session_id``,
    grouping multiple runs into a single conversation/session in the UI.

    Sessions are opt-in — spans without a session_id are still tracked
    individually as runs but won't appear in the Sessions tab.

    Args:
        session_id: Arbitrary string identifier for this session.
                    Use generate_session_id() to create a random UUID.
    """
    _session_id_var.set(session_id)


def get_session_id() -> Optional[str]:
    """Return the current session_id, or None if not set.

    Unlike run_id, session_id is never auto-generated — it returns None
    until explicitly set with set_session_id().
    """
    return _session_id_var.get()


def reset_session() -> None:
    """Clear the current session_id.

    Useful when starting a new conversation after a previous one ended,
    or in tests to isolate session state between test cases.
    """
    _session_id_var.set(None)


def set_user_id(user_id: str) -> None:
    """Pin a user_id for the current async context.

    All spans created after this call will carry ``agentpulse.user_id``,
    enabling per-user cost attribution in the UI.

    The value must be an opaque identifier such as a UUID or internal
    customer ID — NOT an email address or display name.

    Args:
        user_id: Opaque identifier for the end user (e.g. UUID, database PK).

    Raises:
        ValueError: If user_id contains ``@`` or whitespace characters.
    """
    if '@' in user_id or any(c.isspace() for c in user_id):
        raise ValueError(
            "user_id should be an opaque identifier (e.g. UUID or internal customer ID), "
            "not an email or display name. Got a value containing '@' or whitespace."
        )
    _user_id_var.set(user_id)


def get_user_id() -> Optional[str]:
    """Return the current user_id, or None if not set.

    Unlike run_id, user_id is never auto-generated — it returns None
    until explicitly set with set_user_id().
    """
    return _user_id_var.get()


def reset_user() -> None:
    """Clear the current user_id.

    Useful in tests to isolate user state between test cases, or when
    processing requests on behalf of different users sequentially.
    """
    _user_id_var.set(None)
