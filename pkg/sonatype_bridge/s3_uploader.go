package sonatype_bridge

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Uploader abstracts calls to the partner-owned S3 bucket.
type S3Uploader interface {
	UploadFile(ctx context.Context, key string, data []byte, contentType string) error
	VerifyDelivery(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// s3API is the subset of the S3 client used by the uploader.
type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

type awsS3Uploader struct {
	client     s3API
	bucket     string
	prefix     string
	maxRetries int
}

// newS3Uploader builds a client for the partner bucket from the bridge
// configuration only. It deliberately does not load the default AWS config,
// so ambient settings (IAM role, AWS_* environment variables, shared config
// files) can never redirect uploads to a different account or endpoint.
func newS3Uploader(cfg bridgeConfig) (S3Uploader, error) {
	if err := cfg.validateS3(); err != nil {
		return nil, err
	}

	opts := s3.Options{
		Region:       cfg.S3Region,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
		HTTPClient:   &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second},
		UsePathStyle: cfg.S3UsePathStyle,
		// Retries are governed by sonatype_bridge.max_retries via retryWithBackoff.
		Retryer: aws.NopRetryer{},
	}
	if cfg.S3URL != "" {
		opts.BaseEndpoint = aws.String(cfg.S3URL)
	}
	client := s3.New(opts)

	return &awsS3Uploader{
		client:     client,
		bucket:     cfg.S3Bucket,
		prefix:     strings.Trim(cfg.S3FilePrefix, "/"),
		maxRetries: cfg.MaxRetries,
	}, nil
}

func (u *awsS3Uploader) objectKey(key string) string {
	key = strings.TrimPrefix(key, "/")
	if u.prefix == "" {
		return key
	}
	return u.prefix + "/" + key
}

func (u *awsS3Uploader) UploadFile(ctx context.Context, key string, data []byte, contentType string) error {
	objKey := u.objectKey(key)
	err := retryWithBackoff(ctx, u.maxRetries, func() error {
		_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(u.bucket),
			Key:           aws.String(objKey),
			Body:          bytes.NewReader(data),
			ContentLength: aws.Int64(int64(len(data))),
			ContentType:   aws.String(contentType),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", objKey, err)
	}
	return nil
}

func (u *awsS3Uploader) VerifyDelivery(ctx context.Context, key string) error {
	objKey := u.objectKey(key)
	_, err := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(objKey),
	})
	if err != nil {
		return fmt.Errorf("verify delivery %s: %w", objKey, err)
	}
	return nil
}

func (u *awsS3Uploader) Ping(ctx context.Context) error {
	_, err := u.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(u.bucket),
	})
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
