# AgentPulse — Claude Code Hook

Automatically trace every Claude Code tool call (Bash, Edit, Read, …) as
OpenTelemetry spans and send them to AgentPulse. This gives you a full
session timeline in the AgentPulse dashboard with per-tool durations, inputs,
outputs, and cost attribution.

## How it works

Claude Code fires shell hooks on three events:

| Event | What the hook does |
|---|---|
| `PreToolUse` | Records the wall-clock start time for the tool call |
| `PostToolUse` | Computes duration, builds an OTLP span, sends it to AgentPulse |
| `Stop` | Emits a session-stop span and cleans up session state |

The hook script forks a detached background process for network I/O so it
never blocks Claude Code. The parent exits 0 immediately.

## Requirements

- Python 3.8+ (stdlib only — no pip installs required)
- A running AgentPulse instance (local or cloud)
- A project created in AgentPulse with an ingest token

## Installation

```bash
cd tools/claude-code-hook
chmod +x install.sh
./install.sh
```

The installer will prompt for:

| Credential | Description |
|---|---|
| Project ID | UUID shown on your AgentPulse project page |
| OTLP endpoint | Base URL for your AgentPulse collector (default: `http://localhost:4318`) |
| Ingest token | Bearer token for the project |

After installation, **restart Claude Code** for the hooks to take effect.

## What gets installed

| Path | Description |
|---|---|
| `~/.agentpulse/hook/agentpulse_hook.py` | Hook script |
| `~/.agentpulse/credentials` | Credentials file (mode 600) |
| `~/.agentpulse/tmp/` | Per-call start-time state |
| `~/.agentpulse/run/` | Per-session run ID state |
| `~/.claude/settings.json` | Hook entries added (existing config preserved) |

## Verifying it works

1. Open Claude Code and run any tool (e.g. ask it to list files).
2. Open the AgentPulse dashboard → **Sessions** tab.
3. You should see a new session entry with tool spans appearing in real time.

## Credentials file format

`~/.agentpulse/credentials` is a plain key=value file:

```
AGENTPULSE_PROJECT_ID=<uuid>
AGENTPULSE_ENDPOINT=http://localhost:4318
AGENTPULSE_INGEST_TOKEN=<token>
```

Edit this file directly to change credentials at any time — no reinstall needed.

## Uninstalling

1. Remove the hook entries from `~/.claude/settings.json` (the three entries
   under `PreToolUse`, `PostToolUse`, and `Stop` that reference
   `agentpulse_hook.py`).
2. Delete the AgentPulse state directory:
   ```bash
   rm -rf ~/.agentpulse
   ```
