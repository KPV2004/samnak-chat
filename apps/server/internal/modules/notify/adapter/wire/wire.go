// Package wire holds the JSON shapes of notify that both REST responses and
// WebSocket events use, so a client decodes one Notification.
package wire

import (
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// Notification is one notification as its owner sees it.
type Notification struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind" doc:"<module>.<name>, e.g. chat.mention"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Link      string         `json:"link" doc:"Web path to open, e.g. /chat/{channel_id}"`
	Data      map[string]any `json:"data" nullable:"false" doc:"Kind-specific fields for clients that know the kind"`
	ReadAt    *time.Time     `json:"read_at"`
	CreatedAt time.Time      `json:"created_at"`
}

// ToNotification converts a stored notification.
func ToNotification(n domain.Notification) Notification {
	data := n.Data
	if data == nil {
		data = map[string]any{}
	}
	return Notification{
		ID: n.ID, Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link, Data: data,
		ReadAt: n.ReadAt, CreatedAt: n.CreatedAt.UTC(),
	}
}

// ToNotifications converts a page.
func ToNotifications(ns []domain.Notification) []Notification {
	out := make([]Notification, len(ns))
	for i, n := range ns {
		out[i] = ToNotification(n)
	}
	return out
}

// Preferences is one user's notification settings (NOTI-07), as GET returns
// them and PUT replaces them. Times are "HH:MM" in the user's time zone; both
// are null when there is no do-not-disturb window.
type Preferences struct {
	DisabledKinds   []string `json:"disabled_kinds" nullable:"false" doc:"Kinds the user does not want, e.g. chat.dm"`
	MutedChannelIDs []string `json:"muted_channel_ids" nullable:"false" doc:"Channels whose notices are dropped"`
	DNDStart        *string  `json:"dnd_start" nullable:"true" doc:"Do-not-disturb start, HH:MM; null with dnd_end null for none"`
	DNDEnd          *string  `json:"dnd_end" nullable:"true" doc:"Do-not-disturb end, HH:MM; wraps past midnight when before dnd_start"`
}

// ToPreferences converts stored settings.
func ToPreferences(p domain.Preferences) Preferences {
	out := Preferences{DisabledKinds: p.DisabledKinds, MutedChannelIDs: p.MutedChannelIDs}
	if p.DNDStart != "" {
		start, end := p.DNDStart, p.DNDEnd
		out.DNDStart, out.DNDEnd = &start, &end
	}
	return out
}

// ToDomain converts settings a client sent. Owner fields are not set here: the
// use case takes them from the Principal.
func (p Preferences) ToDomain() domain.Preferences {
	out := domain.Preferences{DisabledKinds: p.DisabledKinds, MutedChannelIDs: p.MutedChannelIDs}
	if p.DNDStart != nil {
		out.DNDStart = *p.DNDStart
	}
	if p.DNDEnd != nil {
		out.DNDEnd = *p.DNDEnd
	}
	return out
}

// Names of the frames notify sends to a user's own devices (NOTI-03).
const (
	EventCreated = "notification.created"
	EventRead    = "notification.read"
)

// CreatedEvent is the `data` of notification.created. quiet is true inside the
// user's do-not-disturb window: the client updates the bell but shows no toast
// (NOTI-07).
type CreatedEvent struct {
	WorkspaceID  string       `json:"workspace_id"`
	Notification Notification `json:"notification"`
	UnreadCount  int64        `json:"unread_count"`
	Quiet        bool         `json:"quiet"`
}

// ReadEvent is the `data` of notification.read, so the user's other devices
// clear the bell. all is true for "mark all as read".
type ReadEvent struct {
	WorkspaceID string   `json:"workspace_id"`
	IDs         []string `json:"ids" nullable:"false"`
	All         bool     `json:"all"`
	UnreadCount int64    `json:"unread_count"`
}
