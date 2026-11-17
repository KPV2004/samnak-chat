// Package usecase holds the approval flows and the ports they need.
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// Repository is the storage the use cases need. Every method takes the
// workspace, which callers read from the Principal or from an event. Methods
// return an error wrapping app.ErrNotFound for a missing row.
type Repository interface {
	// InsertRequest stores a new request with its one step and its "created"
	// history line.
	InsertRequest(ctx context.Context, r domain.Request, step domain.Step, h domain.HistoryEntry) error
	Request(ctx context.Context, workspaceID, id string) (domain.Request, error)
	// List returns up to f.Limit requests, newest first.
	List(ctx context.Context, f ListFilter) ([]domain.Request, error)
	// Decide moves a request from pending to "to" and its step with it. It
	// reports changed=false, and changes nothing, when the request is not
	// pending, so two decisions at once cannot both win (APR-04).
	Decide(ctx context.Context, workspaceID, id string, to domain.Status, reason string, at time.Time) (r domain.Request, changed bool, err error)
	InsertHistory(ctx context.Context, h domain.HistoryEntry) error
	// History returns a request's lines, oldest first.
	History(ctx context.Context, requestID string) ([]domain.HistoryEntry, error)
	// SetCard stores where the card was posted, only when no card is stored
	// yet. changed=false means a card already exists.
	SetCard(ctx context.Context, workspaceID, id, channelID, messageID string) (changed bool, err error)

	InsertAttachment(ctx context.Context, a domain.Attachment) error
	Attachment(ctx context.Context, workspaceID, id string) (domain.Attachment, error)
	AttachmentsByIDs(ctx context.Context, workspaceID string, ids []string) ([]domain.Attachment, error)
	// LinkAttachments gives the uploader's unused files to a request and
	// returns how many it linked.
	LinkAttachments(ctx context.Context, workspaceID, requestID, uploaderID string, ids []string) (int64, error)
	// AttachmentsOf returns a request's files in upload order.
	AttachmentsOf(ctx context.Context, requestID string) ([]domain.Attachment, error)
}

// ListFilter selects requests for APR-06. Role is "requester", "approver" or
// empty for both. BeforeID pages back from that request's position.
type ListFilter struct {
	WorkspaceID string
	UserID      string
	Role        string
	Statuses    []domain.Status
	BeforeID    string
	Limit       int
}

// Members tells whether a user is an active member of a workspace.
type Members interface {
	Active(ctx context.Context, workspaceID, userID string) bool
}

// Managers returns the group managers of a user, the default approvers
// (APR-02). It returns nothing when groups are not available.
type Managers interface {
	ManagersOf(ctx context.Context, workspaceID, userID string) ([]string, error)
}

// Names looks up display names. Unknown IDs and a missing directory give an
// empty map entry.
type Names interface {
	Names(ctx context.Context, ids []string) map[string]string
}

// Cards is the chat card registry, narrowed to what approval does. Both
// methods are used from event handlers, after the commit, never inside a
// transaction.
type Cards interface {
	// Post sends the card as the requester into their DM with the approver
	// and returns the channel and the message.
	Post(ctx context.Context, workspaceID, requesterID, approverID string, payload any, fallback string) (channelID, messageID string, err error)
	Update(ctx context.Context, workspaceID, messageID string, payload any) error
}
