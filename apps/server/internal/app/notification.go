package app

import "context"

type Notification struct {
	WorkspaceID string
	UserID      string
	Kind        string // e.g. "approval.approved"
	Title       string
	Body        string
	Link        string
	Data        map[string]any
}

// NotificationRule turns an event into the notifications it should produce.
type NotificationRule func(ctx context.Context, evt Event) ([]Notification, error)

// NotificationRegistry is owned by notify. When notify is disabled a no-op
// implementation is injected, so registering rules is always safe.
type NotificationRegistry interface {
	On(eventName string, rule NotificationRule)
}

type NoopNotificationRegistry struct{}

func (NoopNotificationRegistry) On(string, NotificationRule) {}
