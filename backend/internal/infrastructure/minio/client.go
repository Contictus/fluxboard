// Package miniox is the MinIO/S3 object-storage adapter: presigned PUT/GET for
// attachments plus stat/remove for the confirm flow and orphan GC. It implements
// project.ObjectStore so the usecase layer never touches the SDK or proxies bytes
// (FR-TASK-006, docs/03-ARCHITECTURE.md).
package miniox

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
)

// Config is the MinIO connection configuration (from config.Config).
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Client is a MinIO-backed project.ObjectStore.
type Client struct {
	mc     *minio.Client
	bucket string
}

var _ project.ObjectStore = (*Client)(nil)

// New dials MinIO and ensures the bucket exists (idempotent). A nil error means
// object storage is ready.
func New(ctx context.Context, cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: dial: %w", err)
	}
	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio: bucket check: %w", err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio: make bucket: %w", err)
		}
	}
	return &Client{mc: mc, bucket: cfg.Bucket}, nil
}

// PresignPut returns a time-limited upload URL for key.
func (c *Client) PresignPut(ctx context.Context, key, _ string, ttl time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, c.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("minio: presign put: %w", err)
	}
	return u.String(), nil
}

// PresignGet returns a time-limited download URL for key that forces a download
// with the given filename (Content-Disposition).
func (c *Client) PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error) {
	reqParams := make(url.Values)
	reqParams.Set("response-content-disposition", fmt.Sprintf("attachment; filename=%q", filename))
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, key, ttl, reqParams)
	if err != nil {
		return "", fmt.Errorf("minio: presign get: %w", err)
	}
	return u.String(), nil
}

// Stat returns the object's size, or domain.ErrNotFound if it is absent.
func (c *Client) Stat(ctx context.Context, key string) (int64, error) {
	info, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return 0, domain.ErrNotFound
		}
		return 0, fmt.Errorf("minio: stat: %w", err)
	}
	return info.Size, nil
}

// Remove deletes the object; a missing object is not an error (idempotent).
func (c *Client) Remove(ctx context.Context, key string) error {
	err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && minio.ToErrorResponse(err).Code != "NoSuchKey" {
		return fmt.Errorf("minio: remove: %w", err)
	}
	return nil
}
