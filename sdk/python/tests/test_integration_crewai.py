"""Tests for the CrewAI integration."""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock, patch

import pytest
from opentelemetry.trace import StatusCode


# ── Stub crewai package ───────────────────────────────────────────────────────


def _install_crewai_stub() -> None:
    if "crewai" in sys.modules:
        return

    crewai = types.ModuleType("crewai")
    tools_mod = types.ModuleType("crewai.tools")

    class Crew:
        def __init__(self, name="test_crew", agents=None, tasks=None):
            self.name = name
            self.agents = agents or []
            self.tasks = tasks or []
            self._executor = None

        def kickoff(self, inputs=None):
            return "crew result"

        async def kickoff_async(self, inputs=None):
            return "crew result async"

    class Agent:
        def __init__(self, role="worker", llm=None):
            self.role = role
            self.llm = llm

        def execute_task(self, task, context=None, tools=None):
            return "agent result"

    class BaseTool:
        def __init__(self, name="tool"):
            self.name = name

        def _run(self, *args, **kwargs):
            return "tool result"

        async def _arun(self, *args, **kwargs):
            return "async tool result"

    crewai.Agent = Agent  # type: ignore
    crewai.Crew = Crew  # type: ignore
    tools_mod.BaseTool = BaseTool  # type: ignore
    crewai.tools = tools_mod  # type: ignore
    sys.modules["crewai"] = crewai
    sys.modules["crewai.tools"] = tools_mod


_install_crewai_stub()

if "agentpulse.integrations.crewai" in sys.modules:
    del sys.modules["agentpulse.integrations.crewai"]

from agentpulse.integrations.crewai import (  # noqa: E402
    instrument_crewai,
    uninstrument_crewai,
)


@pytest.fixture(autouse=True)
def reset_crewai():
    uninstrument_crewai()
    yield
    uninstrument_crewai()


# ── Crew.kickoff ──────────────────────────────────────────────────────────────


def test_kickoff_emits_root_span(reset_otel):
    instrument_crewai()
    from crewai import Crew  # type: ignore[import]
    crew = Crew(name="my_crew")
    crew.kickoff()
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "agent.handoff"
    assert s.attributes.get("agent.name") == "my_crew"


def test_kickoff_pins_run_id(reset_otel):
    from agentpulse._context import get_run_id, reset_run
    reset_run()
    instrument_crewai()
    from crewai import Crew  # type: ignore[import]
    crew = Crew()
    crew.kickoff()
    run_id = get_run_id()
    assert run_id is not None
    # Second kickoff re-uses or generates a consistent run_id
    crew.kickoff()
    assert get_run_id() == run_id


def test_kickoff_error_sets_span_status(reset_otel):
    from crewai import Crew  # type: ignore[import]

    saved = Crew.kickoff

    def failing_kickoff(self, inputs=None):
        raise RuntimeError("fail")

    Crew.kickoff = failing_kickoff
    instrument_crewai()
    try:
        crew = Crew(name="bad")
        with pytest.raises(RuntimeError, match="fail"):
            crew.kickoff()
        spans = reset_otel.get_finished_spans()
        assert spans[0].status.status_code == StatusCode.ERROR
    finally:
        uninstrument_crewai()
        Crew.kickoff = saved


@pytest.mark.asyncio
async def test_kickoff_async_emits_span(reset_otel):
    instrument_crewai()
    from crewai import Crew  # type: ignore[import]
    crew = Crew(name="async_crew")
    result = await crew.kickoff_async()
    assert result == "crew result async"
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get("agentpulse.span_kind") == "agent.handoff"


# ── Agent.execute_task ────────────────────────────────────────────────────────


def test_execute_task_emits_agent_span(reset_otel):
    instrument_crewai()
    from crewai import Agent  # type: ignore[import]
    llm = MagicMock(model_name="gpt-4o")
    agent = Agent(role="researcher", llm=llm)
    task = MagicMock(description="Research AI")
    result = agent.execute_task(task)
    assert result == "agent result"
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "agent.handoff"
    assert s.attributes.get("agent.name") == "researcher"
    assert s.attributes.get("gen_ai.request.model") == "gpt-4o"
    assert "Research AI" in (s.attributes.get("gen_ai.prompt") or "")


def test_execute_task_captures_result(reset_otel):
    instrument_crewai()
    from crewai import Agent  # type: ignore[import]
    agent = Agent(role="writer")
    task = MagicMock(description="Write a report")
    agent.execute_task(task)
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("gen_ai.completion") == "agent result"


def test_execute_task_error(reset_otel):
    from crewai import Agent  # type: ignore[import]

    saved = Agent.execute_task

    def failing_execute(self, task, context=None, tools=None):
        raise ValueError("broken")

    Agent.execute_task = failing_execute
    instrument_crewai()
    try:
        agent = Agent(role="broken")
        with pytest.raises(ValueError):
            agent.execute_task(MagicMock(description="x"))
        spans = reset_otel.get_finished_spans()
        assert spans[0].status.status_code == StatusCode.ERROR
    finally:
        uninstrument_crewai()
        Agent.execute_task = saved


# ── BaseTool._run and _arun ───────────────────────────────────────────────────


def test_tool_run_emits_span(reset_otel):
    instrument_crewai()
    from crewai.tools import BaseTool  # type: ignore[import]
    tool = BaseTool(name="web_search")
    result = tool._run("query")
    assert result == "tool result"
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "tool.call"
    assert s.attributes.get("tool.name") == "web_search"
    assert s.attributes.get("tool.input") == "query"
    assert s.attributes.get("tool.output") == "tool result"


@pytest.mark.asyncio
async def test_tool_arun_emits_span(reset_otel):
    instrument_crewai()
    from crewai.tools import BaseTool  # type: ignore[import]
    tool = BaseTool(name="async_search")
    result = await tool._arun("async query")
    assert result == "async tool result"
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get("agentpulse.span_kind") == "tool.call"
    assert s.attributes.get("tool.name") == "async_search"


def test_tool_run_error(reset_otel):
    from crewai.tools import BaseTool  # type: ignore[import]

    saved = BaseTool._run

    def failing_run(self, *args, **kwargs):
        raise RuntimeError("tool broken")

    BaseTool._run = failing_run
    instrument_crewai()
    try:
        tool = BaseTool(name="broken")
        with pytest.raises(RuntimeError):
            tool._run("x")
        spans = reset_otel.get_finished_spans()
        assert spans[0].status.status_code == StatusCode.ERROR
    finally:
        uninstrument_crewai()
        BaseTool._run = saved


# ── Double instrumentation guard ──────────────────────────────────────────────


def test_double_instrument_is_noop(reset_otel, caplog):
    instrument_crewai()
    from crewai import Crew  # type: ignore[import]
    original_kickoff = Crew.kickoff
    instrument_crewai()  # second call should warn and skip
    assert Crew.kickoff is original_kickoff


# ── Context propagation ───────────────────────────────────────────────────────


def test_session_id_propagated_to_crew_span(reset_otel):
    from agentpulse._context import set_session_id
    set_session_id("crew-sess-1")
    instrument_crewai()
    from crewai import Crew  # type: ignore[import]
    Crew(name="c").kickoff()
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.session_id") == "crew-sess-1"


def test_user_id_propagated_to_agent_span(reset_otel):
    from agentpulse._context import set_user_id
    set_user_id("crew-user-1")
    instrument_crewai()
    from crewai import Agent  # type: ignore[import]
    Agent(role="r").execute_task(MagicMock(description="t"))
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get("agentpulse.user_id") == "crew-user-1"


# ── uninstrument ──────────────────────────────────────────────────────────────


def test_uninstrument_restores_original(reset_otel):
    from crewai import Crew  # type: ignore[import]
    instrument_crewai()
    uninstrument_crewai()
    crew = Crew(name="u")
    crew.kickoff()
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 0
