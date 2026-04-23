#!/usr/bin/env bash
# AgentPulse Claude Code hook installer
# Copies the hook script, writes credentials, and merges hooks into
# ~/.claude/settings.json without overwriting any existing configuration.

set -euo pipefail

HOOK_DIR="$HOME/.agentpulse/hook"
TMP_DIR="$HOME/.agentpulse/tmp"
RUN_DIR="$HOME/.agentpulse/run"
CREDS_FILE="$HOME/.agentpulse/credentials"
SETTINGS_FILE="$HOME/.claude/settings.json"
HOOK_SCRIPT="$HOOK_DIR/agentpulse_hook.py"
HOOK_CMD="python3 $HOOK_SCRIPT"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_HOOK="$SCRIPT_DIR/agentpulse_hook.py"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

info()    { echo "  [agentpulse] $*"; }
success() { echo "  [agentpulse] ✓ $*"; }
error()   { echo "  [agentpulse] ✗ $*" >&2; exit 1; }

prompt_value() {
    local label="$1"
    local default="${2:-}"
    local value=""
    if [[ -n "$default" ]]; then
        read -rp "  $label [$default]: " value
        echo "${value:-$default}"
    else
        while [[ -z "$value" ]]; do
            read -rp "  $label: " value
        done
        echo "$value"
    fi
}

# ---------------------------------------------------------------------------
# 1. Check Python 3
# ---------------------------------------------------------------------------

if ! command -v python3 &>/dev/null; then
    error "Python 3 is required but was not found in PATH."
fi
PY_VERSION=$(python3 -c "import sys; print('%d.%d' % sys.version_info[:2])")
info "Found Python $PY_VERSION"

# ---------------------------------------------------------------------------
# 2. Create directories and copy hook script
# ---------------------------------------------------------------------------

mkdir -p "$HOOK_DIR" "$TMP_DIR" "$RUN_DIR"

if [[ ! -f "$SRC_HOOK" ]]; then
    error "Source hook script not found: $SRC_HOOK"
fi

cp "$SRC_HOOK" "$HOOK_SCRIPT"
chmod +x "$HOOK_SCRIPT"
success "Hook script installed to $HOOK_SCRIPT"

# ---------------------------------------------------------------------------
# 3. Collect credentials
# ---------------------------------------------------------------------------

echo ""
echo "  AgentPulse credentials"
echo "  ─────────────────────────────────────────"

PROJECT_ID=$(prompt_value "Project ID (UUID)")
ENDPOINT=$(prompt_value "OTLP endpoint" "http://localhost:4318")
INGEST_TOKEN=$(prompt_value "Ingest token")

# Write credentials file with restricted permissions.
cat > "$CREDS_FILE" <<EOF
AGENTPULSE_PROJECT_ID=$PROJECT_ID
AGENTPULSE_ENDPOINT=$ENDPOINT
AGENTPULSE_INGEST_TOKEN=$INGEST_TOKEN
EOF
chmod 600 "$CREDS_FILE"
success "Credentials written to $CREDS_FILE (mode 600)"

# ---------------------------------------------------------------------------
# 4. Merge hooks into ~/.claude/settings.json
# ---------------------------------------------------------------------------

# Ensure the settings file and its parent directory exist.
mkdir -p "$(dirname "$SETTINGS_FILE")"
if [[ ! -f "$SETTINGS_FILE" ]]; then
    echo '{}' > "$SETTINGS_FILE"
    info "Created $SETTINGS_FILE"
fi

python3 - "$SETTINGS_FILE" "$HOOK_CMD" <<'PYEOF'
import json
import sys

settings_path = sys.argv[1]
hook_cmd      = sys.argv[2]

with open(settings_path, "r") as f:
    try:
        settings = json.load(f)
    except json.JSONDecodeError:
        settings = {}

hooks = settings.setdefault("hooks", {})

# ---------------------------------------------------------------------------
# Idempotent merge: add our hook entry only if not already present.
# ---------------------------------------------------------------------------

def hook_entry_present(entries: list, cmd: str) -> bool:
    """Return True if any entry in the list already references cmd."""
    for entry in entries:
        for h in entry.get("hooks", []):
            if h.get("command") == cmd:
                return True
    return False

agentpulse_hook = {"type": "command", "command": hook_cmd}

# PreToolUse
pre = hooks.setdefault("PreToolUse", [])
if not hook_entry_present(pre, hook_cmd):
    pre.append({"matcher": ".*", "hooks": [agentpulse_hook]})

# PostToolUse
post = hooks.setdefault("PostToolUse", [])
if not hook_entry_present(post, hook_cmd):
    post.append({"matcher": ".*", "hooks": [agentpulse_hook]})

# Stop (no matcher field required)
stop = hooks.setdefault("Stop", [])
if not hook_entry_present(stop, hook_cmd):
    stop.append({"hooks": [agentpulse_hook]})

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")

print("  [agentpulse] ✓ Hooks merged into " + settings_path)
PYEOF

# ---------------------------------------------------------------------------
# 5. Done
# ---------------------------------------------------------------------------

echo ""
echo "  ┌─────────────────────────────────────────────┐"
echo "  │  AgentPulse hook installed successfully     │"
echo "  │                                             │"
echo "  │  Endpoint : $ENDPOINT"
printf "  │  %-45s│\n" ""
echo "  │  Restart Claude Code for hooks to take      │"
echo "  │  effect, then check the Sessions tab in     │"
echo "  │  the AgentPulse dashboard.                  │"
echo "  └─────────────────────────────────────────────┘"
echo ""
