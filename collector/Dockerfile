# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the collector binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /agentpulse-collector ./cmd/collector/...

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Non-root user
RUN addgroup -S agentpulse && adduser -S agentpulse -G agentpulse

WORKDIR /app

COPY --from=builder /agentpulse-collector ./agentpulse-collector

# Default config and attribute registry are mounted at runtime.
# The config.yaml is provided via --config flag or CONFIG_FILE env var.
RUN mkdir -p /etc/agentpulse
COPY config.yaml /etc/agentpulse/config.yaml

USER agentpulse

EXPOSE 4317 4318 13133 8888

ENTRYPOINT ["./agentpulse-collector"]
CMD ["--config", "/etc/agentpulse/config.yaml"]
