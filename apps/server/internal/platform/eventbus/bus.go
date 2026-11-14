package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Bus is an in-memory app.EventBus.
//
// Publish runs the handlers for the event name synchronously, in the order
// they were subscribed, so tests are deterministic. A handler that returns an
// error or panics is logged and never affects the publisher or other handlers.
type Bus struct {
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string][]app.Handler
}

func New(log *slog.Logger) *Bus {
	if log == nil {
		log = slog.Default()
	}
	return &Bus{log: log, handlers: map[string][]app.Handler{}}
}

func (b *Bus) Subscribe(name string, h app.Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = append(b.handlers[name], h)
}

// Publish always returns nil; handler failures are only logged.
func (b *Bus) Publish(ctx context.Context, evt app.Event) error {
	name := evt.EventName()
	b.mu.RLock()
	hs := b.handlers[name] // append-only, so this snapshot is safe to read unlocked
	b.mu.RUnlock()
	for _, h := range hs {
		b.call(ctx, name, h, evt)
	}
	return nil
}

func (b *Bus) call(ctx context.Context, name string, h app.Handler, evt app.Event) {
	defer func() {
		if r := recover(); r != nil {
			b.log.ErrorContext(ctx, "event handler panicked", "event", name, "panic", fmt.Sprint(r))
		}
	}()
	if err := h(ctx, evt); err != nil {
		b.log.ErrorContext(ctx, "event handler failed", "event", name, "error", err)
	}
}
