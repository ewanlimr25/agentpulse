package store

import "context"

// PayloadStore fetches offloaded span payload data from object storage.
// The backend only reads; writes happen in the collector.
type PayloadStore interface {
	// Get fetches the JSON payload for the given S3 key.
	// Returns the raw JSON bytes: {"gen_ai.prompt": "...", "tool.input": "..."}
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes the object at the given S3 key.
	Delete(ctx context.Context, key string) error
	// StatsByPrefix lists all objects under prefix and returns (objectCount, totalBytes, error).
	// Uses a 30-second timeout internally to bound long-running scans.
	StatsByPrefix(ctx context.Context, prefix string) (objectCount int64, totalBytes int64, err error)
}
