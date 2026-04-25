# AgentPulse Deployment Guide

## Overview

AgentPulse has three deployable services:

| Service | Path | Default port | Description |
|---------|------|-------------|-------------|
| **Backend API** | `backend/` | 8080 | Go/chi REST + WebSocket API |
| **OTel Collector** | `collector/` | 4317 (gRPC) / 4318 (HTTP) | Custom OpenTelemetry collector |
| **Web UI** | `web/` | 3000 | Next.js 15 frontend |

External dependencies that must be provisioned separately:

- **Postgres** — topology, projects, budgets, alerts, evals, tokens
- **ClickHouse** — spans, metrics, sessions, user aggregates
- **S3-compatible object store** — span payload offload (MinIO works locally; S3 or R2 in production)

---

## Prerequisites

Provision these managed services before deploying the application:

### Postgres

**Recommended:** [Neon](https://neon.tech) — serverless Postgres with a generous free tier.

1. Create a new project at neon.tech.
2. Copy the connection string (format: `postgres://user:pass@host/dbname?sslmode=require`).
3. Set it as `POSTGRES_DSN` in your deployment environment.

Alternative: any Postgres 15+ instance (RDS, Supabase, Railway Postgres add-on, or self-hosted).

### ClickHouse

**Recommended:** [ClickHouse Cloud](https://clickhouse.cloud) — fully managed, pay-per-use.

1. Create a new service; choose the region closest to your API servers.
2. Note the hostname and port (native TCP is 9440 with TLS in ClickHouse Cloud).
3. Set `CLICKHOUSE_ADDR`, `CLICKHOUSE_USER`, and `CLICKHOUSE_PASSWORD`.

Alternative: self-hosted ClickHouse on a VPS (see Bare VPS section below).

### Object Storage

Any S3-compatible endpoint works: AWS S3, Cloudflare R2, Backblaze B2, MinIO.

Set `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, and `S3_SECRET_KEY`. Create a bucket named `agentpulse-spans` (or override with `S3_BUCKET`). For AWS S3 leave `S3_ENDPOINT` empty.

---

## Building the Docker Image

The `Dockerfile` at the repository root builds a minimal Alpine image for the backend API.

```bash
# From the repository root
docker build -t agentpulse-backend:latest .

# Push to a registry
docker tag agentpulse-backend:latest ghcr.io/<org>/agentpulse-backend:latest
docker push ghcr.io/<org>/agentpulse-backend:latest
```

The build requires Go workspace files (`go.work`, `go.work.sum`) at the repository root. Build must be run from the repository root, not from `backend/`.

---

## Fly.io

> **Note:** The configuration below is an untested template. Verify with `fly status` and `fly logs` after the first deploy.

Fly.io is a good fit for the backend API: it supports persistent TCP connections (WebSockets), health checks, and secret management out of the box.

### fly.toml

Place this file at the repository root alongside the `Dockerfile`.

```toml
app = "agentpulse-backend"
primary_region = "lax"

[build]
  dockerfile = "Dockerfile"

[env]
  APP_ENV         = "production"
  HTTP_HOST       = "0.0.0.0"
  HTTP_PORT       = "8080"
  CLICKHOUSE_DATABASE = "agentpulse"
  S3_ENFORCE_HTTPS    = "true"

[http_service]
  internal_port        = 8080
  force_https          = true
  auto_stop_machines   = true
  auto_start_machines  = true
  min_machines_running = 1

  [[http_service.checks]]
    grace_period  = "10s"
    interval      = "15s"
    method        = "GET"
    path          = "/healthz"
    timeout       = "5s"

[[vm]]
  cpu_kind = "shared"
  cpus     = 1
  memory   = "512mb"
```

### Deploy steps

```bash
# 1. Authenticate and create the app (first time only)
fly auth login
fly launch --no-deploy --copy-config

# 2. Set secrets (never commit these)
fly secrets set \
  POSTGRES_DSN="postgres://user:pass@host/db?sslmode=require" \
  CLICKHOUSE_ADDR="host:9440" \
  CLICKHOUSE_USER="default" \
  CLICKHOUSE_PASSWORD="..." \
  S3_ENDPOINT="https://..." \
  S3_ACCESS_KEY="..." \
  S3_SECRET_KEY="..." \
  CORS_ALLOWED_ORIGINS="https://your-frontend.com" \
  ANTHROPIC_API_KEY="sk-ant-..."

# 3. Deploy
fly deploy

# 4. Verify
fly status
fly logs
curl https://agentpulse-backend.fly.dev/healthz
```

### Collector on Fly.io

The OTel collector has its own `Dockerfile` at `collector/`. Create a separate Fly app:

```bash
cd collector
fly launch --no-deploy
fly secrets set CLICKHOUSE_ENDPOINT="..." POSTGRES_DSN="..."
fly deploy
```

---

## Railway

> **Note:** Untested template — verify env vars match those listed in the reference table below.

Railway auto-detects the `Dockerfile` at the repository root.

### railway.json

```json
{
  "$schema": "https://railway.app/railway.schema.json",
  "build": {
    "builder": "DOCKERFILE",
    "dockerfilePath": "Dockerfile"
  },
  "deploy": {
    "startCommand": "/app/agentpulse-server",
    "healthcheckPath": "/healthz",
    "healthcheckTimeout": 10,
    "restartPolicyType": "ON_FAILURE",
    "restartPolicyMaxRetries": 5
  }
}
```

### Environment variables to set in the Railway dashboard

Set the following in **Settings → Variables**:

```
APP_ENV=production
POSTGRES_DSN=<connection string from Railway Postgres add-on or Neon>
CLICKHOUSE_ADDR=<host:port>
CLICKHOUSE_USER=<user>
CLICKHOUSE_PASSWORD=<password>
CLICKHOUSE_DATABASE=agentpulse
S3_ENDPOINT=<endpoint url>
S3_BUCKET=agentpulse-spans
S3_ACCESS_KEY=<key>
S3_SECRET_KEY=<secret>
S3_ENFORCE_HTTPS=true
CORS_ALLOWED_ORIGINS=https://your-frontend.com
```

Railway exposes the service on a generated `*.up.railway.app` hostname over HTTPS — no extra TLS configuration needed.

---

## Google Cloud Run

> **Note:** Cloud Run is stateless — ClickHouse and Postgres must be external (Cloud Run cannot host them). The template below is untested.

### Deploy command

```bash
gcloud run deploy agentpulse-backend \
  --image ghcr.io/<org>/agentpulse-backend:latest \
  --platform managed \
  --region us-west1 \
  --port 8080 \
  --allow-unauthenticated \
  --min-instances 1 \
  --max-instances 10 \
  --memory 512Mi \
  --cpu 1 \
  --set-env-vars APP_ENV=production,CLICKHOUSE_DATABASE=agentpulse,S3_ENFORCE_HTTPS=true \
  --set-secrets \
    POSTGRES_DSN=agentpulse-postgres-dsn:latest,\
    CLICKHOUSE_ADDR=agentpulse-clickhouse-addr:latest,\
    CLICKHOUSE_PASSWORD=agentpulse-clickhouse-password:latest,\
    S3_ACCESS_KEY=agentpulse-s3-access-key:latest,\
    S3_SECRET_KEY=agentpulse-s3-secret-key:latest
```

Create the secrets first with `gcloud secrets create <name> --data-file=-` or through the Secret Manager console.

### Health check

Cloud Run uses HTTP health checks automatically. The `/healthz` endpoint is available immediately after startup.

### WebSocket note

Cloud Run supports WebSocket connections. Set `--timeout 3600` if you have long-lived WebSocket sessions.

---

## Bare VPS (Ubuntu / Debian)

Use this approach when you want full control over ClickHouse or are self-hosting everything on a single machine.

### docker-compose.prod.yml

This file uses pre-built images and points to external managed databases. Place it on your VPS at `/opt/agentpulse/docker-compose.prod.yml`.

```yaml
services:
  backend:
    image: ghcr.io/<org>/agentpulse-backend:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      APP_ENV: production
      HTTP_HOST: "0.0.0.0"
      HTTP_PORT: "8080"
      POSTGRES_DSN: "${POSTGRES_DSN}"
      CLICKHOUSE_ADDR: "${CLICKHOUSE_ADDR}"
      CLICKHOUSE_DATABASE: "${CLICKHOUSE_DATABASE:-agentpulse}"
      CLICKHOUSE_USER: "${CLICKHOUSE_USER}"
      CLICKHOUSE_PASSWORD: "${CLICKHOUSE_PASSWORD}"
      S3_ENDPOINT: "${S3_ENDPOINT}"
      S3_BUCKET: "${S3_BUCKET:-agentpulse-spans}"
      S3_ACCESS_KEY: "${S3_ACCESS_KEY}"
      S3_SECRET_KEY: "${S3_SECRET_KEY}"
      S3_ENFORCE_HTTPS: "true"
      CORS_ALLOWED_ORIGINS: "${CORS_ALLOWED_ORIGINS}"
      ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/healthz"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s

  collector:
    image: ghcr.io/<org>/agentpulse-collector:latest
    restart: unless-stopped
    ports:
      - "4317:4317"
      - "4318:4318"
    environment:
      CLICKHOUSE_ENDPOINT: "clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@${CLICKHOUSE_ADDR}/${CLICKHOUSE_DATABASE:-agentpulse}"
      CLICKHOUSE_DATABASE: "${CLICKHOUSE_DATABASE:-agentpulse}"
      POSTGRES_DSN: "${POSTGRES_DSN}"
```

Populate secrets in a `.env` file in the same directory (not committed to git):

```bash
POSTGRES_DSN=postgres://user:pass@host/db?sslmode=require
CLICKHOUSE_ADDR=host:9000
CLICKHOUSE_USER=agentpulse
CLICKHOUSE_PASSWORD=...
S3_ENDPOINT=https://...
S3_ACCESS_KEY=...
S3_SECRET_KEY=...
CORS_ALLOWED_ORIGINS=https://your-domain.com
```

Start with: `docker compose -f docker-compose.prod.yml --env-file .env up -d`

### systemd service

Create `/etc/systemd/system/agentpulse.service`:

```ini
[Unit]
Description=AgentPulse Backend
Requires=docker.service
After=docker.service network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/agentpulse
ExecStart=/usr/bin/docker compose -f docker-compose.prod.yml --env-file .env up
ExecStop=/usr/bin/docker compose -f docker-compose.prod.yml down
Restart=on-failure
RestartSec=10s
TimeoutStartSec=120

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable agentpulse
sudo systemctl start agentpulse
sudo systemctl status agentpulse
```

### nginx reverse proxy

Install nginx and create `/etc/nginx/sites-available/agentpulse`:

```nginx
server {
    listen 80;
    server_name your-domain.com;
    # Redirect HTTP to HTTPS — remove this block if handling TLS elsewhere
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    # Backend API + WebSocket
    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;

        # WebSocket upgrade headers
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_read_timeout  3600s;
        proxy_send_timeout  3600s;
    }
}
```

Enable and reload:

```bash
sudo ln -s /etc/nginx/sites-available/agentpulse /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
# Obtain TLS certificate (if using Let's Encrypt)
sudo certbot --nginx -d your-domain.com
```

---

## Environment Variables Reference

All variables are read by the backend at startup. Variables marked **required** cause the server to exit if unset.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_ENV` | | `development` | Set to `production` to tighten CORS and disable dev conveniences |
| `HTTP_HOST` | | `0.0.0.0` | Listen address |
| `HTTP_PORT` | | `8080` | Listen port |
| `HTTP_TLS_CERT` | | — | Path to TLS certificate file; leave blank to use plaintext (put a reverse proxy in front) |
| `HTTP_TLS_KEY` | | — | Path to TLS private key file |
| `CORS_ALLOWED_ORIGINS` | production | — | Comma-separated list of allowed origins, e.g. `https://app.example.com` |
| `POSTGRES_DSN` | yes | — | Full Postgres connection string, e.g. `postgres://user:pass@host/db?sslmode=require` |
| `CLICKHOUSE_ADDR` | yes | `localhost:9000` | ClickHouse native TCP address |
| `CLICKHOUSE_DATABASE` | | `agentpulse` | ClickHouse database name |
| `CLICKHOUSE_USER` | | `agentpulse` | ClickHouse username |
| `CLICKHOUSE_PASSWORD` | yes | — | ClickHouse password |
| `S3_ENDPOINT` | | — | S3-compatible endpoint URL; leave empty for AWS S3 |
| `S3_BUCKET` | | `agentpulse-spans` | Bucket name for span payload offload |
| `S3_ACCESS_KEY` | yes | — | S3 access key ID |
| `S3_SECRET_KEY` | yes | — | S3 secret access key |
| `S3_ENFORCE_HTTPS` | | `false` | Set `true` in production |
| `ANTHROPIC_API_KEY` | | — | Required to run Anthropic-backed eval judges |
| `OPENAI_API_KEY` | | — | Required to run OpenAI-backed eval judges |
| `GOOGLE_AI_API_KEY` | | — | Required to run Google AI eval judges |
| `VAPID_PUBLIC_KEY` | | — | VAPID public key for Web Push notifications |
| `VAPID_PRIVATE_KEY` | | — | VAPID private key |
| `VAPID_SUBJECT` | | — | `mailto:` or `https:` URI per RFC 8292 |
| `RESEND_API_KEY` | | — | Resend API key for transactional email (alert digests) |
| `EMAIL_FROM_ADDRESS` | | — | Sender address for alert digest emails |

---

## Post-Deploy Checklist

Run through this after every new deployment.

- [ ] **Health check passes**
  ```bash
  curl -f https://your-domain.com/healthz
  # Expected: HTTP 200 with a JSON body
  ```

- [ ] **Postgres migrations are up to date**

  Migrations live in `migrations/postgres/`. The server applies them on startup if configured to do so. Verify the schema version matches the current codebase:
  ```bash
  # Connect to Postgres and check the schema_migrations table
  psql "$POSTGRES_DSN" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 5;"
  ```

- [ ] **ClickHouse migrations are up to date**

  Migrations live in `migrations/clickhouse/`. Verify the most recent migration has been applied:
  ```bash
  clickhouse-client --host <host> --user <user> --password <pass> \
    --query "SELECT name FROM system.tables WHERE database = 'agentpulse' ORDER BY name"
  ```

- [ ] **Create the first project and obtain an API key**
  ```bash
  curl -s -X POST https://your-domain.com/api/v1/projects \
    -H "Content-Type: application/json" \
    -d '{"name": "my-first-project"}' | jq .
  # Copy the api_key from the response — use it as AGENTPULSE_API_KEY in your SDK config
  ```

- [ ] **Verify the OTel collector is reachable**
  ```bash
  curl -f http://collector-host:13133/  # collector health check endpoint
  ```

- [ ] **Send a test span**

  Use the Python SDK or `tracegen` tool at `tracegen/` to emit a test trace and confirm it appears in the UI.

- [ ] **Confirm CORS headers** (if frontend is on a different origin)
  ```bash
  curl -I -H "Origin: https://your-frontend.com" https://your-domain.com/api/v1/projects
  # Access-Control-Allow-Origin should match your frontend origin
  ```
