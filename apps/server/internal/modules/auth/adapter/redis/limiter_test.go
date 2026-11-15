package redis_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/redis"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
)

var _ usecase.LoginLimiter = (*redis.LoginLimiter)(nil)

// startRedis returns a client on a fresh redis:7-alpine container.
func startRedis(t *testing.T) *goredis.Client {
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
	opts, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestLimiterBlocksAfterLimitFailures(t *testing.T) {
	ctx := context.Background()
	l := redis.NewLoginLimiter(startRedis(t), 5, time.Minute)

	for i := range 5 {
		if blocked, err := l.Blocked(ctx, "email:a@b.co"); err != nil || blocked {
			t.Fatalf("after %d failures: blocked = %v, err = %v", i, blocked, err)
		}
		if err := l.Fail(ctx, "email:a@b.co"); err != nil {
			t.Fatal(err)
		}
	}
	if blocked, err := l.Blocked(ctx, "email:a@b.co"); err != nil || !blocked {
		t.Fatalf("after 5 failures: blocked = %v, err = %v", blocked, err)
	}
	if blocked, _ := l.Blocked(ctx, "email:other@b.co"); blocked {
		t.Fatal("another key is blocked")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	ctx := context.Background()
	l := redis.NewLoginLimiter(startRedis(t), 2, 300*time.Millisecond)
	_ = l.Fail(ctx, "k")
	_ = l.Fail(ctx, "k")
	if blocked, _ := l.Blocked(ctx, "k"); !blocked {
		t.Fatal("not blocked after 2 failures")
	}
	time.Sleep(400 * time.Millisecond)
	if blocked, err := l.Blocked(ctx, "k"); err != nil || blocked {
		t.Fatalf("still blocked after the window: blocked = %v, err = %v", blocked, err)
	}
	// A new window starts at the next failure.
	_ = l.Fail(ctx, "k")
	if blocked, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("blocked after one failure in a new window")
	}
}

// Later failures must not push the window out, and the key must always carry
// an expiry so a crash between commands cannot leave a permanent block.
func TestLimiterFailKeepsFirstWindow(t *testing.T) {
	ctx := context.Background()
	client := startRedis(t)
	l := redis.NewLoginLimiter(client, 5, time.Minute)

	_ = l.Fail(ctx, "k")
	keys := client.Keys(ctx, "*").Val()
	if len(keys) != 1 {
		t.Fatalf("keys = %v, want one", keys)
	}
	first := client.PTTL(ctx, keys[0]).Val()
	if first <= 0 || first > time.Minute {
		t.Fatalf("PTTL after first failure = %v", first)
	}
	// Shorten the expiry; a second failure must not reset it to a minute.
	client.PExpire(ctx, keys[0], 10*time.Second)
	_ = l.Fail(ctx, "k")
	if got := client.PTTL(ctx, keys[0]).Val(); got <= 0 || got > 10*time.Second {
		t.Fatalf("PTTL after second failure = %v, want at most 10s", got)
	}
}

func TestLimiterIsSharedAcrossInstances(t *testing.T) {
	ctx := context.Background()
	client := startRedis(t)
	a := redis.NewLoginLimiter(client, 3, time.Minute)
	b := redis.NewLoginLimiter(client, 3, time.Minute)
	_ = a.Fail(ctx, "k")
	_ = b.Fail(ctx, "k")
	_ = a.Fail(ctx, "k")
	if blocked, _ := b.Blocked(ctx, "k"); !blocked {
		t.Fatal("second server instance does not see the failures of the first")
	}
}

func TestLimiterConcurrentFailuresAreAllCounted(t *testing.T) {
	ctx := context.Background()
	l := redis.NewLoginLimiter(startRedis(t), 50, time.Minute)
	var wg sync.WaitGroup
	for range 49 {
		wg.Go(func() { _ = l.Fail(ctx, "k") })
	}
	wg.Wait()
	if blocked, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("blocked after 49 of 50 failures")
	}
	_ = l.Fail(ctx, "k")
	if blocked, _ := l.Blocked(ctx, "k"); !blocked {
		t.Fatal("not blocked after 50 failures")
	}
}

func TestLimiterReportsRedisErrors(t *testing.T) {
	ctx := context.Background()
	client := startRedis(t)
	l := redis.NewLoginLimiter(client, 5, time.Minute)
	_ = client.Close()
	if _, err := l.Blocked(ctx, "k"); err == nil {
		t.Fatal("Blocked: want an error when Redis is unreachable")
	}
	if err := l.Fail(ctx, "k"); err == nil {
		t.Fatal("Fail: want an error when Redis is unreachable")
	}
}
