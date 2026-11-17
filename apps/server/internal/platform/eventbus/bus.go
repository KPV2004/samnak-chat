package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

// Bus is an app.EventBus.
//
// Publish runs the handlers for the event name synchronously, in the order
// they were subscribed, so tests are deterministic. A handler that returns an
// error or panics is logged and never affects the publisher or other handlers.
//
// A Bus made by New has no database and works only in memory. A Bus made by
// NewOutbox does the same outside a transaction; inside database.TxManager.Do
// it writes the event to the outbox instead, and Start runs the relay that
// delivers it after the commit (PLT-10, see outbox.go).
type Bus struct {
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string][]app.Handler
	outbox
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

// Publish outside a transaction, or on a bus without a database, returns nil
// and only logs handler failures. Inside a transaction on an outbox bus it
// returns an error when the event cannot be written, for example because its
// type was not declared.
func (b *Bus) Publish(ctx context.Context, evt app.Event) error {
	if b.pool != nil {
		if tx, ok := database.TxFromContext(ctx); ok {
			return b.enqueue(ctx, tx, evt)
		}
	}
	b.deliver(ctx, evt)
	return nil
}

// deliver runs every handler subscribed to evt's name, in subscription order.
func (b *Bus) deliver(ctx context.Context, evt app.Event) {
	name := evt.EventName()
	b.mu.RLock()
	hs := b.handlers[name] // append-only, so this snapshot is safe to read unlocked
	b.mu.RUnlock()
	for _, h := range hs {
		b.call(ctx, name, h, evt)
	}
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
