package domain

// The events of the module (HD-20). Each is published in the transaction that
// changed the ticket, so it is delivered once the change has committed, at
// least once. Every ticket event carries TicketRef, so a listener needs no
// other lookup for the notification rules.

// TicketRef is what every ticket event carries.
type TicketRef struct {
	WorkspaceID string `json:"workspace_id"`
	TicketID    string `json:"ticket_id"`
	QueueID     string `json:"queue_id"`
	Key         string `json:"key"`
	RequesterID string `json:"requester_id"`
}

// TicketCreated is `helpdesk.ticket_created`.
type TicketCreated struct {
	TicketRef
}

func (TicketCreated) EventName() string { return "helpdesk.ticket_created" }

// TicketAssigned is `helpdesk.ticket_assigned`. Claiming a ticket is an
// assignment to the claimer.
type TicketAssigned struct {
	TicketRef
	AssigneeID string `json:"assignee_id"`
	ActorID    string `json:"actor_id"`
}

func (TicketAssigned) EventName() string { return "helpdesk.ticket_assigned" }

// StatusChanged is `helpdesk.status_changed`.
type StatusChanged struct {
	TicketRef
	From    string `json:"from"`
	To      string `json:"to"`
	ActorID string `json:"actor_id"`
}

func (StatusChanged) EventName() string { return "helpdesk.status_changed" }

// TicketResolved is `helpdesk.ticket_resolved`, published with the status
// change to resolved.
type TicketResolved struct {
	TicketRef
	ActorID string `json:"actor_id"`
}

func (TicketResolved) EventName() string { return "helpdesk.ticket_resolved" }

// TicketUpdated is `helpdesk.ticket_updated`: the priority or the category
// changed.
type TicketUpdated struct {
	TicketRef
	ActorID string `json:"actor_id"`
}

func (TicketUpdated) EventName() string { return "helpdesk.ticket_updated" }

// CommentAdded is `helpdesk.comment_added`. An internal comment is only for
// agents and admins; listeners must respect Internal.
type CommentAdded struct {
	TicketRef
	CommentID string `json:"comment_id"`
	AuthorID  string `json:"author_id"`
	Internal  bool   `json:"internal"`
}

func (CommentAdded) EventName() string { return "helpdesk.comment_added" }
