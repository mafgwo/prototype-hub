package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"prototypehub/internal/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3Storage struct {
	client *s3.Client
	bucket string
}

const deleteObjectsBatchSize = 1000

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
		if !cfg.S3AutoCreateBucket {
			return nil, fmt.Errorf("head bucket %s failed and S3_AUTO_CREATE_BUCKET=false: %w", cfg.S3Bucket, err)
		}
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

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	key = strings.Trim(key, "/")
	if key == "" {
		return nil
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		log.Printf("storage delete object failed: bucket=%s key=%s err=%v", s.bucket, key, err)
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) DeletePrefix(ctx context.Context, prefix string) error {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return nil
	}

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: aws.String(prefix),
		MaxKeys: aws.Int32(deleteObjectsBatchSize),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && apiErr.ErrorCode() == "AccessDenied" {
				log.Printf("storage list prefix access denied: bucket=%s prefix=%s err=%v", s.bucket, prefix, err)
				return fmt.Errorf("list objects for prefix %s: access denied; this storage requires ListBucket permission to remove extracted files by prefix", prefix)
			}
			log.Printf("storage list prefix failed: bucket=%s prefix=%s err=%v", s.bucket, prefix, err)
			return fmt.Errorf("list objects for prefix %s: %w", prefix, err)
		}
		if len(page.Contents) == 0 {
			log.Printf("storage delete prefix no objects: bucket=%s prefix=%s", s.bucket, prefix)
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

		for start := 0; start < len(objects); start += deleteObjectsBatchSize {
			end := start + deleteObjectsBatchSize
			if end > len(objects) {
				end = len(objects)
			}

			output, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: &s.bucket,
				Delete: &types.Delete{Objects: objects[start:end], Quiet: aws.Bool(true)},
			})
			if err != nil {
				if shouldFallbackToSingleDelete(err) {
					log.Printf("storage delete prefix batch falling back to single deletes: bucket=%s prefix=%s start=%d end=%d err=%v", s.bucket, prefix, start, end, err)
					if singleErr := s.deleteObjectsIndividually(ctx, objects[start:end]); singleErr != nil {
						return fmt.Errorf("delete objects for prefix %s with single delete fallback: %w", prefix, singleErr)
					}
					continue
				}
				log.Printf("storage delete prefix batch failed: bucket=%s prefix=%s start=%d end=%d err=%v", s.bucket, prefix, start, end, err)
				return fmt.Errorf("delete objects for prefix %s: %w", prefix, err)
			}
			if len(output.Errors) > 0 {
				first := output.Errors[0]
				key := ""
				code := ""
				message := ""
				if first.Key != nil {
					key = *first.Key
				}
				if first.Code != nil {
					code = *first.Code
				}
				if first.Message != nil {
					message = *first.Message
				}
				log.Printf("storage delete prefix batch partial failure: bucket=%s prefix=%s start=%d end=%d code=%s key=%s message=%s errors=%d", s.bucket, prefix, start, end, code, key, message, len(output.Errors))
				return fmt.Errorf("delete objects for prefix %s: %s for %s", prefix, code, key)
			}
		}
	}

	return nil
}

func (s *S3Storage) deleteObjectsIndividually(ctx context.Context, objects []types.ObjectIdentifier) error {
	for _, object := range objects {
		if object.Key == nil || *object.Key == "" {
			continue
		}
		if err := s.Delete(ctx, *object.Key); err != nil {
			return err
		}
	}
	return nil
}

func shouldFallbackToSingleDelete(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "InvalidRequest" && strings.Contains(strings.ToLower(apiErr.ErrorMessage()), "content-md5") {
			return true
		}
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "content-md5") && strings.Contains(message, "invalidrequest")
}
