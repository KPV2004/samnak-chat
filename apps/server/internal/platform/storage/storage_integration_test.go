package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/storage"
)

const (
	minioUser   = "minioadmin"
	minioSecret = "minioadmin"
	bucket      = "samnak-test"
)

// startMinio returns a Storage backed by a fresh MinIO container.
func startMinio(t *testing.T) *storage.Storage {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bitnamilegacy/minio:latest",
			ExposedPorts: []string{"9000/tcp"},
			Env: map[string]string{
				"MINIO_ROOT_USER":     minioUser,
				"MINIO_ROOT_PASSWORD": minioSecret,
			},
			WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("skipping integration test: cannot start minio container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	endpoint, err := c.PortEndpoint(ctx, "9000/tcp", "")
	if err != nil {
		t.Fatal(err)
	}

	admin, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(minioUser, minioSecret, "")})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}

	s, err := storage.New(ctx, storage.Config{
		Endpoint:  "http://" + endpoint,
		Bucket:    bucket,
		AccessKey: minioUser,
		SecretKey: minioSecret,
		PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// upload performs the presigned PUT exactly as a client would. The body and
// the Content-Length header it sends can differ from the signed ones.
func upload(t *testing.T, req app.PresignedRequest, contentType string, body []byte) int {
	t.Helper()
	r, err := http.NewRequest(req.Method, req.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range req.Headers {
		r.Header.Set(k, v)
	}
	r.Header.Set("Content-Type", contentType)
	r.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestMinioUploadAndDownload(t *testing.T) {
	s := startMinio(t)
	ctx := context.Background()
	key := "chat/ws-1/att-1"
	body := []byte("hello สวัสดี")

	if _, err := s.Stat(ctx, key); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Stat before upload: err = %v, want ErrNotFound", err)
	}

	put, err := s.PresignPut(ctx, key, "text/plain", int64(len(body)), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// A body of another size and a different content type are both rejected,
	// and nothing is stored.
	if code := upload(t, put, "text/plain", append(body, '!')); code != http.StatusForbidden {
		t.Fatalf("mismatched size: status %d, want 403", code)
	}
	if code := upload(t, put, "application/x-evil", body); code != http.StatusForbidden {
		t.Fatalf("mismatched content type: status %d, want 403", code)
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Stat after rejected uploads: err = %v, want ErrNotFound", err)
	}

	if code := upload(t, put, "text/plain", body); code != http.StatusOK {
		t.Fatalf("matching upload: status %d, want 200", code)
	}
	size, err := s.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(body)) {
		t.Fatalf("Stat size = %d, want %d", size, len(body))
	}

	get, err := s.PresignGet(ctx, key, "สรุปประชุม.txt", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(get.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("download: status %d body %q", resp.StatusCode, got)
	}
	wantDisp := "attachment; filename*=UTF-8''%E0%B8%AA%E0%B8%A3%E0%B8%B8%E0%B8%9B%E0%B8%9B%E0%B8%A3%E0%B8%B0%E0%B8%8A%E0%B8%B8%E0%B8%A1.txt"
	if d := resp.Header.Get("Content-Disposition"); d != wantDisp {
		t.Fatalf("Content-Disposition = %q, want %q", d, wantDisp)
	}
}

func TestMinioPresignedURLExpires(t *testing.T) {
	s := startMinio(t)
	ctx := context.Background()
	put, err := s.PresignPut(ctx, "chat/ws-1/late", "text/plain", 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	if code := upload(t, put, "text/plain", []byte("hi")); code != http.StatusForbidden {
		t.Fatalf("expired URL: status %d, want 403", code)
	}
}
