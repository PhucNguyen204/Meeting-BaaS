//go:build integration
// +build integration

// Run with: go test -tags=integration ./internal/infra/storage/s3/...
//
// Requires Docker. Spins up a MinIO container, creates a bucket, and verifies
// UploadFile actually puts the object.
package s3_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	"go.uber.org/zap/zaptest"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/s3"
)

// TestS3UploadAgainstMinIO verifies UploadFile writes the local file to the
// configured bucket and the object is then retrievable.
func TestS3UploadAgainstMinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const accessKey = "minioadmin"
	const secretKey = "minioadmin"
	const bucket = "recordings-it"

	minioC, err := tcminio.Run(ctx,
		"minio/minio:RELEASE.2024-08-29T01-40-52Z",
		tcminio.WithUsername(accessKey),
		tcminio.WithPassword(secretKey),
	)
	if err != nil {
		t.Fatalf("minio testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(minioC) })

	endpoint, err := minioC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if !startsWithScheme(endpoint) {
		endpoint = "http://" + endpoint
	}
	if _, err := url.Parse(endpoint); err != nil {
		t.Fatalf("invalid endpoint URL %q: %v", endpoint, err)
	}

	if err := ensureBucket(ctx, t, endpoint, accessKey, secretKey, bucket); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}

	cli, err := s3.NewClient(ctx, zaptest.NewLogger(t), s3.Options{
		Endpoint:     endpoint,
		Region:       "us-east-1",
		Bucket:       bucket,
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(tmp, []byte("hello-from-it"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	const key = "tests/hello.txt"
	if err := cli.UploadFile(ctx, tmp, key); err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := assertObjectExists(ctx, endpoint, accessKey, secretKey, bucket, key); err != nil {
		t.Fatalf("expected object %s/%s to exist: %v", bucket, key, err)
	}
}

// ensureBucket creates the bucket using a raw S3 client (the Client we test
// only knows how to PutObject). Idempotent: existing bucket is OK.
func ensureBucket(ctx context.Context, t *testing.T, endpoint, ak, sk, bucket string) error {
	t.Helper()
	c, err := newRawS3(ctx, endpoint, ak, sk)
	if err != nil {
		return err
	}
	_, err = c.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "BucketAlreadyOwnedByYou", "BucketAlreadyExists":
			return nil
		}
	}
	return err
}

func assertObjectExists(ctx context.Context, endpoint, ak, sk, bucket, key string) error {
	c, err := newRawS3(ctx, endpoint, ak, sk)
	if err != nil {
		return err
	}
	_, err = c.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func newRawS3(ctx context.Context, endpoint, ak, sk string) (*awss3.Client, error) {
	resolver := aws.EndpointResolverWithOptionsFunc(
		func(string, string, ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		},
	)
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithEndpointResolverWithOptions(resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(ak, sk, "")),
	)
	if err != nil {
		return nil, err
	}
	return awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.UsePathStyle = true
	}), nil
}

func startsWithScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ':':
			return i+2 < len(s) && s[i+1] == '/' && s[i+2] == '/'
		case '/':
			return false
		}
	}
	return false
}
