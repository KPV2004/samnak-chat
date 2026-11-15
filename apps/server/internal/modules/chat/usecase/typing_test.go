package usecase

import (
	"testing"
	"time"
)

func TestTypingLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newTypingLimiter(3*time.Second, func() time.Time { return now })

	if !l.allow("u1", "c1") {
		t.Fatal("first typing was refused")
	}
	if l.allow("u1", "c1") {
		t.Error("a second typing in the same instant was allowed")
	}
	now = now.Add(2999 * time.Millisecond)
	if l.allow("u1", "c1") {
		t.Error("typing 2.999 s later was allowed")
	}
	if !l.allow("u2", "c1") || !l.allow("u1", "c2") {
		t.Error("another user or channel shares the window")
	}
	now = now.Add(time.Millisecond)
	if !l.allow("u1", "c1") {
		t.Error("typing 3 s later was refused")
	}
}

func TestTypingLimiterForgetsOldEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newTypingLimiter(3*time.Second, func() time.Time { return now })
	for i := range maxTypingEntries + 10 {
		l.allow("u", string(rune('a'+i%26))+time.Duration(i).String())
	}
	now = now.Add(time.Minute)
	l.allow("u", "fresh")
	if n := l.size(); n > maxTypingEntries {
		t.Errorf("limiter keeps %d entries after they expired, want at most %d", n, maxTypingEntries)
	}
	if n := l.size(); n != 1 {
		t.Errorf("limiter keeps %d entries, want only the fresh one", n)
	}
}
