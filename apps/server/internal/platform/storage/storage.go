package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	defaultRegion = "us-east-1"
	// maxTTL is the longest expiry SigV4 allows for a presigned URL.
	maxTTL = 7 * 24 * time.Hour
	// maxFileNameRunes bounds the Content-Disposition header size.
	maxFileNameRunes = 255
)

// Config describes the S3-compatible endpoint.
type Config struct {
	// Endpoint is host:port or a full URL such as http://localhost:9000.
	// A URL scheme overrides UseSSL.
	Endpoint string
	// PublicEndpoint, when set, is the base the browser uses for presigned
	// URLs (the host is part of the signature), e.g. the reverse proxy's
	// public URL. Endpoint is still used for the server's own calls.
	PublicEndpoint string
	Bucket         string
	AccessKey      string
	SecretKey      string
	// Region defaults to us-east-1. Setting it avoids a network lookup
	// when signing.
	Region string
	UseSSL bool
	// PathStyle puts the bucket in the URL path (http://host/bucket/key),
	// which MinIO needs. Otherwise the client picks the style from the host.
	PathStyle bool
}

// Storage implements app.ObjectStorage on top of minio-go.
type Storage struct {
	client *minio.Client
	// signer presigns URLs; it is client unless PublicEndpoint is set.
	signer *minio.Client
	bucket string
}

var _ app.ObjectStorage = (*Storage)(nil)

// New builds a Storage. It does not contact the server: presigning is local.
func New(_ context.Context, cfg Config) (*Storage, error) {
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("storage: bucket, access key and secret key are required")
	}
	host, secure, err := parseEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}
	region := cfg.Region
	if region == "" {
		region = defaultRegion
	}
	lookup := minio.BucketLookupAuto
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	newClient := func(host string, secure bool) (*minio.Client, error) {
		c, err := minio.New(host, &minio.Options{
			Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure:       secure,
			Region:       region,
			BucketLookup: lookup,
		})
		if err != nil {
			return nil, fmt.Errorf("storage: new client: %w", err)
		}
		return c, nil
	}
	client, err := newClient(host, secure)
	if err != nil {
		return nil, err
	}
	signer := client
	if cfg.PublicEndpoint != "" {
		pubHost, pubSecure, err := parseEndpoint(cfg.PublicEndpoint, cfg.UseSSL)
		if err != nil {
			return nil, fmt.Errorf("public endpoint: %w", err)
		}
		if signer, err = newClient(pubHost, pubSecure); err != nil {
			return nil, err
		}
	}
	return &Storage{client: client, signer: signer, bucket: cfg.Bucket}, nil
}

func parseEndpoint(endpoint string, useSSL bool) (host string, secure bool, err error) {
	if endpoint == "" {
		return "", false, errors.New("storage: endpoint is required")
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, useSSL, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("storage: parse endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		return "", false, fmt.Errorf("storage: endpoint %q must be http(s)://host[:port]", endpoint)
	}
	return u.Host, u.Scheme == "https", nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("storage: "+format+": %w", append(args, app.ErrInvalid)...)
}

func checkTTL(ttl time.Duration) error {
	if ttl < time.Second || ttl > maxTTL {
		return invalid("ttl %v out of range [1s, 7d]", ttl)
	}
	return nil
}

// PresignPut signs Content-Type and Content-Length into the URL, so the
// upload must send exactly these headers (and body size) or the server
// rejects it with a signature error.
func (s *Storage) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (app.PresignedRequest, error) {
	if key == "" {
		return app.PresignedRequest{}, invalid("empty key")
	}
	if contentType == "" {
		return app.PresignedRequest{}, invalid("empty content type")
	}
	if size <= 0 {
		return app.PresignedRequest{}, invalid("size %d must be positive", size)
	}
	if err := checkTTL(ttl); err != nil {
		return app.PresignedRequest{}, err
	}
	headers := map[string]string{
		"Content-Type":   contentType,
		"Content-Length": strconv.FormatInt(size, 10),
	}
	signed := http.Header{}
	for k, v := range headers {
		signed.Set(k, v)
	}
	expiresAt := time.Now().Add(ttl)
	u, err := s.signer.PresignHeader(ctx, http.MethodPut, s.bucket, key, ttl, nil, signed)
	if err != nil {
		return app.PresignedRequest{}, fmt.Errorf("storage: presign put: %w", err)
	}
	return app.PresignedRequest{URL: u.String(), Method: http.MethodPut, Headers: headers, ExpiresAt: expiresAt}, nil
}

// PresignGet returns a download URL that makes the browser save the object
// as fileName (RFC 6266 / RFC 5987 encoding, so Thai names work).
func (s *Storage) PresignGet(ctx context.Context, key, fileName string, ttl time.Duration) (app.PresignedRequest, error) {
	if key == "" {
		return app.PresignedRequest{}, invalid("empty key")
	}
	if err := checkTTL(ttl); err != nil {
		return app.PresignedRequest{}, err
	}
	params := url.Values{"response-content-disposition": {contentDisposition(fileName)}}
	expiresAt := time.Now().Add(ttl)
	u, err := s.signer.Presign(ctx, http.MethodGet, s.bucket, key, ttl, params)
	if err != nil {
		return app.PresignedRequest{}, fmt.Errorf("storage: presign get: %w", err)
	}
	return app.PresignedRequest{URL: u.String(), Method: http.MethodGet, ExpiresAt: expiresAt}, nil
}

// Stat returns the size of the stored object, or an error wrapping
// app.ErrNotFound when the key does not exist.
func (s *Storage) Stat(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, invalid("empty key")
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return 0, fmt.Errorf("storage: object %q: %w", key, app.ErrNotFound)
		}
		return 0, fmt.Errorf("storage: stat: %w", err)
	}
	return info.Size, nil
}

// contentDisposition builds `attachment; filename*=UTF-8”<name>`. The name is
// reduced to its base name and stripped of control characters, which also
// rules out header injection.
func contentDisposition(fileName string) string {
	name := fileName
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if utf8.RuneCountInString(name) > maxFileNameRunes {
		name = string([]rune(name)[:maxFileNameRunes])
	}
	if name == "" {
		return "attachment"
	}
	return "attachment; filename*=UTF-8''" + encodeExtValue(name)
}

// encodeExtValue percent-encodes everything but the RFC 5987 attr-char set.
func encodeExtValue(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}
