#!/usr/bin/env bash
# AgentPulse Bootstrap Script
# One-command initialization for new cloners
# After running this, start the backend with: make backend-run
# Then start the frontend with: cd web && npm run dev

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MARKER_FILE="$REPO_ROOT/.agentpulse-initialized"

# ANSI color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
AMBER='\033[0;33m'
RESET='\033[0m'

# ── Step 1: Check for existing initialization ────────────────────────────────

if [ -f "$MARKER_FILE" ]; then
    echo "Already initialized. Delete $MARKER_FILE to re-run."
    exit 0
fi

# ── Step 2: Generate random credentials ───────────────────────────────────────

ENV_FILE="$REPO_ROOT/.env"
OVERRIDE_FILE="$REPO_ROOT/docker-compose.override.yml"

# Check whether .env already has non-default credentials
if [ -f "$ENV_FILE" ] && ! grep -q "agentpulse:agentpulse" "$ENV_FILE"; then
    echo "Credentials already set in $ENV_FILE — skipping credential generation."
else
    echo "Generating random credentials..."
    POSTGRES_PASSWORD="$(openssl rand -hex 32)"
    CLICKHOUSE_PASSWORD="$(openssl rand -hex 32)"
    S3_SECRET_KEY="$(openssl rand -hex 32)"

    # Write (or overwrite) credential lines in .env
    # Remove any existing credential lines so we can append fresh ones
    if [ -f "$ENV_FILE" ]; then
        # Remove lines for the three keys before re-appending
        TMP_ENV="$(grep -v '^POSTGRES_DSN=' "$ENV_FILE" | grep -v '^CLICKHOUSE_PASSWORD=' | grep -v '^S3_SECRET_KEY=')"
        printf '%s\n' "$TMP_ENV" > "$ENV_FILE"
    fi
    cat >> "$ENV_FILE" <<ENVEOF
POSTGRES_DSN=postgres://agentpulse:${POSTGRES_PASSWORD}@localhost:5432/agentpulse?sslmode=disable
CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}
S3_SECRET_KEY=${S3_SECRET_KEY}
ENVEOF
    echo "Credentials written to $ENV_FILE"
fi

# Write docker-compose.override.yml if it doesn't already exist
if [ ! -f "$OVERRIDE_FILE" ]; then
    # Read credentials from .env for the override file
    POSTGRES_PASSWORD="$(grep '^POSTGRES_DSN=' "$ENV_FILE" | sed 's|.*agentpulse:\([^@]*\)@.*|\1|')"
    CLICKHOUSE_PASSWORD="$(grep '^CLICKHOUSE_PASSWORD=' "$ENV_FILE" | cut -d= -f2)"
    cat > "$OVERRIDE_FILE" <<OVERRIDEEOF
services:
  postgres:
    environment:
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
  clickhouse:
    environment:
      CLICKHOUSE_PASSWORD: ${CLICKHOUSE_PASSWORD}
OVERRIDEEOF
    echo "Credentials written to $OVERRIDE_FILE (gitignored)"
else
    echo "$OVERRIDE_FILE already exists — skipping."
fi

echo ""
echo "Credentials are stored in:"
echo "  $ENV_FILE  (loaded by the backend at runtime)"
echo "  $OVERRIDE_FILE  (picked up by docker compose automatically)"
echo ""

# ── Step 3: Start Docker services ─────────────────────────────────────────────

echo "Starting Docker services (ClickHouse, Postgres, MinIO, OTel Collector)..."
cd "$REPO_ROOT"
docker compose up -d --wait 2>&1 | grep -v "already in use" || true
echo "Docker services started."

# ── Step 4: Run database migrations ───────────────────────────────────────────

echo "Applying database migrations..."
make migrate-up
echo "Database migrations complete."

# ── Step 5: Poll backend health endpoint ──────────────────────────────────────

echo "Waiting for backend to be ready (max 30 attempts, 2s between)..."

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: jq is required (brew install jq)${RESET}"
    exit 1
fi

BACKEND_READY=0
for attempt in {1..30}; do
    if curl -s -f http://localhost:8080/healthz > /dev/null 2>&1; then
        BACKEND_READY=1
        echo "Backend is ready."
        break
    fi
    if [ $attempt -lt 30 ]; then
        sleep 2
    fi
done

if [ $BACKEND_READY -eq 0 ]; then
    echo ""
    echo -e "${RED}Error: Backend did not start after 30 attempts.${RESET}"
    echo "The backend needs to be running on port 8080."
    echo ""
    echo "To start the backend manually:"
    echo "  make backend-run"
    echo ""
    echo "Then create a project via:"
    echo "  curl -X POST http://localhost:8080/api/v1/projects \\"
    echo "    -H 'Content-Type: application/json' \\"
    echo "    -d '{\"name\":\"demo\"}'"
    echo ""
    echo "After the backend is running, re-run this script."
    exit 1
fi

# ── Step 6: Create demo project ───────────────────────────────────────────────

echo "Creating demo project..."

RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/projects \
    -H 'Content-Type: application/json' \
    -d '{"name":"demo"}')

# Extract values from response
PROJECT_ID=$(echo "$RESPONSE" | jq -r '.project.ID // empty')
API_KEY=$(echo "$RESPONSE" | jq -r '.api_key // empty')
ADMIN_KEY=$(echo "$RESPONSE" | jq -r '.admin_key // empty')

if [ -z "$PROJECT_ID" ] || [ -z "$API_KEY" ] || [ -z "$ADMIN_KEY" ]; then
    echo -e "${RED}Error: Failed to create project. Response: $RESPONSE${RESET}"
    exit 1
fi

echo "Project created: $PROJECT_ID"

# ── Step 7: Write web/.env.local ──────────────────────────────────────────────

WEB_ENV_FILE="$REPO_ROOT/web/.env.local"

echo "Writing $WEB_ENV_FILE..."

cat > "$WEB_ENV_FILE" <<EOF
NEXT_PUBLIC_API_URL=http://localhost:8080
AGENTPULSE_PROJECT_ID=$PROJECT_ID
AGENTPULSE_API_KEY=$API_KEY
EOF

# Verify .env.local is in .gitignore
if ! grep -q "\.env\.local" "$REPO_ROOT/.gitignore"; then
    echo -e "${AMBER}Warning: web/.env.local may not be in .gitignore${RESET}"
fi

# ── Step 8: Success banner ────────────────────────────────────────────────────

echo ""
echo "╔════════════════════════════════════════════════════════════════════════╗"
echo "║                                                                        ║"
echo "║                    AgentPulse Initialization Complete                 ║"
echo "║                                                                        ║"
echo "╚════════════════════════════════════════════════════════════════════════╝"
echo ""
echo "Project ID:"
echo "  $PROJECT_ID"
echo ""
echo -e "${GREEN}API Key (store securely):${RESET}"
echo "  $API_KEY"
echo ""
echo -e "${AMBER}Admin Key (store securely — shown once):${RESET}"
echo "  $ADMIN_KEY"
echo ""
echo "Next steps:"
echo "  1. Start the backend (new terminal):"
echo "       make backend-run"
echo ""
echo "  2. Start the frontend:"
echo "       cd web && npm run dev"
echo ""
echo "Local dashboard: http://localhost:3000"
echo ""

# ── Step 9: Create marker file ────────────────────────────────────────────────

touch "$MARKER_FILE"

exit 0
