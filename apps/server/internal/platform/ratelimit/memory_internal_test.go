package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemorySweepsExpiredWindows(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newMemory(func() time.Time { return now })
	ctx := context.Background()

	for _, k := range []string{"a", "b", "c"} {
		if _, err := m.Allow(ctx, k, 1, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(2 * sweepEvery)
	if _, err := m.Allow(ctx, "d", 1, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := len(m.windows); got != 1 {
		t.Fatalf("%d windows kept, want only the live one", got)
	}
}
