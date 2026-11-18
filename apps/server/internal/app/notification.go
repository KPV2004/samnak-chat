package app

import (
	"context"
	"maps"
	"slices"
	"sync"
)

// Notification is one notice for one user, produced by a source module's rule
// (NOTI-01). notify stores it before delivering it (NOTI-02).
type Notification struct {
	WorkspaceID string
	UserID      string
	Kind        string // "<module>.<name>", e.g. "chat.mention"
	Title       string
	Body        string
	Link        string // web path, e.g. "/chat/{channel_id}"
	// ChannelID is the chat channel the notice is about, if any. notify drops
	// notices for channels the user has muted (NOTI-07).
	ChannelID string
	Data      map[string]any
	// Key makes a notification idempotent: notify stores at most one
	// notification per (UserID, Key), so a redelivered event adds nothing.
	// Use the source object, e.g. "chat.message:{message_id}".
	Key string
}

// NotificationRule turns an event into the notifications it should produce.
type NotificationRule func(ctx context.Context, evt Event) ([]Notification, error)

// NotificationRegistry is what modules see as Deps.Notifications. They call On
// in Init and never call notify directly (NOTI-01).
type NotificationRegistry interface {
	On(eventName string, rule NotificationRule)
}

// NotificationRuleSource is the read side notify uses in Subscribe, after
// every module's Init has registered its rules.
type NotificationRuleSource interface {
	// Rules returns a copy of the registered rules by event name.
	Rules() map[string][]NotificationRule
}

// NotificationRules collects rules. The server passes one as
// Deps.Notifications whether or not notify is enabled; when notify is
// disabled nobody reads it, so the rules never run and the source module
// works as before (NOTI-05).
type NotificationRules struct {
	mu    sync.Mutex
	rules map[string][]NotificationRule
}

var (
	_ NotificationRegistry   = (*NotificationRules)(nil)
	_ NotificationRuleSource = (*NotificationRules)(nil)
)

func NewNotificationRules() *NotificationRules {
	return &NotificationRules{rules: map[string][]NotificationRule{}}
}

func (r *NotificationRules) On(eventName string, rule NotificationRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[eventName] = append(r.rules[eventName], rule)
}

func (r *NotificationRules) Rules() map[string][]NotificationRule {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := maps.Clone(r.rules)
	for k, v := range out {
		out[k] = slices.Clone(v)
	}
	return out
}

// NoopNotificationRegistry drops every rule. Tests use it.
type NoopNotificationRegistry struct{}

func (NoopNotificationRegistry) On(string, NotificationRule) {}
