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
