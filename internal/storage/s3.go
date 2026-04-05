package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"prototypehub/internal/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, cfg config.Config) (*S3Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = &cfg.S3Endpoint
		}
		o.UsePathStyle = cfg.S3UsePathStyle
	})

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &cfg.S3Bucket}); err != nil {
		if _, createErr := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &cfg.S3Bucket}); createErr != nil {
			return nil, fmt.Errorf("prepare bucket: %w", err)
		}
	}

	return &S3Storage{client: client, bucket: cfg.S3Bucket}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read upload body: %w", err)
	}

	contentLength := int64(len(data))
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		Body:          bytes.NewReader(data),
		ContentType:   &contentType,
		ContentLength: &contentLength,
	})
	return err
}

func (s *S3Storage) Get(ctx context.Context, key string) (*Object, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	contentType := "application/octet-stream"
	if output.ContentType != nil && *output.ContentType != "" {
		contentType = *output.ContentType
	}
	return &Object{Body: output.Body, ContentType: contentType}, nil
}

func (s *S3Storage) DeletePrefix(ctx context.Context, prefix string) error {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return nil
	}

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects for prefix %s: %w", prefix, err)
		}
		if len(page.Contents) == 0 {
			continue
		}

		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, object := range page.Contents {
			if object.Key == nil || *object.Key == "" {
				continue
			}
			objects = append(objects, types.ObjectIdentifier{Key: object.Key})
		}
		if len(objects) == 0 {
			continue
		}

		_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &s.bucket,
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("delete objects for prefix %s: %w", prefix, err)
		}
	}

	return nil
}
