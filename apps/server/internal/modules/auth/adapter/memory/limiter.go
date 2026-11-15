// Package memory has in-process adapters. They are right for tests and for a
// single server; the Redis limiter replaces the limiter when several servers
// share one limit.
package memory

import (
	"context"
	"sync"
	"time"
)

// LoginLimiter counts failures per key in a fixed window that starts at the
// first failure. It implements usecase.LoginLimiter.
type LoginLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	entries map[string]entry
	// nextSweep is when expired keys are next removed, so memory stays bounded
	// by the number of keys that failed within one window.
	nextSweep time.Time
}

type entry struct {
	count   int
	expires time.Time
}

// NewLoginLimiter blocks a key after limit failures within window. now is the
// clock; pass time.Now outside tests.
func NewLoginLimiter(limit int, window time.Duration, now func() time.Time) *LoginLimiter {
	return &LoginLimiter{limit: limit, window: window, now: now, entries: map[string]entry{}}
}

func (l *LoginLimiter) Blocked(_ context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	return ok && e.expires.After(l.now()) && e.count >= l.limit, nil
}

func (l *LoginLimiter) Fail(_ context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	e, ok := l.entries[key]
	if !ok || !e.expires.After(now) {
		e = entry{expires: now.Add(l.window)}
	}
	e.count++
	l.entries[key] = e
	return nil
}

func (l *LoginLimiter) sweep(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for k, e := range l.entries {
		if !e.expires.After(now) {
			delete(l.entries, k)
		}
	}
	l.nextSweep = now.Add(l.window)
}

// Len returns the number of keys held, for tests.
func (l *LoginLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
