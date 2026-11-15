package storage_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/storage"
)

func newOffline(t *testing.T) *storage.Storage {
	t.Helper()
	s, err := storage.New(context.Background(), storage.Config{
		Endpoint:  "http://minio.test:9000",
		Bucket:    "samnak",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewValidatesConfig(t *testing.T) {
	good := storage.Config{Endpoint: "localhost:9000", Bucket: "b", AccessKey: "a", SecretKey: "s"}
	cases := map[string]func(*storage.Config){
		"no endpoint":   func(c *storage.Config) { c.Endpoint = "" },
		"no bucket":     func(c *storage.Config) { c.Bucket = "" },
		"no access key": func(c *storage.Config) { c.AccessKey = "" },
		"no secret key": func(c *storage.Config) { c.SecretKey = "" },
		"bad endpoint":  func(c *storage.Config) { c.Endpoint = "http://" },
		"endpoint path": func(c *storage.Config) { c.Endpoint = "http://host:9000/sub" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := good
			mutate(&cfg)
			if _, err := storage.New(context.Background(), cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := storage.New(context.Background(), good); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestPresignPutSignsTypeAndLength(t *testing.T) {
	s := newOffline(t)
	before := time.Now()
	req, err := s.PresignPut(context.Background(), "chat/ws/att", "image/png", 1234, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "PUT" {
		t.Fatalf("method = %q", req.Method)
	}
	if req.Headers["Content-Type"] != "image/png" || req.Headers["Content-Length"] != "1234" {
		t.Fatalf("headers = %v", req.Headers)
	}
	u, err := url.Parse(req.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "minio.test:9000" || u.Path != "/samnak/chat/ws/att" {
		t.Fatalf("path-style URL expected, got %s", req.URL)
	}
	q := u.Query()
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	for _, want := range []string{"content-type", "content-length", "host"} {
		if !contains(signed, want) {
			t.Fatalf("signed headers %v lack %s", signed, want)
		}
	}
	if q.Get("X-Amz-Expires") != "900" {
		t.Fatalf("X-Amz-Expires = %q, want 900", q.Get("X-Amz-Expires"))
	}
	if d := req.ExpiresAt.Sub(before.Add(15 * time.Minute)); d < -time.Second || d > 5*time.Second {
		t.Fatalf("ExpiresAt off by %v", d)
	}
}

func TestPresignPutRejectsBadInput(t *testing.T) {
	s := newOffline(t)
	ctx := context.Background()
	cases := map[string]func() error{
		"empty key":    func() error { _, err := s.PresignPut(ctx, "", "text/plain", 1, time.Minute); return err },
		"zero size":    func() error { _, err := s.PresignPut(ctx, "k", "text/plain", 0, time.Minute); return err },
		"neg size":     func() error { _, err := s.PresignPut(ctx, "k", "text/plain", -1, time.Minute); return err },
		"no type":      func() error { _, err := s.PresignPut(ctx, "k", "", 1, time.Minute); return err },
		"zero ttl":     func() error { _, err := s.PresignPut(ctx, "k", "text/plain", 1, 0); return err },
		"ttl too long": func() error { _, err := s.PresignPut(ctx, "k", "text/plain", 1, 8*24*time.Hour); return err },
		"empty get key": func() error {
			_, err := s.PresignGet(ctx, "", "f.txt", time.Minute)
			return err
		},
		"get zero ttl": func() error { _, err := s.PresignGet(ctx, "k", "f.txt", 0); return err },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, app.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestPresignGetContentDisposition(t *testing.T) {
	s := newOffline(t)
	cases := []struct{ name, want string }{
		{"report.pdf", "attachment; filename*=UTF-8''report.pdf"},
		{"สรุปประชุม.pdf", "attachment; filename*=UTF-8''%E0%B8%AA%E0%B8%A3%E0%B8%B8%E0%B8%9B%E0%B8%9B%E0%B8%A3%E0%B8%B0%E0%B8%8A%E0%B8%B8%E0%B8%A1.pdf"},
		{"a b;c\"d.txt", "attachment; filename*=UTF-8''a%20b%3Bc%22d.txt"},
		{"x\r\nSet-Cookie: a=b", "attachment; filename*=UTF-8''xSet-Cookie%3A%20a%3Db"},
		{"../../etc/passwd", "attachment; filename*=UTF-8''passwd"},
		{"", "attachment"},
	}
	for _, tc := range cases {
		req, err := s.PresignGet(context.Background(), "chat/ws/att", tc.name, 15*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if req.Method != "GET" {
			t.Fatalf("method = %q", req.Method)
		}
		u, err := url.Parse(req.URL)
		if err != nil {
			t.Fatal(err)
		}
		if got := u.Query().Get("response-content-disposition"); got != tc.want {
			t.Errorf("%q: disposition = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
