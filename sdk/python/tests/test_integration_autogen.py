"""Tests for the AutoGen / AG2 integration."""

from __future__ import annotations

import sys
import types
from unittest.mock import AsyncMock, MagicMock

import pytest
from opentelemetry.trace import StatusCode


# ── Stub autogen_agentchat (AG2 0.4+) ────────────────────────────────────────


def _install_ag2_stub() -> None:
    if "autogen_agentchat" in sys.modules:
        return

    ag2 = types.ModuleType("autogen_agentchat")
    ag2.__version__ = "0.4.0"  # type: ignore

    base_mod = types.ModuleType("autogen_agentchat.base")
    teams_mod = types.ModuleType("autogen_agentchat.teams")

    class BaseChatAgent:
        def __init__(self, name="agent"):
            self.name = name

        async def on_messages(self, messages, cancellation_token=None):
            return MagicMock(chat_message=MagicMock(content="reply"))

    class SelectorGroupChat:
        async def _select_speaker(self, *args, **kwargs):
            return MagicMock(name="agent_b")

    base_mod.BaseChatAgent = BaseChatAgent  # type: ignore
    teams_mod.SelectorGroupChat = SelectorGroupChat  # type: ignore
    ag2.base = base_mod  # type: ignore
    ag2.teams = teams_mod  # type: ignore

    sys.modules["autogen_agentchat"] = ag2
    sys.modules["autogen_agentchat.base"] = base_mod
    sys.modules["autogen_agentchat.teams"] = teams_mod

    # Stub packaging.version
    if "packaging" not in sys.modules:
        pkg = types.ModuleType("packaging")
        pkg_ver = types.ModuleType("packaging.version")

        class Version:
            def __init__(self, v):
                self._v = tuple(int(x) for x in str(v).split(".")[:3])

            def __ge__(self, other):
                return self._v >= other._v

            def __lt__(self, other):
                return self._v < other._v

        pkg_ver.Version = Version  # type: ignore
        pkg.version = pkg_ver  # type: ignore
        sys.modules["packaging"] = pkg
        sys.modules["packaging.version"] = pkg_ver


_install_ag2_stub()


# Force re-import of autogen module with the stub in place
if "agentpulse.integrations.autogen" in sys.modules:
    del sys.modules["agentpulse.integrations.autogen"]

from agentpulse.integrations.autogen import (  # noqa: E402
    instrument_autogen,
    uninstrument_autogen,
    _AG2,
)


@pytest.fixture(autouse=True)
def reset_autogen():
    uninstrument_autogen()
    yield
    uninstrument_autogen()


# ── AG2 code path ─────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_ag2_on_messages_emits_span(reset_otel):
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]

    instrument_autogen()

    agent = BaseChatAgent(name="researcher")
    result = await agent.on_messages([], None)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "agent.handoff"
    assert s.attributes.get("agent.name") == "researcher"


@pytest.mark.asyncio
async def test_ag2_on_messages_captures_output(reset_otel):
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]

    instrument_autogen()
    agent = BaseChatAgent(name="writer")
    await agent.on_messages([], None)

    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("gen_ai.completion") == "reply"


@pytest.mark.asyncio
async def test_ag2_on_messages_error(reset_otel):
    from agentpulse.integrations.autogen import _originals

    instrument_autogen()
    saved = _originals["ag2.on_messages"]

    async def failing(self, messages, cancellation_token=None):
        raise ValueError("oops")

    _originals["ag2.on_messages"] = failing
    try:
        from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]
        agent = BaseChatAgent(name="broken")
        with pytest.raises(ValueError, match="oops"):
            await agent.on_messages([], None)
        spans = reset_otel.get_finished_spans()
        assert spans[0].status.status_code == StatusCode.ERROR
    finally:
        _originals["ag2.on_messages"] = saved


@pytest.mark.asyncio
async def test_ag2_double_instrument_noop(reset_otel):
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]
    instrument_autogen()
    first_on_messages = BaseChatAgent.on_messages
    instrument_autogen()  # second call should no-op
    assert BaseChatAgent.on_messages is first_on_messages


@pytest.mark.asyncio
async def test_ag2_selector_group_chat_emits_handoff_event(reset_otel):
    from autogen_agentchat.teams import SelectorGroupChat  # type: ignore[import]

    instrument_autogen()
    chat = SelectorGroupChat()

    # Need an active span for the event to attach to
    import opentelemetry.trace as trace_api
    from opentelemetry import context as context_api

    tracer = trace_api.get_tracer("test")
    span = tracer.start_span("parent")
    token = context_api.attach(trace_api.set_span_in_context(span))
    try:
        result = await chat._select_speaker()
    finally:
        context_api.detach(token)
        span.end()

    spans = reset_otel.get_finished_spans()
    parent = next(s for s in spans if s.name == "parent")
    events = [e for e in parent.events if e.name == "agent.handoff"]
    assert len(events) == 1


# ── Context propagation ───────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_session_id_propagated(reset_otel):
    from agentpulse._context import set_session_id
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]

    set_session_id("ag2-sess-1")
    instrument_autogen()
    agent = BaseChatAgent(name="a")
    await agent.on_messages([], None)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.session_id") == "ag2-sess-1"


@pytest.mark.asyncio
async def test_user_id_propagated(reset_otel):
    from agentpulse._context import set_user_id
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]

    set_user_id("ag2-user-1")
    instrument_autogen()
    agent = BaseChatAgent(name="b")
    await agent.on_messages([], None)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.user_id") == "ag2-user-1"


# ── uninstrument ──────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_uninstrument_restores_original(reset_otel):
    from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]

    original = BaseChatAgent.on_messages
    instrument_autogen()
    uninstrument_autogen()

    # After uninstrument, calling on_messages should produce no spans
    agent = BaseChatAgent(name="c")
    await agent.on_messages([], None)
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 0
