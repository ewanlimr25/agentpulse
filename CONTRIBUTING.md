# Contributing to AgentPulse

Thanks for helping build AgentPulse! This guide covers development setup, testing, linting, and our commit conventions.

## Dev Setup

### Prerequisites

- **Docker** (for ClickHouse, Postgres, MinIO)
- **Go** 1.22 or later
- **Node.js** (via nvm; run `nvm use` in the repo)
- **Python** 3.12+ (via pyenv)
- **jq** and **make**

### Quick Start

The fastest way to get running:

```bash
chmod +x scripts/init.sh && ./scripts/init.sh
```

### Manual Setup (if the script fails)

1. Start infrastructure:

```bash
make dev-up
```

This launches ClickHouse (9000/8123), Postgres (5432), and MinIO (9090) as Docker containers.

2. Apply database migrations:

```bash
make migrate-up
```

3. Start the backend API:

```bash
make backend-run
```

The API will listen on `http://localhost:8080`.

4. In a new terminal, start the frontend:

```bash
cd web && npm install && npm run dev
```

Next.js will start on `http://localhost:3000`.

### Environment Configuration

Copy `.env.example` to `.env` before your first run:

```bash
cp .env.example .env
```

Edit `.env` to override defaults. Key variables:
- `POSTGRES_DSN` — Postgres connection (default: `postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable`)
- `CLICKHOUSE_ADDR` — ClickHouse address (default: `localhost:9000`)
- `NEXT_PUBLIC_API_URL` — Frontend's API URL (default: `http://localhost:8080`)

For eval judges and the prompt playground, add:
- `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `GOOGLE_AI_API_KEY`

## Commit Conventions

We follow conventional commits. Keep the subject line under 72 characters with no period.

**Format:**

```
<type>: <description>

<optional body>
```

**Types in use:** `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`

**Recent examples from the repo:**

```
docs: comprehensive audit, tutorials, and licensing
feat: storage management — retention config, usage stats, and manual purge
docs: Claude Code integration guide — setup, token creation, verification, troubleshooting
feat: Claude Code hook integration — zero-config observability for Claude Code sessions
feat: notification channels — Slack, Discord, browser push, email digest
```

## Testing

All tests must pass before opening a PR.

### Backend

```bash
cd backend && go test ./...
cd backend && go test -race ./...
```

### Frontend

```bash
cd web && npm test
```

### TypeScript SDK

```bash
cd sdk/typescript && npm test
```

### Python SDK

```bash
cd sdk/python && python -m pytest
```

### All Tests

Run everything at once:

```bash
make test
```

## Lint

Before committing, run the linters:

### Go

```bash
cd backend && go vet ./...
cd backend && gofmt -l .
```

No output from `gofmt` means clean formatting.

Also check the collector:

```bash
cd collector && go vet ./...
```

### TypeScript

```bash
cd web && npm run lint
cd sdk/typescript && npm run lint
```

**Rule:** No `console.log` in production code.

### All Linters

```bash
make lint
```

## PR Checklist

Before opening a pull request:

- [ ] Tests pass (`make test`)
- [ ] No linter warnings (`make lint`)
- [ ] Migration files included if schema changed (both `migrations/postgres/` and `migrations/clickhouse/`)
- [ ] No hardcoded secrets, API keys, or tokens
- [ ] PR description explains the **why**, not just the **what**

## Common Tasks

### Reset demo data

Truncate all tables (schema remains):

```bash
make db-reset
```

### Seed demo projects

Create demo projects and insert realistic multi-agent runs:

```bash
make seed
```

### View infrastructure logs

```bash
make dev-logs
```

### Build the CLI

```bash
make cli-build
```

Binary appears at `tools/agentpulse-cli`.

## Questions?

Check the Makefile for all available targets:

```bash
make help
```
