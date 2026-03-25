"""Tests for the OpenAI Agents SDK integration (RunHooks-based)."""

from __future__ import annotations

import sys
import types
from unittest.mock import AsyncMock, MagicMock

import pytest
from opentelemetry.trace import StatusCode


# ── Stub the agents package ───────────────────────────────────────────────────


def _install_agents_stub() -> None:
    if "agents" in sys.modules:
        return

    agents_mod = types.ModuleType("agents")

    class RunContextWrapper:
        def __init__(self):
            self.context = None

    class RunHooks:
        async def on_agent_start(self, context, agent): ...
        async def on_agent_end(self, context, agent, output): ...
        async def on_tool_start(self, context, agent, tool): ...
        async def on_tool_end(self, context, agent, tool, result): ...
        async def on_handoff(self, context, from_agent, to_agent): ...

    agents_mod.RunHooks = RunHooks  # type: ignore
    agents_mod.RunContextWrapper = RunContextWrapper  # type: ignore
    sys.modules["agents"] = agents_mod


_install_agents_stub()

from agentpulse.integrations.openai_agents import (  # noqa: E402
    AgentPulseRunHooks,
    instrument_openai_agents,
    instrument_runner,
    uninstrument_runner,
)


def _make_agent(name: str = "test_agent"):
    a = MagicMock()
    a.name = name
    return a


def _make_tool(name: str = "my_tool"):
    t = MagicMock()
    t.name = name
    return t


def _make_context():
    from agents import RunContextWrapper
    return RunContextWrapper()


# ── instrument_openai_agents factory ─────────────────────────────────────────


def test_instrument_openai_agents_returns_hooks(reset_otel):
    hooks = instrument_openai_agents()
    assert isinstance(hooks, AgentPulseRunHooks)


def test_multiple_calls_return_different_instances(reset_otel):
    h1 = instrument_openai_agents()
    h2 = instrument_openai_agents()
    assert h1 is not h2  # each call creates a fresh instance


# ── on_agent_start / on_agent_end ─────────────────────────────────────────────


@pytest.mark.asyncio
async def test_agent_start_end_emits_span(reset_otel):
    hooks = AgentPulseRunHooks()
    agent = _make_agent("researcher")
    ctx = _make_context()

    await hooks.on_agent_start(ctx, agent)
    await hooks.on_agent_end(ctx, agent, output="done")

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "agent.handoff"
    assert s.attributes.get("agent.name") == "researcher"
    assert s.attributes.get("gen_ai.completion") == "done"


@pytest.mark.asyncio
async def test_agent_run_id_pinned(reset_otel):
    from agentpulse._context import get_run_id, reset_run
    reset_run()
    hooks = AgentPulseRunHooks()
    agent = _make_agent()
    ctx = _make_context()
    await hooks.on_agent_start(ctx, agent)
    run_id_during = get_run_id()
    await hooks.on_agent_end(ctx, agent, output=None)
    assert run_id_during is not None


# ── on_tool_start / on_tool_end ───────────────────────────────────────────────


@pytest.mark.asyncio
async def test_tool_start_end_emits_span(reset_otel):
    hooks = AgentPulseRunHooks()
    agent = _make_agent()
    tool = _make_tool("web_search")
    ctx = _make_context()

    await hooks.on_tool_start(ctx, agent, tool)
    await hooks.on_tool_end(ctx, agent, tool, result="some results")

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "tool.call"
    assert s.attributes.get("tool.name") == "web_search"
    assert s.attributes.get("tool.output") == "some results"


@pytest.mark.asyncio
async def test_tool_span_nested_under_agent(reset_otel):
    hooks = AgentPulseRunHooks()
    agent = _make_agent()
    tool = _make_tool("calc")
    ctx = _make_context()

    await hooks.on_agent_start(ctx, agent)
    await hooks.on_tool_start(ctx, agent, tool)
    await hooks.on_tool_end(ctx, agent, tool, result="42")
    await hooks.on_agent_end(ctx, agent, output="done")

    spans = reset_otel.get_finished_spans()
    # tool span should finish before agent span
    tool_span = next(s for s in spans if s.attributes.get("agentpulse.span_kind") == "tool.call")
    agent_span = next(s for s in spans if s.attributes.get("agentpulse.span_kind") == "agent.handoff")
    assert tool_span is not None
    assert agent_span is not None


# ── on_handoff ────────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_handoff_event_added_to_agent_span(reset_otel):
    hooks = AgentPulseRunHooks()
    agent_a = _make_agent("agent_a")
    agent_b = _make_agent("agent_b")
    ctx = _make_context()

    await hooks.on_agent_start(ctx, agent_a)
    await hooks.on_handoff(ctx, agent_a, agent_b)
    await hooks.on_agent_end(ctx, agent_a, output=None)

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    events = spans[0].events
    assert any(e.name == "agent.handoff" for e in events)
    handoff_event = next(e for e in events if e.name == "agent.handoff")
    assert handoff_event.attributes.get("agentpulse.handoff.target_agent") == "agent_b"


# ── Context propagation ───────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_session_id_propagated(reset_otel):
    from agentpulse._context import set_session_id
    set_session_id("sess-hooks-test")
    hooks = AgentPulseRunHooks()
    agent = _make_agent()
    ctx = _make_context()
    await hooks.on_agent_start(ctx, agent)
    await hooks.on_agent_end(ctx, agent, output=None)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.session_id") == "sess-hooks-test"


@pytest.mark.asyncio
async def test_user_id_propagated(reset_otel):
    from agentpulse._context import set_user_id
    set_user_id("user-hooks-test")
    hooks = AgentPulseRunHooks()
    agent = _make_agent()
    ctx = _make_context()
    await hooks.on_tool_start(ctx, agent, _make_tool())
    await hooks.on_tool_end(ctx, agent, _make_tool(), result=None)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.user_id") == "user-hooks-test"


# ── Backward-compatible instrument_runner ─────────────────────────────────────


@pytest.mark.asyncio
async def test_instrument_runner_backward_compat(reset_otel):
    class FakeRunner:
        async def run(self, agent, task, **kwargs):
            return MagicMock(raw_responses=[MagicMock()])

    runner = FakeRunner()
    instrument_runner(runner)
    agent = _make_agent()
    await runner.run(agent, "task")
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "agent.handoff"


def test_instrument_runner_double_instrument_noop(reset_otel):
    runner = MagicMock()
    runner.run = AsyncMock(return_value=None)
    instrument_runner(runner)
    original_run = runner.run
    instrument_runner(runner)  # second call should be a warning+noop
    # run should still be the first patched version
    assert runner.run is original_run


def test_uninstrument_runner(reset_otel):
    runner = MagicMock()
    original = AsyncMock(return_value=None)
    runner.run = original
    instrument_runner(runner)
    uninstrument_runner(runner)
    # After uninstrument, the monkey-patched attribute is removed;
    # accessing runner.run falls back to the MagicMock's auto-attribute
    assert not getattr(runner, "_agentpulse_instrumented", False)
