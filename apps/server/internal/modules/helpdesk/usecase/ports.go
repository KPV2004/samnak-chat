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
	// LockTicket is Ticket, and holds the row until the transaction ends, so
	// two changes to one ticket cannot overwrite each other.
	LockTicket(ctx context.Context, workspaceID, id string) (domain.Ticket, error)
	// UpdateTicket writes the changeable fields of a ticket: queue, category,
	// assignee, status, priority, resolved_at, closed_at and updated_at. A
	// queue and number already taken there is a conflict (409).
	UpdateTicket(ctx context.Context, t domain.Ticket) error
	// ListTickets returns up to f.Limit tickets the filter selects, newest
	// first (by ID, which is a UUIDv7). BeforeID pages back from that ticket.
	ListTickets(ctx context.Context, f TicketFilter) ([]domain.Ticket, error)
	// IsAgent reports whether userID is an agent of the queue.
	IsAgent(ctx context.Context, workspaceID, queueID, userID string) (bool, error)
	// History returns a ticket's lines, oldest first.
	History(ctx context.Context, ticketID string) ([]domain.HistoryEntry, error)

	InsertComment(ctx context.Context, c domain.Comment) error
	// Comments returns a ticket's comments, oldest first.
	Comments(ctx context.Context, ticketID string) ([]domain.Comment, error)

	InsertAttachment(ctx context.Context, a domain.Attachment) error
	// Attachment returns one file. A missing one is an error wrapping
	// app.ErrNotFound.
	Attachment(ctx context.Context, workspaceID, id string) (domain.Attachment, error)
	// AttachmentsByIDs returns the files with these IDs in the workspace.
	AttachmentsByIDs(ctx context.Context, workspaceID string, ids []string) ([]domain.Attachment, error)
	// LinkAttachments gives the uploader's files that are not used yet to a
	// ticket and returns how many it linked.
	LinkAttachments(ctx context.Context, workspaceID, ticketID, uploaderID string, ids []string) (int64, error)
	// AttachmentsOf returns a ticket's files in upload order.
	AttachmentsOf(ctx context.Context, ticketID string) ([]domain.Attachment, error)

	// SetRequesterCard stores the message of the card in the requester's
	// channel, only when none is stored yet. changed=false means one is.
	SetRequesterCard(ctx context.Context, workspaceID, ticketID, messageID string) (changed bool, err error)
	// SetQueueCard is SetRequesterCard for the card in the agents' channel.
	SetQueueCard(ctx context.Context, workspaceID, ticketID, messageID string) (changed bool, err error)
	// DefaultCategory returns the workspace's default category, which new
	// tickets from chat use (HD-03). A missing one is an error wrapping
	// app.ErrNotFound.
	DefaultCategory(ctx context.Context, workspaceID string) (domain.Category, error)
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

// Cards is the chat card registry, narrowed to what helpdesk does. Both
// methods run from event handlers, after the commit, never inside a
// transaction (PLT-10).
type Cards interface {
	Post(ctx context.Context, c app.Card) (messageID string, err error)
	Update(ctx context.Context, workspaceID, messageID string, payload any) error
}

// Names looks up display names. Unknown IDs are left out of the map.
type Names interface {
	Names(ctx context.Context, ids []string) map[string]string
}
