# Claude Code Integration — Agent Logging Guide

Automatically capture every Claude Code tool call (Bash, Edit, Read, Grep, …)
as an OpenTelemetry span in AgentPulse. Sessions appear in the dashboard within
seconds, with per-tool durations, inputs, outputs, and cost attribution.

---

## How it works

Claude Code supports shell hooks that fire on every tool event. The AgentPulse
hook script taps into three events:

| Event | What happens |
|---|---|
| `PreToolUse` | Records the wall-clock start time for the upcoming tool call |
| `PostToolUse` | Computes duration, builds an OTLP span, sends it to the collector |
| `Stop` | Emits a session-stop sentinel span, cleans up local session state |

Each Claude Code **session** (one `claude` invocation) becomes a **Run** in
AgentPulse. Multiple runs that share the same Claude Code `session_id` are
grouped into a **Session** in the Sessions view.

The hook script forks a detached background process for all network I/O and
exits 0 immediately — Claude Code is never blocked by a slow collector or
network hiccup.

---

## Prerequisites

- Claude Code installed and working (`claude --version`)
- Python 3.8+ available at `python3` (no third-party packages needed)
- AgentPulse running — locally or in the cloud
- A project created in the AgentPulse UI

---

## Step 1 — Create an ingest token

Ingest tokens are separate from your project API key. They are scoped to
OTLP span ingest only and are safe to store on your local machine.

**Via the UI:**

1. Open your project in AgentPulse → **Settings** → **Ingest Tokens**
2. Click **New Token**, give it a label (e.g. `claude-code-macbook`)
3. Copy the raw token — it is shown only once

**Via the API:**

```bash
curl -X POST http://localhost:8080/api/v1/projects/<PROJECT_ID>/ingest-tokens \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"label": "claude-code-macbook"}'
```

Response:

```json
{
  "token": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "id": "...",
  "label": "claude-code-macbook",
  "created_at": "2026-04-23T10:00:00Z"
}
```

Save the `token` value — you will need it in Step 2.

---

## Step 2 — Run the installer

From the AgentPulse repository root:

```bash
cd tools/claude-code-hook
chmod +x install.sh
./install.sh
```

The installer will prompt for three values:

| Prompt | Value |
|---|---|
| Project ID | UUID shown on your AgentPulse project page |
| OTLP endpoint | Collector base URL (default: `http://localhost:4318`) |
| Ingest token | Token created in Step 1 |

What the installer does:

- Copies `agentpulse_hook.py` to `~/.agentpulse/hook/`
- Writes your credentials to `~/.agentpulse/credentials` (mode 600)
- Merges hook entries into `~/.claude/settings.json` without overwriting
  any existing hooks you may have configured

The installer is **idempotent** — re-running it updates credentials and
ensures the hook entries are present without duplicating them.

---

## Step 3 — Restart Claude Code

Hook configuration is read at startup. Quit and reopen Claude Code (or start
a new session) for the hooks to take effect.

---

## Step 4 — Verify

1. Open Claude Code and run any tool — for example, ask it to list files in
   your project.
2. Open AgentPulse → **Sessions** tab.
3. A new session should appear within a few seconds, with tool spans showing
   the tool name, duration, and input/output.

If no session appears, see [Troubleshooting](#troubleshooting) below.

---

## What each span contains

Every `PostToolUse` event produces one span with the following attributes:

| Attribute | Value |
|---|---|
| `tool.name` | Claude Code tool name (e.g. `Bash`, `Edit`, `Read`) |
| `tool.input` | JSON-encoded tool input |
| `tool.output` | JSON-encoded tool response |
| `agentpulse.span_kind` | `tool.call` |
| `agentpulse.session_id` | Claude Code session ID |
| `agentpulse.run_id` | Stable UUID for this `claude` process invocation |
| `agentpulse.agent.name` | `claude-code` |
| `claude_code.tool_use_id` | Per-invocation ID from Claude Code |
| `claude_code.cwd` | Working directory at time of tool call |

The `Stop` event produces a `claude_code.session.stop` sentinel span used to
mark session end in the timeline view.

---

## Cloud vs self-hosted

The integration works identically for both deployment models. The only
difference is the OTLP endpoint you provide during installation:

| Deployment | Endpoint |
|---|---|
| Local dev | `http://localhost:4318` |
| Self-hosted server | `http://your-server:4318` |
| Cloud-hosted | `https://collector.your-agentpulse-domain.com` |

For cloud-hosted instances, ingest token validation is enforced at the
collector. Spans without a valid token are dropped before they reach storage.
For self-hosted instances, validation is enabled by default but can be
disabled via the collector config (`authenforceproc.enabled: false`).

---

## Updating credentials

Edit `~/.agentpulse/credentials` directly — no reinstall needed:

```
AGENTPULSE_PROJECT_ID=<uuid>
AGENTPULSE_ENDPOINT=http://localhost:4318
AGENTPULSE_INGEST_TOKEN=<token>
```

Changes take effect on the next tool call (the hook script re-reads the file
on every invocation).

---

## Revoking a token

```bash
# List tokens
curl http://localhost:8080/api/v1/projects/<PROJECT_ID>/ingest-tokens \
  -H "Authorization: Bearer <API_KEY>"

# Revoke by ID
curl -X DELETE \
  http://localhost:8080/api/v1/projects/<PROJECT_ID>/ingest-tokens/<TOKEN_ID> \
  -H "Authorization: Bearer <API_KEY>"
```

After revoking, update `~/.agentpulse/credentials` with a new token or the
hook will fail silently (spans are dropped at the collector).

---

## Troubleshooting

**No sessions appearing in the dashboard**

1. Check that `~/.agentpulse/credentials` exists and has all three keys:
   ```bash
   cat ~/.agentpulse/credentials
   ```
2. Confirm the collector is reachable:
   ```bash
   curl -s -o /dev/null -w "%{http_code}" \
     http://localhost:4318/v1/traces \
     -X POST -H "Content-Type: application/json" -d '{}'
   ```
   Expect `400` (bad request) — any response means the collector is up.
3. Confirm the hooks are registered:
   ```bash
   cat ~/.claude/settings.json | python3 -m json.tool | grep agentpulse
   ```
   You should see three entries referencing `agentpulse_hook.py`.
4. Run the hook script manually with a test payload:
   ```bash
   echo '{"hook_event_name":"PostToolUse","session_id":"test","tool_name":"Bash","tool_use_id":"test-id","tool_input":{},"tool_response":{}}' \
     | python3 ~/.agentpulse/hook/agentpulse_hook.py
   ```
   It should exit 0 with no output.

**Spans appearing but no duration data**

The `PreToolUse` hook may not be registered. Check `~/.claude/settings.json`
for a `PreToolUse` entry. Re-run the installer to add it.

**Session ID keeps changing between runs**

This is expected — each `claude` process invocation gets a new run ID.
Multiple runs sharing the same Claude Code `session_id` are grouped into one
Session in the AgentPulse Sessions view.

---

## Uninstalling

```bash
# Remove hook entries from Claude Code config
# Edit ~/.claude/settings.json and delete the three agentpulse_hook.py entries
# under PreToolUse, PostToolUse, and Stop.

# Remove local state
rm -rf ~/.agentpulse
```
