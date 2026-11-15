package ratelimit_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/ratelimit"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
)

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
	client, err := redis.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// runSuite is the contract every Limiter implementation must satisfy.
func runSuite(t *testing.T, newLimiter func(t *testing.T) ratelimit.Limiter) {
	ctx := context.Background()

	t.Run("allows up to the limit then denies", func(t *testing.T) {
		l := newLimiter(t)
		for i := 1; i <= 5; i++ {
			ok, err := l.Allow(ctx, "login:a@example.com", 5, time.Minute)
			if err != nil || !ok {
				t.Fatalf("call %d: ok=%v err=%v, want allowed", i, ok, err)
			}
		}
		for i := 6; i <= 8; i++ {
			ok, err := l.Allow(ctx, "login:a@example.com", 5, time.Minute)
			if err != nil || ok {
				t.Fatalf("call %d: ok=%v err=%v, want denied", i, ok, err)
			}
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		l := newLimiter(t)
		if ok, err := l.Allow(ctx, "a", 1, time.Minute); err != nil || !ok {
			t.Fatalf("a first: ok=%v err=%v", ok, err)
		}
		if ok, err := l.Allow(ctx, "a", 1, time.Minute); err != nil || ok {
			t.Fatalf("a second: ok=%v err=%v, want denied", ok, err)
		}
		if ok, err := l.Allow(ctx, "b", 1, time.Minute); err != nil || !ok {
			t.Fatalf("b first: ok=%v err=%v, want allowed", ok, err)
		}
	})

	t.Run("window expiry resets the count", func(t *testing.T) {
		l := newLimiter(t)
		const window = 300 * time.Millisecond
		if ok, err := l.Allow(ctx, "k", 1, window); err != nil || !ok {
			t.Fatalf("first: ok=%v err=%v", ok, err)
		}
		if ok, err := l.Allow(ctx, "k", 1, window); err != nil || ok {
			t.Fatalf("second: ok=%v err=%v, want denied", ok, err)
		}
		time.Sleep(window + 200*time.Millisecond)
		if ok, err := l.Allow(ctx, "k", 1, window); err != nil || !ok {
			t.Fatalf("after window: ok=%v err=%v, want allowed", ok, err)
		}
	})

	t.Run("denied calls do not extend the window", func(t *testing.T) {
		l := newLimiter(t)
		const window = 400 * time.Millisecond
		if ok, err := l.Allow(ctx, "k", 1, window); err != nil || !ok {
			t.Fatalf("first: ok=%v err=%v", ok, err)
		}
		deadline := time.Now().Add(window - 100*time.Millisecond)
		for time.Now().Before(deadline) {
			if _, err := l.Allow(ctx, "k", 1, window); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(250 * time.Millisecond)
		if ok, err := l.Allow(ctx, "k", 1, window); err != nil || !ok {
			t.Fatalf("after window: ok=%v err=%v, want allowed", ok, err)
		}
	})

	t.Run("concurrent calls never exceed the limit", func(t *testing.T) {
		l := newLimiter(t)
		const limit, callers = 10, 50
		var allowed atomic.Int32
		var wg sync.WaitGroup
		for range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := l.Allow(ctx, "shared", limit, time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				if ok {
					allowed.Add(1)
				}
			}()
		}
		wg.Wait()
		if got := allowed.Load(); got != limit {
			t.Fatalf("allowed %d, want %d", got, limit)
		}
	})

	t.Run("rejects a non-positive limit or window", func(t *testing.T) {
		l := newLimiter(t)
		for _, tc := range []struct {
			limit  int
			window time.Duration
		}{{0, time.Minute}, {-1, time.Minute}, {1, 0}, {1, -time.Second}} {
			name := fmt.Sprintf("limit=%d window=%s", tc.limit, tc.window)
			if ok, err := l.Allow(ctx, "k", tc.limit, tc.window); err == nil || ok {
				t.Errorf("%s: ok=%v err=%v, want error", name, ok, err)
			}
		}
	})
}

func TestMemory(t *testing.T) {
	runSuite(t, func(*testing.T) ratelimit.Limiter { return ratelimit.NewMemory() })
}

func TestRedis(t *testing.T) {
	client := startRedis(t)
	var n atomic.Int32
	runSuite(t, func(*testing.T) ratelimit.Limiter {
		// One container for the suite; a prefix per subtest keeps keys apart.
		return ratelimit.NewRedis(client, fmt.Sprintf("test%d:", n.Add(1)))
	})
}

func TestRedisSetsExpiryOnTheKey(t *testing.T) {
	client := startRedis(t)
	ctx := context.Background()
	l := ratelimit.NewRedis(client, "p:")
	if _, err := l.Allow(ctx, "k", 5, time.Minute); err != nil {
		t.Fatal(err)
	}
	ttl, err := client.PTTL(ctx, "p:k").Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("ttl = %s, want in (0, 1m]", ttl)
	}

	// A window that is not a whole number of seconds keeps its precision.
	if _, err := l.Allow(ctx, "frac", 5, 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ttl, err = client.PTTL(ctx, "p:frac").Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= time.Second || ttl > 1500*time.Millisecond {
		t.Fatalf("ttl = %s, want in (1s, 1.5s]", ttl)
	}
}

func TestRedisReturnsErrorWhenUnavailable(t *testing.T) {
	client := startRedis(t)
	l := ratelimit.NewRedis(client, "p:")
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	ok, err := l.Allow(context.Background(), "k", 5, time.Minute)
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v, want error and not allowed", ok, err)
	}
}
