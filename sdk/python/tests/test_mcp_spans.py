"""Tests for MCP (Model Context Protocol) span decorators and helpers."""

from __future__ import annotations

import pytest
from opentelemetry import trace
from opentelemetry.trace import StatusCode

from agentpulse.spans import (
    mcp_tool_call,
    mcp_tool_call_ctx,
    mcp_list_tools,
    mcp_list_tools_ctx,
    mcp_server_ctx,
    record_mcp_tool_result,
    record_mcp_discovery,
)
from agentpulse import attributes as attrs


# ── mcp_tool_call decorator ───────────────────────────────────────────────────


def test_mcp_tool_call_sets_span_kind(reset_otel):
    @mcp_tool_call(server_name="fs-server", tool_name="read_file")
    def read_file():
        return "content"

    read_file()
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get(attrs.SPAN_KIND) == "mcp.tool_call"


def test_mcp_tool_call_sets_server_and_tool_name(reset_otel):
    @mcp_tool_call(server_name="filesystem-server", tool_name="write_file")
    def write_file():
        return None

    write_file()
    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get(attrs.MCP_SERVER_NAME) == "filesystem-server"
    assert s.attributes.get(attrs.MCP_TOOL_NAME) == "write_file"
    # Also sets tool.name for analytics
    assert s.attributes.get(attrs.TOOL_NAME) == "write_file"


def test_mcp_tool_call_sets_agent_name(reset_otel):
    @mcp_tool_call(server_name="srv", tool_name="tool", agent_name="my_agent")
    def my_fn():
        return None

    my_fn()
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get(attrs.AGENT_NAME) == "my_agent"


def test_mcp_tool_call_sets_run_id(reset_otel):
    @mcp_tool_call(server_name="srv", tool_name="t")
    def my_fn():
        return None

    my_fn()
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get(attrs.RUN_ID) is not None


def test_mcp_tool_call_error_sets_status(reset_otel):
    @mcp_tool_call(server_name="srv", tool_name="t")
    def failing():
        raise RuntimeError("mcp error")

    with pytest.raises(RuntimeError, match="mcp error"):
        failing()

    spans = reset_otel.get_finished_spans()
    assert spans[0].status.status_code == StatusCode.ERROR


@pytest.mark.asyncio
async def test_mcp_tool_call_async(reset_otel):
    @mcp_tool_call(server_name="srv", tool_name="async_tool")
    async def async_fn():
        return "result"

    result = await async_fn()
    assert result == "result"
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get(attrs.SPAN_KIND) == "mcp.tool_call"


def test_mcp_tool_call_custom_span_name(reset_otel):
    @mcp_tool_call(server_name="srv", tool_name="t", span_name="custom.span")
    def my_fn():
        return None

    my_fn()
    spans = reset_otel.get_finished_spans()
    assert spans[0].name == "custom.span"


# ── mcp_tool_call_ctx context manager ────────────────────────────────────────


def test_mcp_tool_call_ctx_sets_attributes(reset_otel):
    with mcp_tool_call_ctx(server_name="gh-server", tool_name="create_issue") as span:
        pass

    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    s = spans[0]
    assert s.attributes.get(attrs.SPAN_KIND) == "mcp.tool_call"
    assert s.attributes.get(attrs.MCP_SERVER_NAME) == "gh-server"
    assert s.attributes.get(attrs.MCP_TOOL_NAME) == "create_issue"
    assert s.attributes.get(attrs.TOOL_NAME) == "create_issue"


def test_mcp_tool_call_ctx_error(reset_otel):
    with pytest.raises(ValueError, match="ctx error"):
        with mcp_tool_call_ctx(server_name="srv", tool_name="t"):
            raise ValueError("ctx error")

    spans = reset_otel.get_finished_spans()
    assert spans[0].status.status_code == StatusCode.ERROR


# ── mcp_list_tools decorator ──────────────────────────────────────────────────


def test_mcp_list_tools_sets_span_kind(reset_otel):
    @mcp_list_tools(server_name="my-server")
    def discover():
        return ["tool_a", "tool_b"]

    discover()
    spans = reset_otel.get_finished_spans()
    assert len(spans) == 1
    assert spans[0].attributes.get(attrs.SPAN_KIND) == "mcp.list_tools"


def test_mcp_list_tools_sets_server_name(reset_otel):
    @mcp_list_tools(server_name="filesystem-server")
    def discover():
        return []

    discover()
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get(attrs.MCP_SERVER_NAME) == "filesystem-server"


def test_mcp_list_tools_error(reset_otel):
    @mcp_list_tools(server_name="srv")
    def failing():
        raise ConnectionError("server down")

    with pytest.raises(ConnectionError):
        failing()

    spans = reset_otel.get_finished_spans()
    assert spans[0].status.status_code == StatusCode.ERROR


@pytest.mark.asyncio
async def test_mcp_list_tools_async(reset_otel):
    @mcp_list_tools(server_name="async-server")
    async def discover():
        return ["tool_x"]

    result = await discover()
    assert result == ["tool_x"]
    spans = reset_otel.get_finished_spans()
    assert spans[0].attributes.get(attrs.SPAN_KIND) == "mcp.list_tools"


# ── mcp_list_tools_ctx context manager ───────────────────────────────────────


def test_mcp_list_tools_ctx(reset_otel):
    with mcp_list_tools_ctx(server_name="srv") as span:
        pass

    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get(attrs.SPAN_KIND) == "mcp.list_tools"
    assert s.attributes.get(attrs.MCP_SERVER_NAME) == "srv"


# ── record_mcp_tool_result ────────────────────────────────────────────────────


def test_record_mcp_tool_result_attaches_schemas(reset_otel):
    with mcp_tool_call_ctx(server_name="srv", tool_name="read_file") as span:
        record_mcp_tool_result(
            span,
            input_schema='{"path": "string"}',
            output_schema='{"content": "string"}',
            tool_input="/etc/hosts",
            tool_output="127.0.0.1 localhost",
        )

    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get(attrs.MCP_INPUT_SCHEMA) == '{"path": "string"}'
    assert s.attributes.get(attrs.MCP_OUTPUT_SCHEMA) == '{"content": "string"}'
    assert s.attributes.get("tool.input") == "/etc/hosts"
    assert s.attributes.get("tool.output") == "127.0.0.1 localhost"


def test_record_mcp_tool_result_partial(reset_otel):
    with mcp_tool_call_ctx(server_name="srv", tool_name="t") as span:
        record_mcp_tool_result(span, tool_output="result")

    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get("tool.output") == "result"
    assert s.attributes.get(attrs.MCP_INPUT_SCHEMA) is None


# ── record_mcp_discovery ──────────────────────────────────────────────────────


def test_record_mcp_discovery_attaches_tool_list(reset_otel):
    tools = ["read_file", "write_file", "list_directory", "search_files"]
    with mcp_list_tools_ctx(server_name="fs-server") as span:
        record_mcp_discovery(span, tool_count=len(tools), discovered_tools=tools)

    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get(attrs.MCP_TOOL_COUNT) == "4"
    assert s.attributes.get(attrs.MCP_DISCOVERED_TOOLS) == "read_file,write_file,list_directory,search_files"


def test_record_mcp_discovery_empty_list(reset_otel):
    with mcp_list_tools_ctx(server_name="srv") as span:
        record_mcp_discovery(span, tool_count=0, discovered_tools=[])

    spans = reset_otel.get_finished_spans()
    s = spans[0]
    assert s.attributes.get(attrs.MCP_TOOL_COUNT) == "0"
    assert s.attributes.get(attrs.MCP_DISCOVERED_TOOLS) is None


# ── Cross-process correlation: session/request IDs ────────────────────────────


def test_mcp_tool_call_propagates_correlation_ids(reset_otel):
    @mcp_tool_call(
        server_name="srv",
        tool_name="t",
        session_id="sess-42",
        request_id="req-7",
        client_name="claude-code",
        transport="stdio",
    )
    def call():
        return None

    call()
    s = reset_otel.get_finished_spans()[0]
    assert s.attributes.get(attrs.MCP_SESSION_ID) == "sess-42"
    assert s.attributes.get(attrs.MCP_REQUEST_ID) == "req-7"
    assert s.attributes.get(attrs.MCP_CLIENT_NAME) == "claude-code"
    assert s.attributes.get(attrs.MCP_TRANSPORT) == "stdio"


def test_mcp_tool_call_ctx_propagates_correlation_ids(reset_otel):
    with mcp_tool_call_ctx(
        server_name="srv",
        tool_name="t",
        session_id="sess-1",
        request_id="req-2",
    ):
        pass

    s = reset_otel.get_finished_spans()[0]
    assert s.attributes.get(attrs.MCP_SESSION_ID) == "sess-1"
    assert s.attributes.get(attrs.MCP_REQUEST_ID) == "req-2"


# ── mcp_server_ctx (server-side execution) ────────────────────────────────────


def test_mcp_server_ctx_sets_span_kind(reset_otel):
    with mcp_server_ctx(server_name="my-mcp", tool_name="echo"):
        pass
    s = reset_otel.get_finished_spans()[0]
    assert s.attributes.get(attrs.SPAN_KIND) == "mcp.server"
    assert s.attributes.get(attrs.MCP_SERVER_NAME) == "my-mcp"
    assert s.attributes.get(attrs.MCP_TOOL_NAME) == "echo"
    assert s.attributes.get(attrs.MCP_TRANSPORT) == "stdio"


def test_mcp_server_ctx_correlates_with_client(reset_otel):
    with mcp_server_ctx(
        server_name="srv",
        tool_name="t",
        session_id="shared-sess",
        request_id="shared-req",
        client_name="cursor",
        transport="sse",
    ):
        pass
    s = reset_otel.get_finished_spans()[0]
    assert s.attributes.get(attrs.MCP_SESSION_ID) == "shared-sess"
    assert s.attributes.get(attrs.MCP_REQUEST_ID) == "shared-req"
    assert s.attributes.get(attrs.MCP_CLIENT_NAME) == "cursor"
    assert s.attributes.get(attrs.MCP_TRANSPORT) == "sse"


def test_mcp_server_ctx_error(reset_otel):
    with pytest.raises(RuntimeError, match="boom"):
        with mcp_server_ctx(server_name="srv", tool_name="t"):
            raise RuntimeError("boom")
    s = reset_otel.get_finished_spans()[0]
    assert s.status.status_code == StatusCode.ERROR
