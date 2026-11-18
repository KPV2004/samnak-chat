// Package usecase holds the helpdesk flows and the ports they need: queues and
// categories (HD-01, HD-02), tickets (HD-03..09), comments and history (HD-07,
// HD-18) and attachments (HD-03).
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// ActionAdmin is the workspace action that lets a user manage queues,
// categories and agents, and see every ticket (HD-01). The module registers it
// for owners and admins.
const ActionAdmin = "helpdesk.admin"

// Repository is the storage the use cases need. Every method takes the
// workspace, which callers read from the Principal or from the ticket they
// already loaded. A missing row is an error wrapping app.ErrNotFound.
type Repository interface {
	InsertQueue(ctx context.Context, q domain.Queue) error
	// UpdateQueue writes name, channel and archived_at.
	UpdateQueue(ctx context.Context, q domain.Queue) error
	// Queue returns one queue with its agents.
	Queue(ctx context.Context, workspaceID, id string) (domain.Queue, error)
	// Queues returns the workspace's queues with their agents, by name.
	Queues(ctx context.Context, workspaceID string, includeArchived bool) ([]domain.Queue, error)
	// SetAgents replaces the agents of a queue.
	SetAgents(ctx context.Context, workspaceID, queueID string, userIDs []string, at time.Time) error

	InsertCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	Category(ctx context.Context, workspaceID, id string) (domain.Category, error)
	// Categories returns the workspace's categories, by name.
	Categories(ctx context.Context, workspaceID string, includeArchived bool) ([]domain.Category, error)
	// ClearDefault unsets the default flag of every category in the workspace
	// except exceptID, so at most one default is left.
	ClearDefault(ctx context.Context, workspaceID, exceptID string) error
}

// Members tells whether a user is an active member of a workspace.
type Members interface {
	Active(ctx context.Context, workspaceID, userID string) bool
}

// Permissions is the workspace authorizer, narrowed to what helpdesk asks.
type Permissions interface {
	Can(p app.Principal, action string) bool
}
