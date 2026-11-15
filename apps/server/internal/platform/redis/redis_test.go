package redis_test

import (
	"context"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
)

// startRedis returns the URL of a fresh redis:7-alpine container.
func startRedis(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	ctx := context.Background()
	c, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Skipf("skipping integration test: cannot start redis container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	url, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func TestOpenFailsOnBadURL(t *testing.T) {
	if _, err := redis.Open(context.Background(), "not a url"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenFailsWhenUnreachable(t *testing.T) {
	if _, err := redis.Open(context.Background(), "redis://127.0.0.1:1/0"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenPings(t *testing.T) {
	ctx := context.Background()
	client, err := redis.Open(ctx, startRedis(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Set(ctx, "k", "v", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if got := client.Get(ctx, "k").Val(); got != "v" {
		t.Fatalf("got %q, want v", got)
	}
}
