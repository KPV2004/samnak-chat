package app

import "context"

// CardKind describes a chat card a module owns, e.g. "approval.request".
type CardKind struct {
	Kind   string
	Module string
}

// CardRegistry is owned by chat (a core module, always on).
type CardRegistry interface {
	Register(kind CardKind)
	// Post sends a card message to a channel and returns the message ID.
	Post(ctx context.Context, channelID, kind string, payload any, fallbackText string) (string, error)
	// Update replaces a card's payload and broadcasts card.updated.
	Update(ctx context.Context, messageID string, payload any) error
}
