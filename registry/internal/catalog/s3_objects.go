package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config configures an independent S3-compatible object provider.
type S3Config struct {
	Endpoint, AccessKey, SecretKey, Bucket, Region string
	UseTLS                                         bool
}

// S3ObjectStore stores immutable objects in an S3-compatible bucket.
type S3ObjectStore struct {
	client *minio.Client
	bucket string
}

// NewS3ObjectStore configures an independent S3-compatible object provider.
func NewS3ObjectStore(config S3Config) (*S3ObjectStore, error) {
	if config.Endpoint == "" || config.Bucket == "" {
		return nil, errors.New("S3 endpoint and bucket are required")
	}
	client, err := minio.New(config.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""), Secure: config.UseTLS, Region: config.Region})
	if err != nil {
		return nil, fmt.Errorf("configure object store: %w", err)
	}
	return &S3ObjectStore{client: client, bucket: config.Bucket}, nil
}

func (s *S3ObjectStore) ReadObject(path string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	object, err := s.client.GetObject(ctx, s.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	defer object.Close()
	info, err := object.Stat()
	if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(io.LimitReader(object, 536870913))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 536870912 {
		return nil, "", fmt.Errorf("object exceeds maximum size")
	}
	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

func (s *S3ObjectStore) PutObjects(manifest PackageVersion, archive, readme, icon []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check object bucket: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create object bucket: %w", err)
		}
	}
	objects := []struct {
		path, contentType string
		data              []byte
	}{{manifest.Archive.Path, "application/gzip", archive}, {manifest.Readme.Path, "text/markdown; charset=utf-8", readme}, {manifest.Icon.Path, "image/svg+xml", icon}}
	for _, object := range objects {
		if _, err := s.client.StatObject(ctx, s.bucket, object.path, minio.StatObjectOptions{}); err == nil {
			return ErrConflict
		} else if minio.ToErrorResponse(err).StatusCode != http.StatusNotFound {
			return fmt.Errorf("check object %s: %w", object.path, err)
		}
	}
	written := make([]string, 0, len(objects))
	for _, object := range objects {
		if _, err := s.client.PutObject(ctx, s.bucket, object.path, bytes.NewReader(object.data), int64(len(object.data)), minio.PutObjectOptions{ContentType: object.contentType}); err != nil {
			for _, path := range written {
				_ = s.client.RemoveObject(ctx, s.bucket, path, minio.RemoveObjectOptions{})
			}
			return fmt.Errorf("upload object %s: %w", object.path, err)
		}
		written = append(written, object.path)
	}
	return nil
}

func (s *S3ObjectStore) DeleteObjects(manifest PackageVersion) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, path := range []string{manifest.Archive.Path, manifest.Readme.Path, manifest.Icon.Path} {
		if err := s.client.RemoveObject(ctx, s.bucket, path, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("remove object %s: %w", path, err)
		}
	}
	return nil
}
