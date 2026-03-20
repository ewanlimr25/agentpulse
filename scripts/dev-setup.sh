#!/usr/bin/env bash
# AgentPulse dev environment setup
# Run once after cloning the repo.

set -euo pipefail

echo "==> Checking prerequisites..."

# Go
if ! command -v go &>/dev/null; then
  echo "  Installing Go via Homebrew..."
  brew install go
fi
echo "  Go: $(go version)"

# Node
if ! command -v node &>/dev/null; then
  echo "  Node not found. Install via nvm: nvm install --lts"
  exit 1
fi
echo "  Node: $(node --version)"

# Docker
if ! command -v docker &>/dev/null; then
  echo "  Docker not found. Install Docker Desktop: https://www.docker.com/products/docker-desktop/"
  exit 1
fi
echo "  Docker: $(docker --version)"

echo ""
echo "==> Installing frontend dependencies..."
cd web && npm install && cd ..

echo ""
echo "==> Starting infrastructure..."
docker compose up -d

echo ""
echo "==> Waiting for services to be healthy..."
sleep 5

echo ""
echo "==> Applying migrations..."
make migrate-up

echo ""
echo "==> Done! Run 'make seed' to generate synthetic trace data."
echo "    Backend: make backend-run"
echo "    Frontend: make web-dev"
echo "    Collector: make collector-run"
