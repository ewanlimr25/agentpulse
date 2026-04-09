"""Unit tests for agentpulse.replay (Component 2 — SDK replay mode).

These tests stay unit-level: we patch ``record_llm_usage`` /
``record_mcp_tool_result`` and the current span lookup so no live OTel
collector is required.
"""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from agentpulse import attributes as attrs
from agentpulse import replay


def _make_bundle() -> replay.ReplayBundle:
    return replay.ReplayBundle(
        SchemaVersion=1,
        Run={"ID": "run-original-123"},
        Topology={},
        Spans=[
            replay.ReplaySpan(
                SpanID="span-llm-1",
                AgentSpanKind=attrs.LLM_CALL,
                AgentName="Researcher",
                SpanName="ask_llm",
                ModelID="gpt-4o",
                CallIndex=0,
                Inputs={"gen_ai.prompt": "What is 2+2?"},
                Outputs={"gen_ai.completion": "4"},
                InputTokens=10,
                OutputTokens=2,
            ),
            replay.ReplaySpan(
                SpanID="span-tool-1",
                AgentSpanKind=attrs.TOOL_CALL,
                AgentName="Researcher",
                SpanName="web_search",
                ToolName="web_search",
                CallIndex=0,
                Inputs={"tool.input": "claude code"},
                Outputs={"tool.output": "RECORDED_RESULTS"},
            ),
            replay.ReplaySpan(
                SpanID="span-llm-2",
                AgentSpanKind=attrs.LLM_CALL,
                AgentName="Researcher",
                SpanName="ask_llm",
                ModelID="gpt-4o",
                CallIndex=1,
                Inputs={"gen_ai.prompt": "Second prompt"},
                Outputs={"gen_ai.completion": "second answer"},
                InputTokens=5,
                OutputTokens=3,
            ),
        ],
    )


def test_load_bundle_from_file_roundtrip(tmp_path: Path) -> None:
    bundle = _make_bundle()
    payload = {
        "data": {
            "SchemaVersion": bundle.SchemaVersion,
            "Run": bundle.Run,
            "Topology": bundle.Topology,
            "Spans": [
                {
                    "SpanID": s.SpanID,
                    "AgentSpanKind": s.AgentSpanKind,
                    "AgentName": s.AgentName,
                    "SpanName": s.SpanName,
                    "ModelID": s.ModelID,
                    "ToolName": s.ToolName,
                    "CallIndex": s.CallIndex,
                    "Inputs": s.Inputs,
                    "Outputs": s.Outputs,
                    "InputTokens": s.InputTokens,
                    "OutputTokens": s.OutputTokens,
                }
                for s in bundle.Spans
            ],
        }
    }
    file_path = tmp_path / "bundle.json"
    file_path.write_text(json.dumps(payload))

    loaded = replay.load_bundle(file_path)
    assert loaded.SchemaVersion == 1
    assert loaded.Run["ID"] == "run-original-123"
    assert len(loaded.Spans) == 3
    assert loaded.Spans[0].Outputs["gen_ai.completion"] == "4"
    assert loaded.Spans[1].ToolName == "web_search"


@pytest.fixture
def patched_span():
    """Patch trace.get_current_span and the record helpers used by intercept()."""
    fake_span = MagicMock(name="current_span")
    with patch("agentpulse.replay.trace.get_current_span", return_value=fake_span), \
         patch("agentpulse.replay._spans_module.record_llm_usage") as rec_llm, \
         patch("agentpulse.replay._spans_module.record_mcp_tool_result") as rec_tool:
        yield fake_span, rec_llm, rec_tool


def test_intercept_matches_by_call_index(patched_span) -> None:
    fake_span, rec_llm, rec_tool = patched_span
    engine = replay.ReplayEngine(_make_bundle())

    matched, value = engine.intercept(attrs.LLM_CALL, "Researcher", "ask_llm", "What is 2+2?")
    assert matched is True
    assert value == "4"
    rec_llm.assert_called_once()
    # Provenance attribute set on current span
    fake_span.set_attribute.assert_any_call("agentpulse.replay_source_run_id", "run-original-123")
    fake_span.set_attribute.assert_any_call("agentpulse.replay_source_span_id", "span-llm-1")


def test_intercept_increments_call_index_for_repeats(patched_span) -> None:
    engine = replay.ReplayEngine(_make_bundle())
    matched1, val1 = engine.intercept(attrs.LLM_CALL, "Researcher", "ask_llm", "What is 2+2?")
    matched2, val2 = engine.intercept(attrs.LLM_CALL, "Researcher", "ask_llm", "Second prompt")
    assert matched1 and matched2
    assert val1 == "4"
    assert val2 == "second answer"


def test_intercept_returns_override_when_set(patched_span) -> None:
    engine = replay.ReplayEngine(_make_bundle(), overrides={"web_search": "OVERRIDE_RESULT"})
    matched, value = engine.intercept(attrs.TOOL_CALL, "Researcher", "web_search", "claude code")
    assert matched is True
    assert value == "OVERRIDE_RESULT"


def test_intercept_marks_unmatched_on_miss(patched_span) -> None:
    fake_span, _, _ = patched_span
    engine = replay.ReplayEngine(_make_bundle())
    matched, value = engine.intercept(attrs.LLM_CALL, "Unknown", "no_such_span", None)
    assert matched is False
    assert value is None
    fake_span.set_attribute.assert_any_call("agentpulse.replay.unmatched", True)


def test_intercept_marks_divergence_on_input_mismatch(patched_span) -> None:
    fake_span, _, _ = patched_span
    engine = replay.ReplayEngine(_make_bundle())
    engine.intercept(attrs.TOOL_CALL, "Researcher", "web_search", "DIFFERENT INPUT")
    fake_span.set_attribute.assert_any_call("agentpulse.replay.diverged", True)
    fake_span.set_attribute.assert_any_call("agentpulse.replay.input.actual", "DIFFERENT INPUT")
    fake_span.set_attribute.assert_any_call("agentpulse.replay.input.recorded", "claude code")
