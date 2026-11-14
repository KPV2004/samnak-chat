package app

import "context"

// Event announces that something happened. Names are "<module>.<past_tense>",
// e.g. "approval.request_approved". Fields may be added, never removed.
type Event interface {
	EventName() string
}

type Publisher interface {
	Publish(ctx context.Context, evt Event) error
}

// Handler must be idempotent: delivery is at least once.
type Handler func(ctx context.Context, evt Event) error

type EventBus interface {
	Publisher
	// Subscribe registers h for events named name. A handler that errors or
	// panics is logged and never affects the publisher or other handlers.
	Subscribe(name string, h Handler)
}
