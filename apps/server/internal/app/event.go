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

// EventBus delivers events between modules.
//
// Publish outside a transaction runs the handlers now. Publish inside
// TxManager.Do writes the event to the outbox in that transaction; it is
// delivered only after the commit, at least once, also after a crash
// (PLT-10). A rolled-back transaction delivers nothing. Each outbox event is
// handled on one server instance.
type EventBus interface {
	Publisher
	// Subscribe registers h for events named name. A handler that errors or
	// panics is logged and never affects the publisher or other handlers.
	Subscribe(name string, h Handler)
	// Declare lists the event types a module publishes inside transactions,
	// so the outbox can decode them back into those types. Call it in Init
	// with zero values, e.g. Declare(TaskCreated{}, TaskAssigned{}). The
	// event must round-trip through encoding/json. Publishing an undeclared
	// event inside a transaction is an error.
	Declare(protos ...Event)
}
