package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// Notification kinds the helpdesk module produces. The web renders them from
// the notification's data.
const (
	KindTicketCreated  = "helpdesk.ticket_created"
	KindTicketAssigned = "helpdesk.ticket_assigned"
	KindStatusChanged  = "helpdesk.status_changed"
	KindComment        = "helpdesk.comment"
)

// Notify turns a helpdesk event into the notifications of HD-20: a new ticket
// tells the queue's agents, an assignment tells the assignee, a status change
// to waiting or resolved tells the requester, and a comment tells the other
// side. The stored ticket is the source of every message, so a stale or
// repeated event can be answered from it; the key carries the ticket's
// updated_at or the comment's ID, so a redelivery adds nothing (PLT-10).
func (s *Service) Notify(ctx context.Context, evt app.Event) ([]app.Notification, error) {
	switch e := evt.(type) {
	case domain.TicketCreated:
		return s.notifyCreated(ctx, e)
	case domain.TicketAssigned:
		return s.notifyAssigned(ctx, e)
	case domain.StatusChanged:
		return s.notifyStatus(ctx, e)
	case domain.CommentAdded:
		return s.notifyComment(ctx, e)
	default:
		return nil, nil
	}
}

// notifyCreated tells the agents of the queue; the requester is left out even
// when they are an agent, because it is their own request.
func (s *Service) notifyCreated(ctx context.Context, e domain.TicketCreated) ([]app.Notification, error) {
	t, ok, err := s.notifyTicket(ctx, e.WorkspaceID, e.TicketID)
	if err != nil || !ok {
		return nil, err
	}
	q, err := s.d.Repo.Queue(ctx, e.WorkspaceID, e.QueueID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("helpdesk: notification queue: %w", err)
	}
	title := "New ticket " + t.Key
	if name := s.names(ctx, e.RequesterID)[e.RequesterID]; name != "" {
		title = name + " opened ticket " + t.Key
	}
	key := fmt.Sprintf("helpdesk:%s:created:%s:", t.ID, stamp(t.UpdatedAt))
	out := make([]app.Notification, 0, len(q.AgentIDs))
	for _, uid := range q.AgentIDs {
		if uid == "" || uid == t.RequesterID {
			continue
		}
		out = append(out, s.ticketNotice(t, uid, KindTicketCreated, title, key+uid))
	}
	return out, nil
}

// notifyAssigned tells the assignee, unless they claimed the ticket
// themselves.
func (s *Service) notifyAssigned(ctx context.Context, e domain.TicketAssigned) ([]app.Notification, error) {
	if e.AssigneeID == "" || e.AssigneeID == e.ActorID {
		return nil, nil
	}
	t, ok, err := s.notifyTicket(ctx, e.WorkspaceID, e.TicketID)
	if err != nil || !ok {
		return nil, err
	}
	title := "You were assigned ticket " + t.Key
	if name := s.names(ctx, e.ActorID)[e.ActorID]; name != "" {
		title = name + " assigned you ticket " + t.Key
	}
	return []app.Notification{{
		WorkspaceID: t.WorkspaceID, UserID: e.AssigneeID, Kind: KindTicketAssigned, Title: title, Body: t.Title,
		Link: "/helpdesk/" + t.ID, Data: s.notifyData(t),
		Key: fmt.Sprintf("helpdesk:%s:assigned:%s:%s", t.ID, stamp(t.UpdatedAt), e.AssigneeID),
	}}, nil
}

// notifyStatus tells the requester when their ticket is waiting for them or
// has been resolved. Other statuses tell nobody.
func (s *Service) notifyStatus(ctx context.Context, e domain.StatusChanged) ([]app.Notification, error) {
	if e.To != string(domain.StatusWaiting) && e.To != string(domain.StatusResolved) {
		return nil, nil
	}
	t, ok, err := s.notifyTicket(ctx, e.WorkspaceID, e.TicketID)
	if err != nil || !ok {
		return nil, err
	}
	if t.RequesterID == "" || t.RequesterID == e.ActorID {
		return nil, nil
	}
	title := "Your ticket " + t.Key + " is waiting for you"
	if e.To == string(domain.StatusResolved) {
		title = "Your ticket " + t.Key + " was resolved"
	}
	return []app.Notification{{
		WorkspaceID: t.WorkspaceID, UserID: t.RequesterID, Kind: KindStatusChanged, Title: title, Body: t.Title,
		Link: "/helpdesk/" + t.ID, Data: s.notifyData(t),
		Key: fmt.Sprintf("helpdesk:%s:status_changed:%s:%s", t.ID, stamp(t.UpdatedAt), t.RequesterID),
	}}, nil
}

// notifyComment tells the other side: a public comment goes to the requester
// and the assignee, an internal one to the assignee only, and never to its
// author.
func (s *Service) notifyComment(ctx context.Context, e domain.CommentAdded) ([]app.Notification, error) {
	t, ok, err := s.notifyTicket(ctx, e.WorkspaceID, e.TicketID)
	if err != nil || !ok {
		return nil, err
	}
	to := []string{ t.RequesterID }
	if e.Internal {
		to = []string{ t.AssigneeID }
	} else {
		to = append(to, t.AssigneeID)
	}
	title := "New comment on " + t.Key
	if e.Internal {
		title = "New internal note on " + t.Key
	}
	out := make([]app.Notification, 0, len(to))
	seen := map[string]bool{e.AuthorID: true}
	for _, uid := range to {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		n := s.ticketNotice(t, uid, KindComment, title, "")
		n.Key = fmt.Sprintf("helpdesk:%s:comment:%s:%s", t.ID, e.CommentID, uid)
		n.Data["internal"] = e.Internal
		out = append(out, n)
	}
	return out, nil
}

// notifyTicket loads the stored ticket a notification describes. A ticket that
// is gone by the time the event is handled tells nobody.
func (s *Service) notifyTicket(ctx context.Context, workspaceID, ticketID string) (domain.Ticket, bool, error) {
	t, err := s.d.Repo.Ticket(ctx, workspaceID, ticketID)
	if err != nil {
		if isNotFound(err) {
			return domain.Ticket{}, false, nil
		}
		return domain.Ticket{}, false, fmt.Errorf("helpdesk: notification ticket: %w", err)
	}
	return t, true, nil
}

// ticketNotice is the notification of one ticket event for one user.
func (s *Service) ticketNotice(t domain.Ticket, userID, kind, title, key string) app.Notification {
	return app.Notification{
		WorkspaceID: t.WorkspaceID, UserID: userID, Kind: kind, Title: title, Body: t.Title,
		Link: "/helpdesk/" + t.ID, Data: s.notifyData(t), Key: key,
	}
}

// stamp renders a time for a notification key: stable and distinct per change.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// notifyData is what the web needs to draw a ticket notification.
func (s *Service) notifyData(t domain.Ticket) map[string]any {
	return map[string]any{
		"ticket_id": t.ID, "key": t.Key, "queue_id": t.QueueID,
		"status": string(t.Status), "title": t.Title,
	}
}
