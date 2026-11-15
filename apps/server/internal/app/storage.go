package app

import (
	"context"
	"time"
)

// PresignedRequest is a URL the client calls directly against object storage.
type PresignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string // headers the client must send with the request
	ExpiresAt time.Time
}

// ObjectStorage issues presigned URLs for uploads and downloads (S3 or
// MinIO). It is implemented by internal/platform/storage. Keys are chosen by
// the calling module and must start with the module name, e.g. "chat/...".
type ObjectStorage interface {
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (PresignedRequest, error)
	// PresignGet returns a download URL; fileName sets Content-Disposition.
	PresignGet(ctx context.Context, key, fileName string, ttl time.Duration) (PresignedRequest, error)
	// Stat returns the stored object's size, or an error wrapping ErrNotFound.
	Stat(ctx context.Context, key string) (size int64, err error)
}
