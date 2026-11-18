package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Status is where a ticket stands (HD-05). Only the transitions in the table
// of the status rules change it.
type Status string

const (
	StatusNew        Status = "new"
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusWaiting    Status = "waiting"
	StatusResolved   Status = "resolved"
	StatusClosed     Status = "closed"
)

// Valid reports whether s is one of the six statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusOpen, StatusInProgress, StatusWaiting, StatusResolved, StatusClosed:
		return true
	}
	return false
}

// Priority is how urgent a ticket is (HD-09).
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Valid reports whether p is one of the four priorities.
func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// Limits of the text fields, counted in characters.
const (
	MaxTitleRunes       = 200
	MaxDescriptionRunes = 8000
	MaxCommentRunes     = 8000
)

// Ticket is one request for help (HD-03). The key is the queue's prefix and the
// number, such as IT-0142. Empty IDs mean none: no assignee, no card, no
// source message.
type Ticket struct {
	ID          string
	WorkspaceID string
	QueueID     string
	CategoryID  string
	// Number is unique within the queue (HD-04). Key is derived from it and
	// the queue's prefix, so the repository fills it in when it reads a ticket.
	Number      int64
	Key         string
	Title       string
	Description string
	RequesterID string
	AssigneeID  string
	Status      Status
	Priority    Priority
	// SourceChannelID and SourceMessageID name the chat message the ticket was
	// made from (HD-03).
	SourceChannelID string
	SourceMessageID string
	// MessageID is the card in the requester's channel; QueueMessageID the card
	// in the agents' channel. Both are set once the cards are posted.
	MessageID      string
	QueueMessageID string
	ResolvedAt     *time.Time
	ClosedAt       *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TicketKey is the human key of a ticket: the prefix and the number padded to
// four digits, such as IT-0142 and IT-10000 (HD-04).
func TicketKey(prefix string, number int64) string {
	return fmt.Sprintf("%s-%04d", prefix, number)
}

// NewTicketInput is what a caller chooses when creating a ticket.
type NewTicketInput struct {
	WorkspaceID     string
	QueueID         string
	CategoryID      string
	RequesterID     string
	Title           string
	Description     string
	Priority        Priority // empty means normal
	SourceChannelID string
	SourceMessageID string
}

// NewTicket builds a new, unassigned ticket with a checked title, description
// and priority. The caller supplies the ID and the time, and assigns the
// number when it stores the ticket.
func NewTicket(in NewTicketInput, id string, now time.Time) (Ticket, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Ticket{}, invalid("title is required")
	}
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return Ticket{}, invalid("title is longer than %d characters", MaxTitleRunes)
	}
	if utf8.RuneCountInString(in.Description) > MaxDescriptionRunes {
		return Ticket{}, invalid("description is longer than %d characters", MaxDescriptionRunes)
	}
	pr := in.Priority
	if pr == "" {
		pr = PriorityNormal
	}
	if !pr.Valid() {
		return Ticket{}, invalid("priority must be low, normal, high or urgent")
	}
	now = now.UTC()
	return Ticket{
		ID: id, WorkspaceID: in.WorkspaceID, QueueID: in.QueueID, CategoryID: in.CategoryID,
		Title: title, Description: in.Description, RequesterID: in.RequesterID,
		Status: StatusNew, Priority: pr, SourceChannelID: in.SourceChannelID, SourceMessageID: in.SourceMessageID,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}
