package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
}

func MinioConfigFromEnv() MinioConfig {
	config := MinioConfig{
		Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		AccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		SecretKey: os.Getenv("MINIO_SECRET_KEY"),
		Bucket:    os.Getenv("MINIO_BUCKET"),
	}
	if config.Endpoint == "" {
		config.Endpoint = "localhost:9000"
	}
	if config.Bucket == "" {
		config.Bucket = "ebike-motor-media"
	}
	return config
}

type MinioStorage struct {
	client *minio.Client
	bucket string
}

func NewMinioStorage(config MinioConfig) (*MinioStorage, error) {
	if config.AccessKey == "" || config.SecretKey == "" {
		return nil, errors.New("не заданы MINIO_ACCESS_KEY и MINIO_SECRET_KEY")
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}
	return &MinioStorage{client: client, bucket: config.Bucket}, nil
}

func (s *MinioStorage) Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, content, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("загрузка %s в Minio: %w", key, err)
	}
	return nil
}

func (s *MinioStorage) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
