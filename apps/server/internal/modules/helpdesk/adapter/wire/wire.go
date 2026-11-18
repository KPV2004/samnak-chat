// Package wire holds the JSON shapes of the helpdesk REST API. Type names
// start with "Helpdesk" because Huma names schemas after their Go types and
// those names are shared by every module's operations.
package wire

import (
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// HelpdeskCategory is a category of a queue (HD-02).
type HelpdeskCategory struct {
	ID        string `json:"id"`
	QueueID   string `json:"queue_id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default" doc:"The category new tickets from chat take; one per workspace"`
}

// HelpdeskQueue is a queue with its agents (HD-01). Categories are listed by
// GET /helpdesk/queues; the other queue responses leave them empty.
type HelpdeskQueue struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Prefix     string             `json:"prefix" doc:"The ticket key prefix, e.g. IT for IT-0142"`
	ChannelID  *string            `json:"channel_id" doc:"The agents' channel; null when the queue has none"`
	AgentIDs   []string           `json:"agent_ids" nullable:"false"`
	Categories []HelpdeskCategory `json:"categories" nullable:"false"`
}

// ToQueue converts a stored queue with its active categories.
func ToQueue(q domain.Queue, cats []domain.Category) HelpdeskQueue {
	out := HelpdeskQueue{
		ID: q.ID, Name: q.Name, Prefix: q.Prefix, AgentIDs: append([]string{}, q.AgentIDs...),
		Categories: make([]HelpdeskCategory, 0, len(cats)),
	}
	if q.ChannelID != "" {
		id := q.ChannelID
		out.ChannelID = &id
	}
	for _, c := range cats {
		out.Categories = append(out.Categories, ToCategory(c))
	}
	return out
}

// ToQueues converts the queue summaries of GET /helpdesk/queues.
func ToQueues(list []usecase.QueueSummary) []HelpdeskQueue {
	out := make([]HelpdeskQueue, len(list))
	for i, s := range list {
		out[i] = ToQueue(s.Queue, s.Categories)
	}
	return out
}

// ToCategory converts a stored category.
func ToCategory(c domain.Category) HelpdeskCategory {
	return HelpdeskCategory{ID: c.ID, QueueID: c.QueueID, Name: c.Name, IsDefault: c.IsDefault}
}

// HelpdeskTicket is a ticket as its requester and the queue see it (HD-03).
type HelpdeskTicket struct {
	ID              string     `json:"id"`
	Key             string     `json:"key" doc:"The queue prefix and the number, e.g. IT-0142"`
	Number          int64      `json:"number"`
	QueueID         string     `json:"queue_id"`
	CategoryID      string     `json:"category_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	RequesterID     string     `json:"requester_id"`
	AssigneeID      *string    `json:"assignee_id" doc:"null until an agent claims or is assigned the ticket"`
	Status          string     `json:"status" enum:"new,open,in_progress,waiting,resolved,closed"`
	Priority        string     `json:"priority" enum:"low,normal,high,urgent"`
	SourceChannelID *string    `json:"source_channel_id" doc:"The channel of the message the ticket was made from"`
	SourceMessageID *string    `json:"source_message_id"`
	MessageID       *string    `json:"message_id" doc:"The card in the requester's channel; null until it is posted"`
	ResolvedAt      *time.Time `json:"resolved_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ToTicket converts a stored ticket.
func ToTicket(t domain.Ticket) HelpdeskTicket {
	out := HelpdeskTicket{
		ID: t.ID, Key: t.Key, Number: t.Number, QueueID: t.QueueID, CategoryID: t.CategoryID, Title: t.Title,
		Description: t.Description, RequesterID: t.RequesterID, Status: string(t.Status), Priority: string(t.Priority),
		ResolvedAt: utcPtr(t.ResolvedAt), ClosedAt: utcPtr(t.ClosedAt), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	out.AssigneeID = optString(t.AssigneeID)
	out.SourceChannelID = optString(t.SourceChannelID)
	out.SourceMessageID = optString(t.SourceMessageID)
	out.MessageID = optString(t.MessageID)
	return out
}

// optString returns nil for the empty string, which means none.
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// HelpdeskHistoryEntry is one line of a ticket's history (HD-18).
type HelpdeskHistoryEntry struct {
	ID       string    `json:"id"`
	ActorID  string    `json:"actor_id"`
	Action   string    `json:"action" enum:"created,assigned,status_changed,priority_changed,category_changed,commented"`
	From     string    `json:"from" doc:"The value before the change; empty when there was none"`
	To       string    `json:"to" doc:"The value after the change"`
	Internal bool      `json:"internal" doc:"Only agents and admins see internal lines"`
	At       time.Time `json:"at"`
}

// HelpdeskComment is a comment on a ticket (HD-07).
type HelpdeskComment struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	Internal  bool      `json:"internal" doc:"Only agents and admins see internal comments"`
	CreatedAt time.Time `json:"created_at"`
}

// ToComment converts a stored comment.
func ToComment(c domain.Comment) HelpdeskComment {
	return HelpdeskComment{
		ID: c.ID, TicketID: c.TicketID, AuthorID: c.AuthorID, Body: c.Body, Internal: c.Internal, CreatedAt: c.CreatedAt.UTC(),
	}
}

// HelpdeskTicketDetail is a ticket with its comments and history, for the
// ticket's page.
type HelpdeskTicketDetail struct {
	HelpdeskTicket
	Comments []HelpdeskComment      `json:"comments" nullable:"false" doc:"Oldest first; internal ones only for agents and admins"`
	History  []HelpdeskHistoryEntry `json:"history" nullable:"false" doc:"Oldest first; internal lines only for agents and admins"`
}

// ToDetail converts a ticket with its comments and history.
func ToDetail(d usecase.Detail) HelpdeskTicketDetail {
	out := HelpdeskTicketDetail{
		HelpdeskTicket: ToTicket(d.Ticket),
		Comments:       make([]HelpdeskComment, len(d.Comments)),
		History:        make([]HelpdeskHistoryEntry, len(d.History)),
	}
	for i, c := range d.Comments {
		out.Comments[i] = ToComment(c)
	}
	for i, h := range d.History {
		out.History[i] = HelpdeskHistoryEntry{
			ID: h.ID, ActorID: h.ActorID, Action: string(h.Action), From: h.From, To: h.To, Internal: h.Internal, At: h.At.UTC(),
		}
	}
	return out
}

// ToTickets converts a page of tickets.
func ToTickets(ts []domain.Ticket) []HelpdeskTicket {
	out := make([]HelpdeskTicket, len(ts))
	for i, t := range ts {
		out[i] = ToTicket(t)
	}
	return out
}
