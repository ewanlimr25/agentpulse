package clickhouseexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemPayloadStore_PutAndGet(t *testing.T) {
	store := newMemPayloadStore()
	data := []byte(`{"gen_ai.prompt":"hello world"}`)
	key := "proj-1/2024-01-01/run-1/span-abc.json"

	err := store.Put(context.Background(), key, data)
	require.NoError(t, err)

	got := store.Get(key)
	assert.Equal(t, data, got)
}

func TestMemPayloadStore_GetMissing(t *testing.T) {
	store := newMemPayloadStore()
	assert.Nil(t, store.Get("nonexistent"))
}

func TestMemPayloadStore_PutIsolatesData(t *testing.T) {
	store := newMemPayloadStore()
	data := []byte("original")
	err := store.Put(context.Background(), "k", data)
	require.NoError(t, err)

	// Mutating original slice should not affect stored data.
	data[0] = 'X'
	assert.Equal(t, []byte("original"), store.Get("k"))
}

func TestNoopPayloadStore_PutReturnsError(t *testing.T) {
	store := noopPayloadStore{}
	err := store.Put(context.Background(), "any/key.json", []byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3 disabled")
}

func TestKeyComponentValidation_InvalidProjectID(t *testing.T) {
	invalidIDs := []string{
		"../etc/passwd",
		"proj/bad",
		"",
		"has spaces",
		"toolooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong",
	}
	for _, id := range invalidIDs {
		assert.False(t, validKeyComponent.MatchString(id), "expected %q to be invalid", id)
	}
}

func TestKeyComponentValidation_ValidProjectID(t *testing.T) {
	validIDs := []string{
		"proj-123",
		"my_project",
		"a",
		"PROJ-abc-123_xyz",
	}
	for _, id := range validIDs {
		assert.True(t, validKeyComponent.MatchString(id), "expected %q to be valid", id)
	}
}

func TestS3Config_StringRedactsCredentials(t *testing.T) {
	cfg := S3Config{
		Endpoint:       "https://s3.amazonaws.com",
		Bucket:         "my-bucket",
		Region:         "us-east-1",
		AccessKey:      "AKIAIOSFODNN7EXAMPLE",
		SecretKey:      "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		ThresholdBytes: 8192,
		Enabled:        true,
	}

	s := cfg.String()
	assert.Contains(t, s, "https://s3.amazonaws.com")
	assert.Contains(t, s, "my-bucket")
	assert.Contains(t, s, "us-east-1")
	assert.Contains(t, s, "<redacted>")
	assert.NotContains(t, s, "AKIAIOSFODNN7EXAMPLE")
	assert.NotContains(t, s, "wJalrXUtnFEMI")
}

func TestConfigValidate_HTTPEndpointWithEnforceHTTPS(t *testing.T) {
	cfg := defaultConfig()
	cfg.S3.Enabled = true
	cfg.S3.EnforceHTTPS = true
	cfg.S3.Endpoint = "http://minio:9000"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPS")
}

func TestConfigValidate_HTTPEndpointWithoutEnforceHTTPS(t *testing.T) {
	cfg := defaultConfig()
	cfg.S3.Enabled = true
	cfg.S3.EnforceHTTPS = false
	cfg.S3.Endpoint = "http://minio:9000"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfigValidate_HTTPSEndpointWithEnforceHTTPS(t *testing.T) {
	cfg := defaultConfig()
	cfg.S3.Enabled = true
	cfg.S3.EnforceHTTPS = true
	cfg.S3.Endpoint = "https://s3.amazonaws.com"

	err := cfg.Validate()
	assert.NoError(t, err)
}
