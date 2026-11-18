package helpdesk

import (
	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// Events other modules may listen for (HD-20). Each is published inside the
// transaction that changed the ticket, so it is delivered once the change has
// committed, at least once. Every ticket event carries the ticket's workspace,
// ID, queue, key and requester.
type (
	// TicketCreated is `helpdesk.ticket_created`.
	TicketCreated = domain.TicketCreated
	// TicketAssigned is `helpdesk.ticket_assigned`.
	TicketAssigned = domain.TicketAssigned
	// StatusChanged is `helpdesk.status_changed`.
	StatusChanged = domain.StatusChanged
	// TicketResolved is `helpdesk.ticket_resolved`.
	TicketResolved = domain.TicketResolved
	// TicketUpdated is `helpdesk.ticket_updated`.
	TicketUpdated = domain.TicketUpdated
	// CommentAdded is `helpdesk.comment_added`; internal comments are for agents and admins only.
	CommentAdded = domain.CommentAdded
)

// eventTypes lists every event the module publishes inside a transaction, so
// the outbox can decode them back (PLT-10).
var eventTypes = []app.Event{
	TicketCreated{}, TicketAssigned{}, StatusChanged{}, TicketResolved{}, TicketUpdated{}, CommentAdded{},
}
