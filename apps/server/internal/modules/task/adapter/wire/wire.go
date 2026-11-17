// Package wire holds the JSON shapes of the task module's REST responses.
package wire

import (
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// Task is one task as a reader of its channel sees it.
type Task struct {
	ID              string     `json:"id"`
	ChannelID       string     `json:"channel_id"`
	MessageID       *string    `json:"message_id" doc:"The task card's message; null until the card is posted"`
	SourceMessageID *string    `json:"source_message_id" doc:"The message the task was created from, if any"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	CreatorID       string     `json:"creator_id"`
	AssigneeID      *string    `json:"assignee_id"`
	DueAt           *time.Time `json:"due_at"`
	Priority        string     `json:"priority" enum:"low,normal,high"`
	Status          string     `json:"status" enum:"todo,doing,done"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ToTask converts a task.
func ToTask(t domain.Task) Task {
	var due *time.Time
	if t.DueAt != nil {
		d := t.DueAt.UTC()
		due = &d
	}
	return Task{
		ID: t.ID, ChannelID: t.ChannelID, MessageID: optional(t.MessageID), SourceMessageID: optional(t.SourceMessageID),
		Title: t.Title, Description: t.Description, CreatorID: t.CreatorID, AssigneeID: optional(t.AssigneeID), DueAt: due,
		Priority: string(t.Priority), Status: string(t.Status), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
}

// ToTasks converts a page.
func ToTasks(ts []domain.Task) []Task {
	out := make([]Task, len(ts))
	for i, t := range ts {
		out[i] = ToTask(t)
	}
	return out
}
