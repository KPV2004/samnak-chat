package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
)

func TestOpenRejectsBadURL(t *testing.T) {
	if _, err := redis.Open(context.Background(), "not a url"); err == nil {
		t.Fatal("want error for a malformed URL")
	}
}

func TestOpenFailsWhenUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := redis.Open(ctx, "redis://127.0.0.1:1/0"); err == nil {
		t.Fatal("want ping error")
	}
}

// startRedis returns a URL for a fresh redis:7-alpine container.
func startRedis(t *testing.T) string {
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
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("skipping integration test: cannot start redis container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return "redis://" + host + ":" + port.Port() + "/0"
}

func TestCheckReportsHealthAndFailure(t *testing.T) {
	ctx := context.Background()
	c, err := redis.Open(ctx, startRedis(t))
	if err != nil {
		t.Fatal(err)
	}
	check := redis.Check(c)
	if err := check(ctx); err != nil {
		t.Fatalf("healthy check = %v", err)
	}
	_ = c.Close()
	if err := check(ctx); err == nil {
		t.Fatal("want error after the client is closed")
	}
}

func TestRateLimiterFixedWindow(t *testing.T) {
	ctx := context.Background()
	c, err := redis.Open(ctx, startRedis(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	rl := redis.NewRateLimiter(c)

	const window = 700 * time.Millisecond
	for i := 1; i <= 5; i++ {
		ok, err := rl.Allow(ctx, "login:a@x.io", 5, window)
		if err != nil || !ok {
			t.Fatalf("attempt %d: ok=%v err=%v, want allowed", i, ok, err)
		}
	}
	if ok, err := rl.Allow(ctx, "login:a@x.io", 5, window); err != nil || ok {
		t.Fatalf("attempt 6: ok=%v err=%v, want denied", ok, err)
	}
	if ok, err := rl.Allow(ctx, "login:b@x.io", 5, window); err != nil || !ok {
		t.Fatalf("other key: ok=%v err=%v, want allowed", ok, err)
	}

	time.Sleep(window + 200*time.Millisecond)
	if ok, err := rl.Allow(ctx, "login:a@x.io", 5, window); err != nil || !ok {
		t.Fatalf("after window: ok=%v err=%v, want allowed", ok, err)
	}
}

func TestRateLimiterAlwaysSetsExpiry(t *testing.T) {
	ctx := context.Background()
	c, err := redis.Open(ctx, startRedis(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := redis.NewRateLimiter(c).Allow(ctx, "k", 1, time.Minute); err != nil {
		t.Fatal(err)
	}
	if ttl := c.PTTL(ctx, "k").Val(); ttl <= 0 {
		t.Fatalf("ttl = %v, want a positive expiry so keys cannot live forever", ttl)
	}
}
