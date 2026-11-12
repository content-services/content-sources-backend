package sonatype_bridge

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // ETag comparison only, not used for security
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"
)

// S3Uploader abstracts calls to the partner-owned S3 bucket.
//
// The partner bucket disallows overwriting objects, so uploads are
// conditional: an existing object with identical content is treated as
// already delivered, and one with different content yields an
// *ObjectConflictError.
type S3Uploader interface {
	// UploadFile uploads key under the configured file prefix.
	UploadFile(ctx context.Context, key string, data []byte, contentType string) error
	// UploadRootFile uploads key relative to the bucket root, ignoring the file prefix.
	UploadRootFile(ctx context.Context, key string, data []byte, contentType string) error
	VerifyDelivery(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// ObjectConflictError reports that an object already exists in the partner
// bucket with different content. It is not retryable since the bucket
// disallows overwrites.
type ObjectConflictError struct {
	Key string
}

func (e *ObjectConflictError) Error() string {
	return fmt.Sprintf("object %s already exists with different content", e.Key)
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
	return u.putObject(ctx, u.objectKey(key), data, contentType)
}

func (u *awsS3Uploader) UploadRootFile(ctx context.Context, key string, data []byte, contentType string) error {
	return u.putObject(ctx, strings.TrimPrefix(key, "/"), data, contentType)
}

// putObject writes objKey only if it does not already exist. When the
// object exists, its stored checksum is compared with data: identical
// content counts as delivered, anything else is an *ObjectConflictError.
func (u *awsS3Uploader) putObject(ctx context.Context, objKey string, data []byte, contentType string) error {
	sum := sha256.Sum256(data)
	checksum := base64.StdEncoding.EncodeToString(sum[:])

	var exists bool
	err := retryWithBackoff(ctx, u.maxRetries, func() error {
		_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:            aws.String(u.bucket),
			Key:               aws.String(objKey),
			Body:              bytes.NewReader(data),
			ContentLength:     aws.Int64(int64(len(data))),
			ContentType:       aws.String(contentType),
			IfNoneMatch:       aws.String("*"),
			ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
			ChecksumSHA256:    aws.String(checksum),
		})
		if isPreconditionFailed(err) {
			exists = true
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", objKey, err)
	}
	if !exists {
		return nil
	}

	same, err := u.sameContent(ctx, objKey, checksum, data)
	if err != nil {
		return fmt.Errorf("upload %s: %w", objKey, err)
	}
	if !same {
		return &ObjectConflictError{Key: objKey}
	}
	log.Info().Str("key", objKey).Msg("object already delivered with identical content, skipping")
	return nil
}

// sameContent reports whether the existing objKey matches data, using the
// stored SHA-256 checksum when present and a single-part MD5 ETag otherwise.
func (u *awsS3Uploader) sameContent(ctx context.Context, objKey, checksum string, data []byte) (bool, error) {
	var out *s3.HeadObjectOutput
	err := retryWithBackoff(ctx, u.maxRetries, func() error {
		var err error
		out, err = u.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket:       aws.String(u.bucket),
			Key:          aws.String(objKey),
			ChecksumMode: types.ChecksumModeEnabled,
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("inspect existing object: %w", err)
	}

	if stored := aws.ToString(out.ChecksumSHA256); stored != "" {
		return stored == checksum, nil
	}
	etag := strings.Trim(aws.ToString(out.ETag), `"`)
	if len(etag) == 32 && !strings.Contains(etag, "-") {
		md5sum := md5.Sum(data) //nolint:gosec // ETag comparison only
		return strings.EqualFold(etag, hex.EncodeToString(md5sum[:])), nil
	}
	return false, nil
}

// isPreconditionFailed reports whether a conditional write was rejected
// because the object already exists.
func isPreconditionFailed(err error) bool {
	var respErr *awshttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusPreconditionFailed
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
