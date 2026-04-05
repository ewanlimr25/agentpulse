package clickhouseexporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// PayloadStore abstracts the remote storage for offloaded span payloads.
type PayloadStore interface {
	Put(ctx context.Context, key string, data []byte) error
}

// s3PayloadStore uploads payload blobs to an S3-compatible bucket.
type s3PayloadStore struct {
	client *s3.Client
	cfg    S3Config
}

// newS3PayloadStore creates a new s3PayloadStore from config.
func newS3PayloadStore(cfg S3Config) (*s3PayloadStore, error) {
	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
		if cfg.Endpoint != "" {
			return aws.Endpoint{
				URL:           cfg.Endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	awsCfg := aws.Config{
		Region:                      cfg.Region,
		Credentials:                 creds,
		EndpointResolverWithOptions: resolver,
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &s3PayloadStore{
		client: client,
		cfg:    cfg,
	}, nil
}

// Put uploads data to S3 at the given key with a context timeout.
func (s *s3PayloadStore) Put(ctx context.Context, key string, data []byte) error {
	uploadCtx, cancel := context.WithTimeout(ctx, s.cfg.UploadTimeout)
	defer cancel()

	_, err := s.client.PutObject(uploadCtx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
		ACL:         s3types.ObjectCannedACLPrivate,
	})
	if err != nil {
		return fmt.Errorf("s3 put %q: %w", key, err)
	}
	return nil
}

// noopPayloadStore is used when S3 is disabled. Any accidental call returns an error.
type noopPayloadStore struct{}

func (noopPayloadStore) Put(_ context.Context, _ string, _ []byte) error {
	return errors.New("s3 disabled")
}

// memPayloadStore is an in-memory store for use in tests.
type memPayloadStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemPayloadStore() *memPayloadStore {
	return &memPayloadStore{data: make(map[string][]byte)}
}

// Put stores data at the given key.
func (m *memPayloadStore) Put(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.data[key] = cp
	return nil
}

// Get retrieves data stored at key. Returns nil if not found.
func (m *memPayloadStore) Get(key string) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[key]
}

// Keys returns all stored keys.
func (m *memPayloadStore) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.data))
	for k := range m.data {
		out = append(out, k)
	}
	return out
}
