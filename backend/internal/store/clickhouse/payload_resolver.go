package clickhouse

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// ResolvePayloads fetches and merges offloaded payload fields into span.Attributes.
// If payloads is nil or span.PayloadS3Key is empty, returns immediately (no-op).
// If S3 fetch fails, logs a warning and leaves Attributes unchanged (fail-open).
// Security: asserts key starts with projectID+"/" before fetching.
func ResolvePayloads(ctx context.Context, span *domain.Span, payloads store.PayloadStore) {
	if payloads == nil || span.PayloadS3Key == "" {
		return
	}

	// SECURITY: key must start with span.ProjectID + "/"
	if !strings.HasPrefix(span.PayloadS3Key, span.ProjectID+"/") {
		slog.Warn("payload_resolver: S3 key does not match span project, skipping",
			"key", span.PayloadS3Key, "project_id", span.ProjectID)
		return
	}

	data, err := payloads.Get(ctx, span.PayloadS3Key)
	if err != nil {
		slog.Warn("payload_resolver: S3 fetch failed, returning inline data",
			"key", span.PayloadS3Key, "error", err)
		return
	}

	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		slog.Warn("payload_resolver: failed to unmarshal S3 payload", "error", err)
		return
	}

	if span.Attributes == nil {
		span.Attributes = make(map[string]string)
	}
	for k, v := range fields {
		span.Attributes[k] = v
	}
}
