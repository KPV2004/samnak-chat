package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/memory"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestLimiterBlocksAfterLimitFailures(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := memory.NewLoginLimiter(5, time.Minute, clock.Now)

	for i := range 5 {
		if blocked, _ := l.Blocked(ctx, "k"); blocked {
			t.Fatalf("blocked after %d failures", i)
		}
		if err := l.Fail(ctx, "k"); err != nil {
			t.Fatal(err)
		}
	}
	if blocked, _ := l.Blocked(ctx, "k"); !blocked {
		t.Fatal("not blocked after 5 failures")
	}
	if blocked, _ := l.Blocked(ctx, "other"); blocked {
		t.Fatal("another key is blocked")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := memory.NewLoginLimiter(2, time.Minute, clock.Now)
	_ = l.Fail(ctx, "k")
	_ = l.Fail(ctx, "k")
	clock.Advance(59 * time.Second)
	if blocked, _ := l.Blocked(ctx, "k"); !blocked {
		t.Fatal("window ended early")
	}
	clock.Advance(2 * time.Second)
	if blocked, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("still blocked after the window")
	}
	// A failure after expiry starts a new window with a count of one.
	_ = l.Fail(ctx, "k")
	if blocked, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("old failures were carried over")
	}
}

func TestLimiterForgetsExpiredKeys(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := memory.NewLoginLimiter(1, time.Minute, clock.Now)
	for i := range 100 {
		_ = l.Fail(ctx, "k"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	clock.Advance(2 * time.Minute)
	_ = l.Fail(ctx, "fresh")
	if n := l.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1: expired keys are not purged", n)
	}
}

func TestLimiterIsSafeForConcurrentUse(t *testing.T) {
	ctx := context.Background()
	l := memory.NewLoginLimiter(5, time.Minute, time.Now)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_ = l.Fail(ctx, "k")
			_, _ = l.Blocked(ctx, "k")
		})
	}
	wg.Wait()
	if blocked, _ := l.Blocked(ctx, "k"); !blocked {
		t.Fatal("not blocked after 50 failures")
	}
}
