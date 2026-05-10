// Package s3 provides an S3-compatible client for uploading recordings
// and logs. Works with both AWS S3 and MinIO.
//
// Implements Repository pattern for artifact storage.
package s3

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

// Client wraps the AWS S3 SDK for uploading bot artifacts.
type Client struct {
	log    *zap.Logger
	client *s3.Client
	bucket string
}

// Options configures the S3 client.
type Options struct {
	Endpoint     string // e.g. http://localhost:9000 for MinIO
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool // required for MinIO
}

// NewClient creates an S3 client. For MinIO dev, set Endpoint + UsePathStyle.
func NewClient(ctx context.Context, log *zap.Logger, opts Options) (*Client, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Region == "" {
		opts.Region = "us-east-1"
	}
	if opts.Bucket == "" {
		opts.Bucket = "recordings"
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if opts.Endpoint != "" {
				return aws.Endpoint{
					URL:               opts.Endpoint,
					HostnameImmutable: true,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		},
	)

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(opts.Region),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			opts.AccessKey, opts.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: load config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = opts.UsePathStyle
	})

	return &Client{
		log:    log.Named("s3"),
		client: s3Client,
		bucket: opts.Bucket,
	}, nil
}

// Upload implements the states.Uploader interface. Uploads the recording
// MP4 to the configured S3 bucket.
func (c *Client) Upload(ctx context.Context, mc *sm.MeetingContext) error {
	if mc.Config.MP4S3Path == "" {
		c.log.Debug("no mp4_s3_path configured, skipping upload")
		return nil
	}

	return c.UploadFile(ctx, mc.Config.MP4S3Path, mc.Config.MP4S3Path)
}

// UploadFile uploads a local file to S3 with the given key.
func (c *Client) UploadFile(ctx context.Context, localPath, s3Key string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("s3: open %s: %w", localPath, err)
	}
	defer f.Close()

	_, err = c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("s3: put %s: %w", s3Key, err)
	}

	c.log.Info("uploaded to s3",
		zap.String("bucket", c.bucket),
		zap.String("key", s3Key),
	)
	return nil
}
