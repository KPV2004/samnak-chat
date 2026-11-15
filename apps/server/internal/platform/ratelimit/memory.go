package ratelimit

import (
	"context"
	"sync"
	"time"
)

// sweepEvery bounds how often Memory scans for expired windows.
const sweepEvery = time.Minute

type window struct {
	count   int
	expires time.Time
}

// Memory is a Limiter that keeps counters in the process. It suits tests and
// a single instance; with several server instances use Redis (NFR-04).
type Memory struct {
	now func() time.Time

	mu        sync.Mutex
	windows   map[string]*window
	nextSweep time.Time
}

// NewMemory returns an empty in-memory Limiter.
func NewMemory() *Memory {
	return newMemory(time.Now)
}

func newMemory(now func() time.Time) *Memory {
	return &Memory{now: now, windows: make(map[string]*window)}
}

// Allow implements Limiter.
func (m *Memory) Allow(_ context.Context, key string, limit int, win time.Duration) (bool, error) {
	if err := validate(limit, win); err != nil {
		return false, err
	}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweep(now)

	w, ok := m.windows[key]
	if !ok || !now.Before(w.expires) {
		w = &window{expires: now.Add(win)}
		m.windows[key] = w
	}
	w.count++
	return w.count <= limit, nil
}

// sweep drops expired windows so keys seen once do not stay forever. The
// caller holds m.mu.
func (m *Memory) sweep(now time.Time) {
	if now.Before(m.nextSweep) {
		return
	}
	for k, w := range m.windows {
		if !now.Before(w.expires) {
			delete(m.windows, k)
		}
	}
	m.nextSweep = now.Add(sweepEvery)
}
