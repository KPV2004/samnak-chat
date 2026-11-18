package domain

import "time"

// Action is what a line of a ticket's history records (HD-18).
type Action string

const (
	ActionCreated         Action = "created"
	ActionAssigned        Action = "assigned"
	ActionStatusChanged   Action = "status_changed"
	ActionPriorityChanged Action = "priority_changed"
	ActionCategoryChanged Action = "category_changed"
	ActionCommented       Action = "commented"
)

// HistoryEntry records one change of a ticket: who made it, when, what it was
// and what it became. Internal lines are hidden from the requester, like
// internal comments (HD-07).
type HistoryEntry struct {
	ID       string
	TicketID string
	ActorID  string
	Action   Action
	From     string
	To       string
	Internal bool
	At       time.Time
}
