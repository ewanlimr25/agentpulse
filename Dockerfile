# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Install git and ca-certificates (needed for go get over HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Copy go workspace files so the module graph resolves correctly
COPY go.work go.work.sum ./

# Copy only the backend module first so dependency downloads are cached
COPY backend/go.mod backend/go.sum ./backend/

# Download dependencies (cached layer — only re-runs when go.mod/go.sum change)
WORKDIR /src/backend
RUN go mod download

# Copy the rest of the source
WORKDIR /src
COPY backend/ ./backend/

# Build a fully static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o /out/agentpulse-server \
    github.com/agentpulse/agentpulse/backend/cmd/server

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.19

# Certificates + timezone data used at runtime (TLS outbound calls, LLM APIs)
RUN apk add --no-cache ca-certificates tzdata

# Non-root user for least-privilege execution
RUN addgroup -S agentpulse && adduser -S -G agentpulse agentpulse

WORKDIR /app

COPY --from=builder /out/agentpulse-server .

# ── Environment variable defaults (override at runtime) ──────────────────────
# Server
ENV HTTP_HOST=0.0.0.0
ENV HTTP_PORT=8080
ENV APP_ENV=production

# Postgres
ENV POSTGRES_DSN=""

# ClickHouse
ENV CLICKHOUSE_ADDR=""
ENV CLICKHOUSE_DATABASE=agentpulse
ENV CLICKHOUSE_USER=agentpulse
ENV CLICKHOUSE_PASSWORD=""

# Object storage (S3 / MinIO)
ENV S3_ENDPOINT=""
ENV S3_BUCKET=agentpulse-spans
ENV S3_ACCESS_KEY=""
ENV S3_SECRET_KEY=""
ENV S3_ENFORCE_HTTPS=true

# CORS
ENV CORS_ALLOWED_ORIGINS=""

# LLM providers (evals)
ENV ANTHROPIC_API_KEY=""
ENV OPENAI_API_KEY=""
ENV GOOGLE_AI_API_KEY=""

# Notifications
ENV VAPID_PUBLIC_KEY=""
ENV VAPID_PRIVATE_KEY=""
ENV VAPID_SUBJECT=""
ENV RESEND_API_KEY=""
ENV EMAIL_FROM_ADDRESS=""

USER agentpulse

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/agentpulse-server"]
