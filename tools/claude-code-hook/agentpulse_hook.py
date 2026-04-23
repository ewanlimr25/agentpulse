#!/usr/bin/env python3
"""AgentPulse Claude Code hook — emits OTLP spans for every tool invocation.

Events: PreToolUse (record start time), PostToolUse (send span), Stop (session span).
Architecture: parent forks a detached child for I/O and exits 0 immediately.
Requirements: Python 3.8+, stdlib only.
"""

from __future__ import annotations

import hashlib
import http.client
import json
import os
import pathlib
import secrets
import subprocess
import sys
import time
import uuid
from typing import Any, Dict, Optional

AGENTPULSE_DIR = pathlib.Path.home() / ".agentpulse"
CREDENTIALS_FILE = AGENTPULSE_DIR / "credentials"
TMP_DIR = AGENTPULSE_DIR / "tmp"
RUN_DIR = AGENTPULSE_DIR / "run"

HOOK_SCOPE_NAME = "agentpulse-claude-code-hook"
HOOK_SCOPE_VERSION = "0.1.0"
SERVICE_NAME = "claude-code"
AGENT_NAME = "claude-code"
HTTP_TIMEOUT = 5


# ---------------------------------------------------------------------------
# Credentials
# ---------------------------------------------------------------------------

def load_credentials() -> Optional[Dict[str, str]]:
    """Parse ~/.agentpulse/credentials. Returns None if missing or incomplete."""
    if not CREDENTIALS_FILE.exists():
        return None
    creds: Dict[str, str] = {}
    try:
        for raw_line in CREDENTIALS_FILE.read_text().splitlines():
            line = raw_line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, _, value = line.partition("=")
            creds[key.strip()] = value.strip()
    except OSError:
        return None
    required = {"AGENTPULSE_PROJECT_ID", "AGENTPULSE_ENDPOINT", "AGENTPULSE_INGEST_TOKEN"}
    return creds if required.issubset(creds) else None


# ---------------------------------------------------------------------------
# Run identity
# ---------------------------------------------------------------------------

def get_or_create_run_id(session_id: str) -> str:
    """Return persistent run UUID for this session, creating it if needed."""
    RUN_DIR.mkdir(parents=True, exist_ok=True)
    run_file = RUN_DIR / f"{session_id}.json"
    if run_file.exists():
        try:
            return json.loads(run_file.read_text())["run_id"]
        except (OSError, KeyError, json.JSONDecodeError):
            pass
    run_id = str(uuid.uuid4())
    run_file.write_text(json.dumps({"run_id": run_id}))
    return run_id


def delete_run_id(session_id: str) -> None:
    """Remove run-ID file on session Stop."""
    try:
        (RUN_DIR / f"{session_id}.json").unlink(missing_ok=True)
    except OSError:
        pass


# ---------------------------------------------------------------------------
# Trace / span ID helpers
# ---------------------------------------------------------------------------

def trace_id_from_run_id(run_id: str) -> str:
    """Derive deterministic 32-char hex trace ID from run_id via SHA-256."""
    return hashlib.sha256(run_id.encode()).digest()[:16].hex()


def new_span_id() -> str:
    """Generate random 16-char hex span ID."""
    return secrets.token_bytes(8).hex()


# ---------------------------------------------------------------------------
# Start-time temp file
# ---------------------------------------------------------------------------

def write_start_time(tool_use_id: str) -> None:
    """Persist wall-clock start time for a tool invocation."""
    TMP_DIR.mkdir(parents=True, exist_ok=True)
    (TMP_DIR / f"{tool_use_id}.json").write_text(json.dumps({"start_ns": time.time_ns()}))


def read_and_delete_start_time(tool_use_id: str) -> Optional[int]:
    """Read persisted start time, delete file. Returns None if missing."""
    tmp_file = TMP_DIR / f"{tool_use_id}.json"
    try:
        start_ns: int = json.loads(tmp_file.read_text())["start_ns"]
        tmp_file.unlink(missing_ok=True)
        return start_ns
    except (OSError, KeyError, json.JSONDecodeError):
        return None


# ---------------------------------------------------------------------------
# OTLP span construction
# ---------------------------------------------------------------------------

def _s(value: str) -> Dict[str, Any]:
    return {"stringValue": value}


def _resource_attrs(project_id: str, ingest_token: str, session_id: str, run_id: str) -> list:
    return [
        {"key": "service.name",           "value": _s(SERVICE_NAME)},
        {"key": "agentpulse.agent.name",  "value": _s(AGENT_NAME)},
        {"key": "agentpulse.project_id",  "value": _s(project_id)},
        {"key": "agentpulse.ingest_token","value": _s(ingest_token)},
        {"key": "agentpulse.session_id",  "value": _s(session_id)},
        {"key": "agentpulse.run_id",      "value": _s(run_id)},
    ]


def build_tool_span(
    trace_id: str,
    span_id: str,
    tool_name: str,
    tool_use_id: str,
    cwd: str,
    tool_input: Any,
    tool_response: Any,
    start_ns: int,
    end_ns: int,
) -> Dict[str, Any]:
    """Construct the OTLP/JSON span dict for a PostToolUse event."""
    return {
        "traceId":           trace_id,
        "spanId":            span_id,
        "name":              f"tool.{tool_name}",
        "kind":              1,
        "startTimeUnixNano": str(start_ns),
        "endTimeUnixNano":   str(end_ns),
        "attributes": [
            {"key": "tool.name",               "value": _s(tool_name)},
            {"key": "agentpulse.span_kind",    "value": _s("tool.call")},
            {"key": "gen_ai.operation.name",   "value": _s("tool_call")},
            {"key": "tool.input",              "value": _s(json.dumps(tool_input))},
            {"key": "tool.output",             "value": _s(json.dumps(tool_response))},
            {"key": "claude_code.tool_use_id", "value": _s(tool_use_id)},
            {"key": "claude_code.cwd",         "value": _s(cwd)},
        ],
        "status": {"code": 1},
    }


def build_stop_span(trace_id: str, span_id: str, now_ns: int) -> Dict[str, Any]:
    """Construct the OTLP/JSON span dict for a Stop event."""
    return {
        "traceId":           trace_id,
        "spanId":            span_id,
        "name":              "claude_code.session.stop",
        "kind":              1,
        "startTimeUnixNano": str(now_ns),
        "endTimeUnixNano":   str(now_ns),
        "attributes": [
            {"key": "agentpulse.span_kind", "value": _s("claude_code.session.stop")},
        ],
        "status": {"code": 1},
    }


def build_payload(
    project_id: str,
    ingest_token: str,
    session_id: str,
    run_id: str,
    span: Dict[str, Any],
) -> Dict[str, Any]:
    """Wrap a span in the full OTLP/JSON resourceSpans envelope."""
    return {
        "resourceSpans": [{
            "resource": {"attributes": _resource_attrs(project_id, ingest_token, session_id, run_id)},
            "scopeSpans": [{
                "scope": {"name": HOOK_SCOPE_NAME, "version": HOOK_SCOPE_VERSION},
                "spans": [span],
            }],
        }]
    }


# ---------------------------------------------------------------------------
# OTLP HTTP send
# ---------------------------------------------------------------------------

def send_otlp(endpoint: str, payload: Dict[str, Any]) -> None:
    """POST OTLP/JSON to {endpoint}/v1/traces. Errors are silently ignored."""
    try:
        body = json.dumps(payload).encode()
        base = endpoint.rstrip("/")
        use_https = base.startswith("https://")
        host_part = base.removeprefix("https://").removeprefix("http://")
        if ":" in host_part:
            host, port_str = host_part.rsplit(":", 1)
            port = int(port_str)
        else:
            host, port = host_part, (443 if use_https else 80)
        Conn = http.client.HTTPSConnection if use_https else http.client.HTTPConnection
        conn = Conn(host, port, timeout=HTTP_TIMEOUT)
        conn.request(
            "POST", "/v1/traces", body=body,
            headers={"Content-Type": "application/json", "Content-Length": str(len(body))},
        )
        conn.getresponse()
        conn.close()
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Child worker
# ---------------------------------------------------------------------------

def _child_work(payload_env: str) -> None:
    """Executed in the detached child — builds and sends OTLP span."""
    try:
        data = json.loads(payload_env)
        event: str      = data["event"]
        session_id: str = data["session_id"]
        creds: Dict     = data["creds"]

        project_id   = creds["AGENTPULSE_PROJECT_ID"]
        endpoint     = creds["AGENTPULSE_ENDPOINT"]
        ingest_token = creds["AGENTPULSE_INGEST_TOKEN"]

        run_id   = get_or_create_run_id(session_id)
        trace_id = trace_id_from_run_id(run_id)
        span_id  = new_span_id()

        if event == "PostToolUse":
            tool_name     = data["tool_name"]
            tool_use_id   = data["tool_use_id"]
            cwd           = data.get("cwd", "")
            tool_input    = data.get("tool_input", {})
            tool_response = data.get("tool_response", {})
            end_ns        = time.time_ns()
            start_ns      = read_and_delete_start_time(tool_use_id)
            if start_ns is None:
                start_ns = end_ns
            span = build_tool_span(
                trace_id, span_id, tool_name, tool_use_id,
                cwd, tool_input, tool_response, start_ns, end_ns,
            )
        elif event == "Stop":
            now_ns = time.time_ns()
            span = build_stop_span(trace_id, span_id, now_ns)
            delete_run_id(session_id)
        else:
            return

        send_otlp(endpoint, build_payload(project_id, ingest_token, session_id, run_id, span))
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Parent helpers
# ---------------------------------------------------------------------------

def handle_pre_tool_use(hook_data: Dict[str, Any]) -> None:
    """Write start-time temp file — no network I/O, runs in parent."""
    tool_use_id = hook_data.get("tool_use_id", "")
    if tool_use_id:
        write_start_time(tool_use_id)


def fork_child(event: str, session_id: str, hook_data: Dict[str, Any], creds: Dict[str, str]) -> None:
    """Spawn a detached background process to send the OTLP span."""
    child_payload = {
        "event":         event,
        "session_id":    session_id,
        "creds":         creds,
        "tool_name":     hook_data.get("tool_name", ""),
        "tool_use_id":   hook_data.get("tool_use_id", ""),
        "cwd":           hook_data.get("cwd", ""),
        "tool_input":    hook_data.get("tool_input", {}),
        "tool_response": hook_data.get("tool_response", {}),
    }
    env = os.environ.copy()
    env["AGENTPULSE_HOOK_PAYLOAD"] = json.dumps(child_payload)
    subprocess.Popen(
        [sys.executable, __file__, "--child"],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
        env=env,
    )


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def main() -> None:
    if len(sys.argv) > 1 and sys.argv[1] == "--child":
        payload_env = os.environ.get("AGENTPULSE_HOOK_PAYLOAD", "")
        if payload_env:
            _child_work(payload_env)
        sys.exit(0)

    try:
        raw = sys.stdin.read()
        hook_data: Dict[str, Any] = json.loads(raw) if raw.strip() else {}
    except Exception:
        hook_data = {}

    event      = hook_data.get("hook_event_name", "")
    session_id = hook_data.get("session_id", "")

    if event == "PreToolUse":
        handle_pre_tool_use(hook_data)
        sys.exit(0)

    if event in ("PostToolUse", "Stop"):
        creds = load_credentials()
        if creds is not None:
            fork_child(event, session_id, hook_data, creds)

    sys.exit(0)


if __name__ == "__main__":
    main()
