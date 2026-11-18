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

	// NextTicketNumber takes the next number of an active queue in one atomic
	// step, so concurrent creates never share a number (HD-04). It returns the
	// number with the queue's prefix. An archived or missing queue is an error
	// wrapping app.ErrNotFound.
	NextTicketNumber(ctx context.Context, workspaceID, queueID string) (prefix string, number int64, err error)
	InsertTicket(ctx context.Context, t domain.Ticket) error
	InsertHistory(ctx context.Context, h domain.HistoryEntry) error
	// Ticket returns one ticket with its key. A missing one is an error
	// wrapping app.ErrNotFound.
	Ticket(ctx context.Context, workspaceID, id string) (domain.Ticket, error)
	// ListTickets returns up to f.Limit tickets the filter selects, newest
	// first (by ID, which is a UUIDv7). BeforeID pages back from that ticket.
	ListTickets(ctx context.Context, f TicketFilter) ([]domain.Ticket, error)
	// IsAgent reports whether userID is an agent of the queue.
	IsAgent(ctx context.Context, workspaceID, queueID, userID string) (bool, error)
	// History returns a ticket's lines, oldest first.
	History(ctx context.Context, ticketID string) ([]domain.HistoryEntry, error)
}

// TicketFilter selects the tickets of a list. The visibility rule (requester,
// agents of the queue, admins) is part of it: Admin and UserID say who asks.
// Empty fields do not filter; NoAssignee selects the unassigned ones.
type TicketFilter struct {
	WorkspaceID string
	UserID      string
	Admin       bool
	QueueID     string
	Statuses    []domain.Status
	AssigneeID  string
	NoAssignee  bool
	RequesterID string
	BeforeID    string
	Limit       int
}

// Messages reads a chat message for a user who can read its channel (HD-03).
// It is app.MessageReader; nil when chat does not provide it.
type Messages interface {
	ReadMessage(ctx context.Context, workspaceID, messageID, userID string) (app.MessageSnapshot, error)
}

// Members tells whether a user is an active member of a workspace.
type Members interface {
	Active(ctx context.Context, workspaceID, userID string) bool
}

// Permissions is the workspace authorizer, narrowed to what helpdesk asks.
type Permissions interface {
	Can(p app.Principal, action string) bool
}
