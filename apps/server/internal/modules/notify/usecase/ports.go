// Package usecase holds the notify flows and the ports they need.
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// Repository is the storage the use cases need. Every method takes the
// workspace and the user, which callers read from the Principal. Methods
// return an error wrapping app.ErrNotFound for a missing row.
type Repository interface {
	// Insert stores n and reports whether the row was new. A duplicate
	// (UserID, Key) with a non-empty key is not an error: inserted is false
	// (NOTI-02).
	Insert(ctx context.Context, n domain.Notification) (domain.Notification, bool, error)
	// List returns up to limit notifications of one user in one workspace,
	// newest first. beforeID pages back from that notification's position;
	// empty means the first page. An ID that does not belong to the user in
	// this workspace is app.ErrNotFound.
	List(ctx context.Context, workspaceID, userID string, unreadOnly bool, beforeID string, limit int) ([]domain.Notification, error)
	// UnreadCount counts the user's unread notifications in one workspace.
	UnreadCount(ctx context.Context, workspaceID, userID string) (int64, error)
	// MarkRead marks one notification read. changed is false when it was
	// already read; another user's row or a missing one is app.ErrNotFound.
	MarkRead(ctx context.Context, workspaceID, userID, id string, at time.Time) (changed bool, err error)
	// MarkAllRead marks every unread notification of the user read and
	// returns the IDs that changed.
	MarkAllRead(ctx context.Context, workspaceID, userID string, at time.Time) ([]string, error)
	// Prune deletes read notifications created before the cutoff and returns
	// how many rows went away.
	Prune(ctx context.Context, before time.Time) (int64, error)
	// Preferences returns the saved settings of the given users in one
	// workspace, keyed by user ID. A user with no saved settings is absent.
	Preferences(ctx context.Context, workspaceID string, userIDs []string) (map[string]domain.Preferences, error)
	// SavePreferences stores p as the whole of the user's settings in its
	// workspace, replacing any earlier row (NOTI-07).
	SavePreferences(ctx context.Context, p domain.Preferences) error
}

// Directory tells the time zone of each user, for the do-not-disturb window.
// A user it does not list is in UTC. The server adapts auth.Directory.
type Directory interface {
	Timezones(ctx context.Context, userIDs []string) (map[string]string, error)
}

// Events delivers notifications to a user's own devices after the rows
// committed. Delivery is best effort: the database is the source of truth
// and clients refetch after reconnecting (NOTI-03).
type Events interface {
	// Created tells one user that one notification exists, with the unread
	// count read after the batch committed. quiet is true inside the user's
	// do-not-disturb window: the client shows no toast (NOTI-07).
	Created(ctx context.Context, n domain.Notification, unreadCount int64, quiet bool)
	// Read tells one user that notifications were marked read, so their
	// other devices clear the bell. all is true for "mark all as read".
	Read(ctx context.Context, workspaceID, userID string, ids []string, all bool, unreadCount int64)
}
