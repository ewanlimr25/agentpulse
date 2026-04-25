// Package localfs implements store.PayloadStore against the local filesystem.
// Used by indie mode in place of S3/R2/MinIO. Keys are SHA-256 hex names that
// map to files under a configurable root directory.
package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// PayloadStore stores offloaded span-payload JSON files under a root directory.
// Keys must be path-safe (no ".." components). Trade-off: no presigned URLs,
// no cross-host shared access — fine for single-binary deployments.
type PayloadStore struct {
	root string
}

// New returns a PayloadStore rooted at dir, creating it if necessary.
func New(dir string) (*PayloadStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("localfs: mkdir %s: %w", dir, err)
	}
	return &PayloadStore{root: dir}, nil
}

// Get returns the raw JSON bytes for a key.
func (s *PayloadStore) Get(ctx context.Context, key string) ([]byte, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("localfs: read %s: %w", key, err)
	}
	return data, nil
}

// Put writes raw bytes to a key. Used by the embedded OTLP receiver to offload
// oversized payload fields. The team-mode equivalent lives in the collector.
func (s *PayloadStore) Put(ctx context.Context, key string, body []byte) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("localfs: mkdir parent %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("localfs: write %s: %w", key, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("localfs: rename %s: %w", key, err)
	}
	return nil
}

// Delete removes a payload file. No-op if it does not exist.
func (s *PayloadStore) Delete(ctx context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("localfs: remove %s: %w", key, err)
	}
	return nil
}

// StatsByPrefix walks the root and returns (objectCount, totalBytes) for files
// whose relative path starts with prefix.
func (s *PayloadStore) StatsByPrefix(ctx context.Context, prefix string) (int64, int64, error) {
	var count, bytes int64
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.root, path)
		if prefix != "" && len(rel) < len(prefix) {
			return nil
		}
		if prefix != "" && rel[:len(prefix)] != prefix {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		count++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("localfs: walk: %w", err)
	}
	return count, bytes, nil
}

// resolve converts a logical key to an absolute filesystem path, rejecting
// paths that would escape the root directory.
func (s *PayloadStore) resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("localfs: empty key")
	}
	clean := filepath.Clean("/" + key) // force absolute, normalize ".." etc.
	full := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, full)
	if err != nil || rel == ".." || (len(rel) >= 3 && rel[:3] == "../") {
		return "", fmt.Errorf("localfs: invalid key %q", key)
	}
	return full, nil
}
