// Package domain holds the notification model and the rules that hold
// regardless of storage (SRS section 8).
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Notification is one stored notice for one user (NOTI-02). It only exists
// once it is in the database; notify delivers after the row commits.
type Notification struct {
	ID          string
	WorkspaceID string
	UserID      string
	Kind        string
	Title       string
	Body        string
	Link        string
	Data        map[string]any
	// Key is the source object, e.g. "chat.message:{id}". Empty means the
	// row is never deduplicated.
	Key       string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// Valid reports whether a rule's notification can be stored. A notification
// without a user, workspace or kind cannot be addressed to anyone, and an ID
// that is not a UUID can never match a row, so it is dropped like an empty
// one. Dropping is logged; it never fails the event (NOTI-02).
func Valid(n app.Notification) bool {
	if strings.TrimSpace(n.UserID) == "" || strings.TrimSpace(n.WorkspaceID) == "" || strings.TrimSpace(n.Kind) == "" {
		return false
	}
	if _, err := uuid.Parse(n.UserID); err != nil {
		return false
	}
	if _, err := uuid.Parse(n.WorkspaceID); err != nil {
		return false
	}
	return true
}

// New converts a rule's notification into a storable one. The caller supplies
// the ID and the time so it controls identity and the clock.
func New(n app.Notification, id string, at time.Time) Notification {
	data := n.Data
	if data == nil {
		data = map[string]any{}
	}
	return Notification{
		ID:          id,
		WorkspaceID: n.WorkspaceID,
		UserID:      n.UserID,
		Kind:        n.Kind,
		Title:       n.Title,
		Body:        n.Body,
		Link:        n.Link,
		Data:        data,
		Key:         n.Key,
		CreatedAt:   at.UTC(),
	}
}
