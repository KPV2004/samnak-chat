package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

const (
	// DefaultLimit and MaxLimit bound a page of tickets.
	DefaultLimit = 20
	MaxLimit     = 100
)

// Detail is a ticket with its history (HD-18), for the ticket's page.
type Detail struct {
	Ticket  domain.Ticket
	History []domain.HistoryEntry
}

// actor says who the caller is for the rules of one ticket: admin, agent of its
// queue, or neither.
func (s *Service) actor(ctx context.Context, p app.Principal, t domain.Ticket) (domain.Actor, error) {
	a := domain.Actor{UserID: p.UserID, Admin: s.isAdmin(p)}
	if !a.Admin {
		agent, err := s.d.Repo.IsAgent(ctx, p.WorkspaceID, t.QueueID, p.UserID)
		if err != nil {
			return domain.Actor{}, err
		}
		a.Agent = agent
	}
	return a, nil
}

// Get returns one ticket with its history. Only its requester, the agents of
// its queue and the admins see it; everyone else, in any workspace, gets not
// found (HD-10).
func (s *Service) Get(ctx context.Context, p app.Principal, id string) (Detail, error) {
	t, err := s.d.Repo.Ticket(ctx, p.WorkspaceID, id)
	if err != nil {
		return Detail{}, err
	}
	a, err := s.actor(ctx, p, t)
	if err != nil {
		return Detail{}, err
	}
	if !t.Visible(a) {
		return Detail{}, fmt.Errorf("ticket: %w", app.ErrNotFound)
	}
	hist, err := s.d.Repo.History(ctx, t.ID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Ticket: t, History: hist}, nil
}

// ListQuery filters the tickets the caller may see (HD-10). Status is a
// comma-separated list. Assignee is "me", "none" or a user ID; Requester is
// "me". Cursor is the next_cursor of the previous page.
type ListQuery struct {
	QueueID   string
	Status    string
	Assignee  string
	Requester string
	Cursor    string
	Limit     int
}

// Page is one page of tickets, newest first. NextCursor is empty on the last
// page.
type Page struct {
	Items      []domain.Ticket
	NextCursor string
}

// List returns one page of the tickets the caller may see.
func (s *Service) List(ctx context.Context, p app.Principal, q ListQuery) (Page, error) {
	f := TicketFilter{WorkspaceID: p.WorkspaceID, UserID: p.UserID, Admin: s.isAdmin(p)}
	if q.QueueID != "" {
		if !validID(q.QueueID) {
			return Page{}, invalid("queue_id is not a valid ID")
		}
		f.QueueID = q.QueueID
	}
	if q.Status != "" {
		for _, raw := range strings.Split(q.Status, ",") {
			st := domain.Status(strings.TrimSpace(raw))
			if !st.Valid() {
				return Page{}, invalid("status must be one of new, open, in_progress, waiting, resolved, closed")
			}
			f.Statuses = append(f.Statuses, st)
		}
	}
	switch {
	case q.Assignee == "":
	case q.Assignee == "me":
		f.AssigneeID = p.UserID
	case q.Assignee == "none":
		f.NoAssignee = true
	case validID(q.Assignee):
		f.AssigneeID = q.Assignee
	default:
		return Page{}, invalid("assignee must be me, none or a user ID")
	}
	switch q.Requester {
	case "":
	case "me":
		f.RequesterID = p.UserID
	default:
		return Page{}, invalid("requester must be me")
	}
	if q.Cursor != "" {
		if !validID(q.Cursor) {
			return Page{}, invalid("cursor is not valid")
		}
		f.BeforeID = q.Cursor
	}
	limit := q.Limit
	switch {
	case limit < 0:
		return Page{}, invalid("limit must not be negative")
	case limit == 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	f.Limit = limit + 1

	rows, err := s.d.Repo.ListTickets(ctx, f)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		page.NextCursor = rows[limit-1].ID
	}
	return page, nil
}
