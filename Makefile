.PHONY: help dev-up dev-down dev-logs migrate-up migrate-down \
        collector-build collector-run backend-build backend-run \
        web-install web-dev web-build \
        test test-collector test-backend test-web test-sdk-ts \
        sdk-ts-install sdk-ts-codegen sdk-ts-build \
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
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/005_eval_prompt_version.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/006_session_id.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/007_run_metrics_session.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/008_session_agg.sql
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/004_project_eval_configs.up.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/009_user_id.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/010_run_metrics_user.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/011_user_agg.sql
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < migrations/postgres/005_budget_scope_user.up.sql
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < migrations/clickhouse/012_search_indexes.sql
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
	@set -o allexport; [ -f .env ] && . ./.env || true; set +o allexport; cd backend && go run ./cmd/server/...

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
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse --query "TRUNCATE TABLE session_agg;"
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse --query "TRUNCATE TABLE user_agg;"
	@echo "Database reset complete."

seed: db-reset ## Create demo projects via API and seed with realistic multi-agent runs
	go run ./tools/tracegen/... --demo
	@echo "Waiting for spans to land in ClickHouse..."
	@sleep 5
	$(MAKE) seed-evals

seed-evals: ## Insert mock eval scores (multi-type) and eval configs for all demo projects
	@echo "Inserting mock eval scores..."
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < scripts/seed-evals.sql
	@echo "Inserting eval trend data (5 synthetic runs per project)..."
	docker compose exec -T clickhouse clickhouse-client --user agentpulse --password agentpulse \
		--database agentpulse < scripts/seed-eval-trend.sql
	@echo "Inserting eval configs per project..."
	docker compose exec -T postgres psql -U agentpulse -d agentpulse < scripts/seed-eval-configs.sql
	@echo "Mock evals inserted."

# ── TypeScript SDK ────────────────────────────────────────────────────────────

sdk-ts-install: ## Install TypeScript SDK dependencies
	cd sdk/typescript && npm install

sdk-ts-codegen: ## Regenerate attribute constants from config/agent_attributes.yaml
	cd sdk/typescript && npm run codegen

sdk-ts-build: sdk-ts-codegen ## Build the TypeScript SDK (runs codegen first)
	cd sdk/typescript && npm run build

test-sdk-ts: sdk-ts-codegen ## Run TypeScript SDK tests
	cd sdk/typescript && npm test

# ── Combined ─────────────────────────────────────────────────────────────────

test: test-collector test-backend test-web test-sdk-ts ## Run all tests

lint: ## Run linters across all services
	cd collector && go vet ./...
	cd backend && go vet ./...
	cd web && npm run lint
	cd sdk/typescript && npm run lint
