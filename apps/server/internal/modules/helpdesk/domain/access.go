package domain

// Actor is who acts on a ticket, as the rules see it. Admin is the workspace
// action helpdesk.admin; Agent is membership of the ticket's queue. Both are
// answered by the use case, so the rules here stay free of storage.
type Actor struct {
	UserID string
	Admin  bool
	Agent  bool
}

// Visible reports whether the actor may see the ticket: its requester, the
// agents of its queue and the admins. Everyone else gets not found (HD-10).
func (t Ticket) Visible(a Actor) bool {
	return a.UserID != "" && (a.Admin || a.Agent || a.UserID == t.RequesterID)
}

// IsRequester reports whether the actor opened the ticket.
func (t Ticket) IsRequester(a Actor) bool {
	return a.UserID != "" && a.UserID == t.RequesterID
}

// IsAssignee reports whether the actor is the ticket's assignee.
func (t Ticket) IsAssignee(a Actor) bool {
	return a.UserID != "" && a.UserID == t.AssigneeID
}
