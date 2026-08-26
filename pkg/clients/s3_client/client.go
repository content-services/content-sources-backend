package s3_client

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	cfg "github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

type S3Client interface {
	Put(ctx context.Context, key string, body io.Reader) error
}

type s3Client struct {
	client *s3.Client
	bucket string
}

func NewS3Client(store cfg.ObjectStore) (S3Client, error) {
	if store.Name == "" {
		return nil, fmt.Errorf("s3 not configured")
	}

	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(store.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(store.AccessKey, store.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		if store.URL != "" {
			o.BaseEndpoint = aws.String(store.URL)
		}
	})
	return &s3Client{client: client, bucket: store.Name}, nil
}

func (c *s3Client) Put(ctx context.Context, storageKey string, body io.Reader) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &c.bucket,
		Key:    &storageKey,
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to s3: %w", storageKey, err)
	}
	log.Info().Msgf("Uploaded %s to s3", storageKey)
	return nil
}
