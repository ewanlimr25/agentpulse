.PHONY: help dev-up dev-down dev-logs migrate-up migrate-down \
        collector-build collector-run backend-build backend-run \
        web-install web-dev web-build \
        test test-collector test-backend test-web \
        seed lint

SHELL := /bin/bash
export PATH := /opt/homebrew/bin:$(PATH)

# ── Infrastructure ────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

dev-up: ## Start local infrastructure (ClickHouse, Postgres, MinIO)
	docker compose up -d
	@echo "Waiting for services..."
	@docker compose exec postgres pg_isready -U agentpulse -q || true
	@echo "Infrastructure ready."

dev-down: ## Stop and remove local infrastructure
	docker compose down

dev-logs: ## Tail infrastructure logs
	docker compose logs -f

# ── Migrations ────────────────────────────────────────────────────────────────

migrate-up: ## Apply all pending migrations
	@echo "Applying Postgres migrations..."
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/001_initial.up.sql
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/002_eval_jobs.up.sql
	@echo "Applying ClickHouse migrations..."
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/001_spans.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/002_metrics_agg.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/003_run_metrics.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/004_span_evals.sql
	@echo "Migrations complete."

migrate-down: ## Roll back Postgres migrations
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/002_eval_jobs.down.sql
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/001_initial.down.sql

# ── Collector ─────────────────────────────────────────────────────────────────

collector-build: ## Build the OTel collector binary
	cd collector && go build ./...

collector-run: ## Run the OTel collector locally (requires dev-up)
	cd collector && go run ./cmd/collector/... --config config.dev.yaml

test-collector: ## Run collector unit tests
	cd collector && go test ./... -race -count=1

# ── Backend ───────────────────────────────────────────────────────────────────

backend-build: ## Build the backend API binary
	cd backend && go build ./...

backend-run: ## Run the backend API (requires dev-up + migrate-up)
	cd backend && go run ./cmd/server/...

test-backend: ## Run backend unit + integration tests
	cd backend && go test ./... -race -count=1

# ── Frontend ──────────────────────────────────────────────────────────────────

web-install: ## Install frontend dependencies
	cd web && npm install

web-dev: ## Start Next.js dev server
	cd web && npm run dev

web-build: ## Build the frontend for production
	cd web && npm run build

test-web: ## Run frontend tests
	cd web && npm test

# ── Tools ─────────────────────────────────────────────────────────────────────

db-reset: ## Truncate all app data (keeps schema; safe to re-seed)
	@echo "Resetting Postgres..."
	docker compose exec -T postgres psql -U agentpulse -d agentpulse -c \
		"TRUNCATE budget_alerts, budget_rules, topology_edges, topology_nodes, projects CASCADE;"
	docker compose exec -T postgres psql -U agentpulse -d agentpulse -c \
		"TRUNCATE eval_jobs;"
	@echo "Resetting ClickHouse..."
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse --query "TRUNCATE TABLE spans;"
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse --query "TRUNCATE TABLE span_evals;"
	@echo "Database reset complete."

seed: db-reset ## Create demo projects via API and seed with realistic multi-agent runs
	go run ./tools/tracegen/... --demo

# ── Combined ─────────────────────────────────────────────────────────────────

test: test-collector test-backend test-web ## Run all tests

lint: ## Run linters across all services
	cd collector && go vet ./...
	cd backend && go vet ./...
	cd web && npm run lint
