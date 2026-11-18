package domain

import (
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// ReopenWindow is how long after resolution the requester may reopen a ticket
// (HD-05).
const ReopenWindow = 7 * 24 * time.Hour

// who names the people allowed to make a status move (HD-05).
type who int

const (
	// whoHandler is the assignee or an admin.
	whoHandler who = iota
	// whoRequesterOrAdmin is the requester or an admin.
	whoRequesterOrAdmin
	// whoRequester is the requester only.
	whoRequester
)

// move is one row of the status table: a move made through the status
// endpoint, who may make it, and whether it is a reopen with a time limit.
type move struct {
	from, to Status
	who      who
	reopen   bool
}

// moves is the table of HD-05. new -> open is not here: a ticket is opened by
// being claimed or assigned (CheckAssign), never by the status endpoint.
var moves = []move{
	{StatusOpen, StatusInProgress, whoHandler, false},
	{StatusInProgress, StatusWaiting, whoHandler, false},
	{StatusWaiting, StatusInProgress, whoHandler, false},
	{StatusInProgress, StatusResolved, whoHandler, false},
	{StatusResolved, StatusClosed, whoRequesterOrAdmin, false},
	{StatusResolved, StatusOpen, whoRequester, true},
}

// CheckStatus says whether the actor may move the ticket to status to at now.
// A move that is not in the table is a conflict whoever asks (409). A move in
// the table asked by the wrong person is forbidden (403). A reopen after the
// window is a conflict (409).
func (t Ticket) CheckStatus(a Actor, to Status, now time.Time) error {
	var m *move
	for i := range moves {
		if moves[i].from == t.Status && moves[i].to == to {
			m = &moves[i]
			break
		}
	}
	if m == nil {
		return fmt.Errorf("a ticket cannot go from %s to %s: %w", t.Status, to, app.ErrConflict)
	}
	allowed := false
	switch m.who {
	case whoHandler:
		allowed = t.IsAssignee(a) || a.Admin
	case whoRequesterOrAdmin:
		allowed = t.IsRequester(a) || a.Admin
	case whoRequester:
		allowed = t.IsRequester(a)
	}
	if !allowed {
		return fmt.Errorf("you cannot move this ticket from %s to %s: %w", t.Status, to, app.ErrForbidden)
	}
	if m.reopen && (t.ResolvedAt == nil || now.After(t.ResolvedAt.Add(ReopenWindow))) {
		return fmt.Errorf("a resolved ticket can be reopened only within seven days: %w", app.ErrConflict)
	}
	return nil
}

// Move sets the status and records the time it resolved, closed or reopened.
// The caller has checked the move with CheckStatus.
func (t *Ticket) Move(to Status, now time.Time) {
	now = now.UTC()
	t.Status = to
	switch to {
	case StatusResolved:
		t.ResolvedAt = &now
	case StatusClosed:
		t.ClosedAt = &now
	case StatusOpen:
		// A reopen clears resolved_at; the assignee stays.
		t.ResolvedAt = nil
	}
	t.UpdatedAt = now
}

// CheckAssign says whether the actor may assign the ticket to assigneeID. An
// admin may assign anyone who is an agent of the queue (the caller checks
// that). An agent may only claim: take the ticket for themselves while it is
// unassigned or already theirs. Nobody may assign a closed ticket (409).
func (t Ticket) CheckAssign(a Actor, assigneeID string) error {
	if t.Status == StatusClosed {
		return fmt.Errorf("a closed ticket cannot be assigned: %w", app.ErrConflict)
	}
	if a.Admin {
		return nil
	}
	if !a.Agent || a.UserID == "" {
		return fmt.Errorf("only admins and agents of the queue assign tickets: %w", app.ErrForbidden)
	}
	if assigneeID != a.UserID {
		return fmt.Errorf("an agent can only assign a ticket to themselves: %w", app.ErrForbidden)
	}
	if t.AssigneeID != "" && t.AssigneeID != a.UserID {
		return fmt.Errorf("the ticket is already assigned to someone else: %w", app.ErrForbidden)
	}
	return nil
}

// Assign sets the assignee. A new ticket becomes open, because being assigned
// is what opens it (HD-06). The caller has checked CheckAssign.
func (t *Ticket) Assign(assigneeID string, now time.Time) {
	t.AssigneeID = assigneeID
	if t.Status == StatusNew {
		t.Status = StatusOpen
	}
	t.UpdatedAt = now.UTC()
}

// CheckEdit says whether the actor may change the priority or the category
// (HD-09): agents and admins only.
func (t Ticket) CheckEdit(a Actor) error {
	if a.Admin || a.Agent {
		return nil
	}
	return fmt.Errorf("only agents and admins change a ticket's priority or category: %w", app.ErrForbidden)
}
