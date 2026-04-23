package s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

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

// Delete removes the object at the given S3 key.
func (s *S3PayloadStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete object %q: %w", key, err)
	}
	return nil
}

const statsByPrefixTimeout = 30 * time.Second

// StatsByPrefix lists all objects under prefix and returns (objectCount, totalBytes, error).
// Uses a 30-second context timeout to bound long-running scans.
func (s *S3PayloadStore) StatsByPrefix(ctx context.Context, prefix string) (int64, int64, error) {
	scanCtx, cancel := context.WithTimeout(ctx, statsByPrefixTimeout)
	defer cancel()

	var objectCount, totalBytes int64
	var continuationToken *string

	for {
		resp, err := s.client.ListObjectsV2(scanCtx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return 0, 0, fmt.Errorf("s3 list objects prefix %q: %w", prefix, err)
		}

		for _, obj := range resp.Contents {
			objectCount++
			if obj.Size != nil {
				totalBytes += *obj.Size
			}
		}

		if resp.IsTruncated == nil || !*resp.IsTruncated || resp.NextContinuationToken == nil {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	return objectCount, totalBytes, nil
}

const deleteBatchSize = 1000

// DeleteByKeys deletes a batch of S3 keys using the DeleteObjects API.
// Keys are processed in batches of up to 1000 (the S3 API maximum).
func (s *S3PayloadStore) DeleteByKeys(ctx context.Context, keys []string) (int64, error) {
	var deleted int64
	for i := 0; i < len(keys); i += deleteBatchSize {
		end := i + deleteBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]

		objects := make([]types.ObjectIdentifier, len(batch))
		for j, k := range batch {
			k := k // capture
			objects[j] = types.ObjectIdentifier{Key: aws.String(k)}
		}

		resp, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return deleted, fmt.Errorf("s3 delete objects batch: %w", err)
		}
		if len(resp.Errors) > 0 {
			return deleted, fmt.Errorf("s3 delete objects: %d errors, first: %s %s", len(resp.Errors), aws.ToString(resp.Errors[0].Key), aws.ToString(resp.Errors[0].Message))
		}
		deleted += int64(len(batch))
	}
	return deleted, nil
}
