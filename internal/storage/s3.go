package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/config"
)

// S3Store stores audio files in an S3-compatible object store.
type S3Store struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	prefix        string
	presignExpiry config.S3Config
	log           zerolog.Logger
}

// NewS3Store creates an S3 audio store from config.
func NewS3Store(cfg config.S3Config, log zerolog.Logger) (*S3Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	presignClient := s3.NewPresignClient(client)

	return &S3Store{
		client:        client,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
		prefix:        cfg.Prefix,
		presignExpiry: cfg,
		log:           log.With().Str("component", "s3-store").Logger(),
	}, nil
}

// HeadBucket checks that the bucket exists and credentials are valid.
func (s *S3Store) HeadBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &s.bucket,
	})
	return err
}

func (s *S3Store) Save(ctx context.Context, key string, data []byte, contentType string) error {
	objKey := s.objectKey(key)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &objKey,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	return err
}

func (s *S3Store) LocalPath(key string) string {
	return ""
}

func (s *S3Store) URL(ctx context.Context, key string) (string, error) {
	objKey := s.objectKey(key)
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	}, func(opts *s3.PresignOptions) {
		opts.Expires = s.presignExpiry.PresignExpiry
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	objKey := s.objectKey(key)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Store) Exists(ctx context.Context, key string) bool {
	objKey := s.objectKey(key)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	})
	return err == nil
}

func (s *S3Store) Type() string { return "s3" }

func (s *S3Store) objectKey(key string) string {
	if s.prefix != "" {
		return s.prefix + "/audio/" + key
	}
	return "audio/" + key
}
