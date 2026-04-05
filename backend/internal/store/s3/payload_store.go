package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/agentpulse/agentpulse/backend/internal/config"
)

// S3PayloadStore implements store.PayloadStore using AWS S3 (or MinIO-compatible storage).
type S3PayloadStore struct {
	client *s3.Client
	bucket string
}

// New creates a new S3PayloadStore from the given S3 config.
// Returns an error if EnforceHTTPS is true and the endpoint uses http://.
func New(cfg config.S3Config) (*S3PayloadStore, error) {
	if cfg.EnforceHTTPS && strings.HasPrefix(cfg.Endpoint, "http://") {
		return nil, fmt.Errorf("S3 endpoint must use HTTPS (set S3_ENFORCE_HTTPS=false to override in development)")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
		awsconfig.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("s3 load config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = true
	})

	return &S3PayloadStore{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Get fetches the JSON payload for the given S3 key.
// Returns the raw JSON bytes for the offloaded payload fields.
func (s *S3PayloadStore) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object %q: %w", key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read body %q: %w", key, err)
	}
	return data, nil
}
