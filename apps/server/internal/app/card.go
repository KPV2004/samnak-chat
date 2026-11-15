package app

import "context"

// CardKind describes a chat card a module owns, e.g. "approval.request".
type CardKind struct {
	Kind   string // "<module>.<name>"
	Module string
}

// Card is a card message to post. Cards are often posted from event handlers
// that have no Principal, so the workspace and sender are explicit.
type Card struct {
	WorkspaceID  string
	ChannelID    string
	SenderID     string // the user the card is shown as sent by
	Kind         string // a registered CardKind.Kind
	Payload      any    // marshalled to JSON; chat never interprets it
	FallbackText string // shown when the owning module is disabled (PLT-11)
	ThreadRootID string // optional
}

// CardRegistry is owned by chat (a core module, always on). chat provides it
// in Provide; other modules get it with Lookup[CardRegistry] in Init.
type CardRegistry interface {
	// Register declares a card kind. Call it from Init.
	Register(kind CardKind)
	// Post sends a card message and returns its message ID. Kind must be
	// registered. It sends message.created like any other message.
	Post(ctx context.Context, c Card) (messageID string, err error)
	// Update replaces a card's payload and sends card.updated to everyone in
	// the channel. Only the module that registered the kind may update it.
	Update(ctx context.Context, workspaceID, messageID string, payload any) error
	// DirectChannel returns the DM channel between userIDs in a workspace,
	// creating it if needed, so a module can send a card to a person.
	DirectChannel(ctx context.Context, workspaceID string, userIDs []string) (channelID string, err error)
}
