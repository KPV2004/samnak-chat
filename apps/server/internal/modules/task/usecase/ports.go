// Package usecase holds the task flows and the ports they need.
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// Filter narrows a task list. Empty fields do not filter.
type Filter struct {
	AssigneeID string
	CreatorID  string
	ChannelID  string
	Statuses   []domain.Status
	// DueFrom (inclusive) and DueBefore (exclusive) bound DueAt; tasks without
	// a due date never match when either is set.
	DueFrom   *time.Time
	DueBefore *time.Time
	// OpenOnly leaves out done tasks.
	OpenOnly bool
}

// Due is a task the due scan claimed.
type Due struct {
	TaskID      string
	WorkspaceID string
	AssigneeID  string
	DueAt       time.Time
}

// Repository is the storage the use cases need. Every method that reads or
// writes one task takes the workspace, which callers read from the Principal.
// A missing row is an error wrapping app.ErrNotFound.
type Repository interface {
	Insert(ctx context.Context, t domain.Task) (domain.Task, error)
	Get(ctx context.Context, workspaceID, id string) (domain.Task, error)
	// GetForUpdate locks the row until the transaction ends, so two edits
	// cannot overwrite each other.
	GetForUpdate(ctx context.Context, workspaceID, id string) (domain.Task, error)
	// Update writes every editable field and the sent marks, and returns the
	// stored row.
	Update(ctx context.Context, t domain.Task) (domain.Task, error)
	// SetMessage stores the card's message ID only when none is stored, and
	// reports whether it did.
	SetMessage(ctx context.Context, workspaceID, id, messageID string) (bool, error)
	// List returns up to limit tasks newest first (by ID, which is a UUIDv7).
	// afterID pages on: only tasks older than that ID. Empty means the first
	// page.
	List(ctx context.Context, workspaceID string, f Filter, afterID string, limit int) ([]domain.Task, error)
	// ClaimDueSoon marks up to batch open, assigned tasks whose due date is
	// after now and not after horizon as announced, and returns them. Rows
	// another transaction holds are skipped.
	ClaimDueSoon(ctx context.Context, now, horizon time.Time, batch int) ([]Due, error)
	// ClaimOverdue does the same for tasks whose due date is not after now.
	ClaimOverdue(ctx context.Context, now time.Time, batch int) ([]Due, error)
}

// Channels answers channel visibility with chat's rules. A missing provider
// answers false for everyone.
type Channels interface {
	CanRead(ctx context.Context, workspaceID, channelID, userID string) (bool, error)
}

// Members answers whether a user is an active member of a workspace.
type Members interface {
	IsActiveMember(ctx context.Context, workspaceID, userID string) (bool, error)
}

// Directory resolves display names. Unknown users are left out, and a missing
// provider answers an empty map.
type Directory interface {
	Names(ctx context.Context, ids []string) map[string]string
}

// Cards posts and updates the task card. It is nil when chat does not provide
// a card registry.
type Cards interface {
	Post(ctx context.Context, c app.Card) (messageID string, err error)
	Update(ctx context.Context, workspaceID, messageID string, payload any) error
}

// Events publishes the task events (TASK-08). The implementation publishes on
// the bus; inside a transaction the event is delivered after the commit.
type Events interface {
	Created(ctx context.Context, t domain.Task) error
	// Assigned is published on create with an assignee and when the assignee
	// changes. t carries the new UpdatedAt.
	Assigned(ctx context.Context, t domain.Task, actorID string) error
	StatusChanged(ctx context.Context, t domain.Task, from domain.Status, actorID string) error
	Updated(ctx context.Context, t domain.Task) error
	DueSoon(ctx context.Context, d Due) error
	Overdue(ctx context.Context, d Due) error
}
