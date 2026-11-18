package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Comment is a message on a ticket (HD-07). Internal comments are for agents
// and admins; the requester never sees them.
type Comment struct {
	ID        string
	TicketID  string
	AuthorID  string
	Body      string
	Internal  bool
	CreatedAt time.Time
}

// NewCommentInput is what an author writes. Internal is decided by the caller
// and checked by CheckComment.
type NewCommentInput struct {
	TicketID string
	AuthorID string
	Body     string
	Internal bool
}

// NewComment builds a comment with a checked body. The caller supplies the ID
// and the time.
func NewComment(in NewCommentInput, id string, now time.Time) (Comment, error) {
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return Comment{}, invalid("body is required")
	}
	if utf8.RuneCountInString(body) > MaxCommentRunes {
		return Comment{}, invalid("body is longer than %d characters", MaxCommentRunes)
	}
	return Comment{
		ID: id, TicketID: in.TicketID, AuthorID: in.AuthorID, Body: body, Internal: in.Internal, CreatedAt: now.UTC(),
	}, nil
}

// CheckComment says whether the actor may write a comment of the given kind on
// the ticket. Anyone who may see the ticket may write a public comment; only
// agents and admins may write an internal one (403 otherwise).
func (t Ticket) CheckComment(a Actor, internal bool) error {
	if internal && !a.SeesInternal() {
		return fmt.Errorf("only agents and admins write internal comments: %w", app.ErrForbidden)
	}
	return nil
}

// SeesInternal reports whether the actor may read internal comments and history
// lines: agents and admins.
func (a Actor) SeesInternal() bool { return a.Admin || a.Agent }
