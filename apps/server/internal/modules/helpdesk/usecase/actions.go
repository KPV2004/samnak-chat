package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// EditInput changes the priority or the category of a ticket. A nil pointer
// leaves the field alone.
type EditInput struct {
	Priority   *domain.Priority
	CategoryID *string
}

// lockVisible locks the ticket for the rest of the transaction and returns it
// with the caller's actor. A ticket the caller may not see is not found.
func (s *Service) lockVisible(ctx context.Context, p app.Principal, id string) (domain.Ticket, domain.Actor, error) {
	t, err := s.d.Repo.LockTicket(ctx, p.WorkspaceID, id)
	if err != nil {
		return domain.Ticket{}, domain.Actor{}, err
	}
	a, err := s.actor(ctx, p, t)
	if err != nil {
		return domain.Ticket{}, domain.Actor{}, err
	}
	if !t.Visible(a) {
		return domain.Ticket{}, domain.Actor{}, fmt.Errorf("ticket: %w", app.ErrNotFound)
	}
	return t, a, nil
}

// record adds a line to the ticket's history (HD-18).
func (s *Service) record(ctx context.Context, t domain.Ticket, actorID string, action domain.Action, from, to string, internal bool, at time.Time) error {
	return s.d.Repo.InsertHistory(ctx, domain.HistoryEntry{
		ID: newID(), TicketID: t.ID, ActorID: actorID, Action: action, From: from, To: to, Internal: internal, At: at,
	})
}

// Assign assigns the ticket to assigneeID (HD-06). An agent claims a ticket
// for themselves; an admin assigns an agent of the queue. A new ticket becomes
// open. Assigning the current assignee again changes nothing.
func (s *Service) Assign(ctx context.Context, p app.Principal, id, assigneeID string) (domain.Ticket, error) {
	if !validID(assigneeID) {
		return domain.Ticket{}, invalid("assignee_id is not a valid ID")
	}
	var out domain.Ticket
	err := s.d.Tx.Do(ctx, func(ctx context.Context) error {
		t, a, err := s.lockVisible(ctx, p, id)
		if err != nil {
			return err
		}
		if err := t.CheckAssign(a, assigneeID); err != nil {
			return err
		}
		if t.AssigneeID == assigneeID {
			out = t
			return nil
		}
		agent, err := s.d.Repo.IsAgent(ctx, p.WorkspaceID, t.QueueID, assigneeID)
		if err != nil {
			return err
		}
		if !agent {
			return invalid("assignee is not an agent of the queue")
		}

		now := s.clock()
		prev, from := t.AssigneeID, t.Status
		t.Assign(assigneeID, now)
		if err := s.d.Repo.UpdateTicket(ctx, t); err != nil {
			return err
		}
		if err := s.record(ctx, t, p.UserID, domain.ActionAssigned, prev, assigneeID, false, now); err != nil {
			return err
		}
		if err := s.publish(ctx, domain.TicketAssigned{TicketRef: ref(t), AssigneeID: assigneeID, ActorID: p.UserID}); err != nil {
			return err
		}
		if t.Status != from {
			if err := s.recordStatus(ctx, t, p.UserID, from, now); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	if err != nil {
		return domain.Ticket{}, err
	}
	return out, nil
}

// recordStatus records and publishes that the status of t changed from from.
func (s *Service) recordStatus(ctx context.Context, t domain.Ticket, actorID string, from domain.Status, at time.Time) error {
	if err := s.record(ctx, t, actorID, domain.ActionStatusChanged, string(from), string(t.Status), false, at); err != nil {
		return err
	}
	return s.publish(ctx, domain.StatusChanged{TicketRef: ref(t), From: string(from), To: string(t.Status), ActorID: actorID})
}

// ChangeStatus moves the ticket to status to, by the table of HD-05. Moving to
// resolved also publishes helpdesk.ticket_resolved.
func (s *Service) ChangeStatus(ctx context.Context, p app.Principal, id string, to domain.Status) (domain.Ticket, error) {
	if !to.Valid() {
		return domain.Ticket{}, invalid("status must be one of new, open, in_progress, waiting, resolved, closed")
	}
	var out domain.Ticket
	err := s.d.Tx.Do(ctx, func(ctx context.Context) error {
		t, a, err := s.lockVisible(ctx, p, id)
		if err != nil {
			return err
		}
		now := s.clock()
		if err := t.CheckStatus(a, to, now); err != nil {
			return err
		}
		from := t.Status
		t.Move(to, now)
		if err := s.d.Repo.UpdateTicket(ctx, t); err != nil {
			return err
		}
		if err := s.recordStatus(ctx, t, p.UserID, from, now); err != nil {
			return err
		}
		if to == domain.StatusResolved {
			if err := s.publish(ctx, domain.TicketResolved{TicketRef: ref(t), ActorID: p.UserID}); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	if err != nil {
		return domain.Ticket{}, err
	}
	return out, nil
}

// Edit changes the priority or the category of a ticket (HD-09, HD-02). Agents
// and admins only. A category in another queue moves the ticket there and keeps
// its number (the key takes the new prefix). An assignee who is not an agent of
// that queue is dropped, so the ticket is waiting for someone who can see it.
func (s *Service) Edit(ctx context.Context, p app.Principal, id string, in EditInput) (domain.Ticket, error) {
	if in.Priority != nil && !in.Priority.Valid() {
		return domain.Ticket{}, invalid("priority must be low, normal, high or urgent")
	}
	if in.CategoryID != nil && !validID(*in.CategoryID) {
		return domain.Ticket{}, invalid("category_id is not a valid ID")
	}
	var out domain.Ticket
	err := s.d.Tx.Do(ctx, func(ctx context.Context) error {
		t, a, err := s.lockVisible(ctx, p, id)
		if err != nil {
			return err
		}
		if err := t.CheckEdit(a); err != nil {
			return err
		}
		now := s.clock()
		changed := false
		if in.Priority != nil && *in.Priority != t.Priority {
			from := t.Priority
			t.Priority, t.UpdatedAt = *in.Priority, now
			changed = true
			if err := s.record(ctx, t, p.UserID, domain.ActionPriorityChanged, string(from), string(t.Priority), false, now); err != nil {
				return err
			}
		}
		if in.CategoryID != nil && *in.CategoryID != t.CategoryID {
			moved, err := s.moveToCategory(ctx, p, &t, *in.CategoryID, now)
			if err != nil {
				return err
			}
			changed = changed || moved
		}
		if !changed {
			out = t
			return nil
		}
		if err := s.d.Repo.UpdateTicket(ctx, t); err != nil {
			return err
		}
		if err := s.publish(ctx, domain.TicketUpdated{TicketRef: ref(t), ActorID: p.UserID}); err != nil {
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		return domain.Ticket{}, err
	}
	return out, nil
}

// moveToCategory sets the category of t, and its queue when the category is in
// another one. It records the change and any assignee dropped on the way.
func (s *Service) moveToCategory(ctx context.Context, p app.Principal, t *domain.Ticket, categoryID string, now time.Time) (bool, error) {
	cat, err := s.d.Repo.Category(ctx, p.WorkspaceID, categoryID)
	if err != nil {
		if isNotFound(err) {
			return false, invalid("category_id is not a category of this workspace")
		}
		return false, err
	}
	if cat.ArchivedAt != nil {
		return false, invalid("category is archived")
	}
	if cat.QueueID != t.QueueID {
		q, err := s.d.Repo.Queue(ctx, p.WorkspaceID, cat.QueueID)
		if err != nil {
			return false, err
		}
		if q.ArchivedAt != nil {
			return false, invalid("queue %s is archived", q.Prefix)
		}
		if t.AssigneeID != "" {
			agent, err := s.d.Repo.IsAgent(ctx, p.WorkspaceID, q.ID, t.AssigneeID)
			if err != nil {
				return false, err
			}
			if !agent {
				prev := t.AssigneeID
				t.AssigneeID = ""
				if err := s.record(ctx, *t, p.UserID, domain.ActionAssigned, prev, "", false, now); err != nil {
					return false, err
				}
			}
		}
		t.QueueID, t.Key = q.ID, domain.TicketKey(q.Prefix, t.Number)
	}
	from := t.CategoryID
	t.CategoryID, t.UpdatedAt = cat.ID, now
	if err := s.record(ctx, *t, p.UserID, domain.ActionCategoryChanged, from, cat.ID, false, now); err != nil {
		return false, err
	}
	return true, nil
}
