// Package domain holds the task model and the rules that hold regardless of
// storage (SRS TASK-02, TASK-05).
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type Status string

const (
	StatusTodo  Status = "todo"
	StatusDoing Status = "doing"
	StatusDone  Status = "done"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// Limits of the text fields, counted in characters.
const (
	MaxTitleRunes       = 200
	MaxDescriptionRunes = 4000
)

func (s Status) Valid() bool {
	return s == StatusTodo || s == StatusDoing || s == StatusDone
}

func (p Priority) Valid() bool {
	return p == PriorityLow || p == PriorityNormal || p == PriorityHigh
}

// Task is one work item that lives in a channel. An empty AssigneeID,
// MessageID or SourceMessageID means none.
type Task struct {
	ID          string
	WorkspaceID string
	ChannelID   string
	// SourceMessageID is the message the task was created from, if any.
	SourceMessageID string
	// MessageID is the task card's message, set once the card is posted.
	MessageID   string
	Title       string
	Description string
	CreatorID   string
	AssigneeID  string
	DueAt       *time.Time
	Priority    Priority
	Status      Status
	// DueSoonSentAt and OverdueSentAt mark that the due scan already
	// announced this due date (TASK-06). Changing DueAt clears both.
	DueSoonSentAt *time.Time
	OverdueSentAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewInput is what a caller chooses when creating a task.
type NewInput struct {
	WorkspaceID     string
	ChannelID       string
	CreatorID       string
	Title           string
	Description     string
	AssigneeID      string
	DueAt           *time.Time
	Priority        Priority // empty means normal
	SourceMessageID string
}

func validTitle(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("title is required: %w", app.ErrInvalid)
	}
	if utf8.RuneCountInString(s) > MaxTitleRunes {
		return "", fmt.Errorf("title is longer than %d characters: %w", MaxTitleRunes, app.ErrInvalid)
	}
	return s, nil
}

func validDescription(s string) (string, error) {
	if utf8.RuneCountInString(s) > MaxDescriptionRunes {
		return "", fmt.Errorf("description is longer than %d characters: %w", MaxDescriptionRunes, app.ErrInvalid)
	}
	return s, nil
}

// New builds a todo task. The caller supplies the ID and the time so it
// controls identity and the clock.
func New(in NewInput, id string, now time.Time) (Task, error) {
	title, err := validTitle(in.Title)
	if err != nil {
		return Task{}, err
	}
	desc, err := validDescription(in.Description)
	if err != nil {
		return Task{}, err
	}
	pr := in.Priority
	if pr == "" {
		pr = PriorityNormal
	}
	if !pr.Valid() {
		return Task{}, fmt.Errorf("priority must be low, normal or high: %w", app.ErrInvalid)
	}
	now = now.UTC()
	t := Task{
		ID: id, WorkspaceID: in.WorkspaceID, ChannelID: in.ChannelID, SourceMessageID: in.SourceMessageID,
		Title: title, Description: desc, CreatorID: in.CreatorID, AssigneeID: in.AssigneeID,
		Priority: pr, Status: StatusTodo, CreatedAt: now, UpdatedAt: now,
	}
	if in.DueAt != nil {
		d := in.DueAt.UTC()
		t.DueAt = &d
	}
	return t, nil
}

// CanEdit reports whether p may change the task: its creator, its assignee,
// the owner and an admin (TASK-05). Reading is the channel's rule, not this
// one's. A status change follows the same rule.
func (t Task) CanEdit(p app.Principal) bool {
	if p.UserID == "" {
		return false
	}
	return p.UserID == t.CreatorID || p.UserID == t.AssigneeID || p.Role == app.RoleOwner || p.Role == app.RoleAdmin
}

// Patch is a partial edit. A nil pointer leaves a field alone. Assignee and
// due date can be cleared, so they carry an explicit Set flag: AssigneeSet
// with an empty AssigneeID clears it, DueSet with a nil DueAt clears it.
type Patch struct {
	Title       *string
	Description *string
	Priority    *Priority
	Status      *Status
	AssigneeSet bool
	AssigneeID  string
	DueSet      bool
	DueAt       *time.Time
}

// Outcome says which events an edit calls for.
type Outcome struct {
	// Assigned is true when the task now has a different, non-empty assignee.
	Assigned bool
	// StatusChanged is true when the status differs; From is the old one.
	StatusChanged bool
	From          Status
	// Updated is true when any other field changed, including clearing the
	// assignee.
	Updated bool
}

// Any reports whether the edit changed anything.
func (o Outcome) Any() bool { return o.Assigned || o.StatusChanged || o.Updated }

// Apply validates p and applies it. It leaves the task untouched when p is
// invalid or changes nothing; otherwise it sets UpdatedAt to now. Changing
// the due date clears both sent marks, so the new date is announced again.
func (t *Task) Apply(p Patch, now time.Time) (Outcome, error) {
	next := *t
	var out Outcome

	if p.Title != nil {
		title, err := validTitle(*p.Title)
		if err != nil {
			return Outcome{}, err
		}
		if title != next.Title {
			next.Title, out.Updated = title, true
		}
	}
	if p.Description != nil {
		desc, err := validDescription(*p.Description)
		if err != nil {
			return Outcome{}, err
		}
		if desc != next.Description {
			next.Description, out.Updated = desc, true
		}
	}
	if p.Priority != nil {
		if !p.Priority.Valid() {
			return Outcome{}, fmt.Errorf("priority must be low, normal or high: %w", app.ErrInvalid)
		}
		if *p.Priority != next.Priority {
			next.Priority, out.Updated = *p.Priority, true
		}
	}
	if p.Status != nil {
		if !p.Status.Valid() {
			return Outcome{}, fmt.Errorf("status must be todo, doing or done: %w", app.ErrInvalid)
		}
		if *p.Status != next.Status {
			out.StatusChanged, out.From, next.Status = true, next.Status, *p.Status
		}
	}
	if p.AssigneeSet && p.AssigneeID != next.AssigneeID {
		if p.AssigneeID == "" {
			out.Updated = true
		} else {
			out.Assigned = true
		}
		next.AssigneeID = p.AssigneeID
	}
	if p.DueSet && !sameTime(p.DueAt, next.DueAt) {
		next.DueAt = nil
		if p.DueAt != nil {
			d := p.DueAt.UTC()
			next.DueAt = &d
		}
		next.DueSoonSentAt, next.OverdueSentAt = nil, nil
		out.Updated = true
	}

	if !out.Any() {
		return Outcome{}, nil
	}
	next.UpdatedAt = now.UTC()
	*t = next
	return out, nil
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

var mentionRe = regexp.MustCompile(`<@([0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})>`)

// ParseCommand reads the arguments of "/task <title>": the first "<@user>"
// token anywhere sets the assignee and is removed; the rest is the title.
// Later tokens stay in the title as written.
func ParseCommand(args string) (title, assigneeID string, err error) {
	rest := args
	if loc := mentionRe.FindStringSubmatchIndex(args); loc != nil {
		assigneeID = strings.ToLower(args[loc[2]:loc[3]])
		rest = args[:loc[0]] + " " + args[loc[1]:]
	}
	title, err = validTitle(strings.Join(strings.Fields(rest), " "))
	if err != nil {
		return "", "", err
	}
	return title, assigneeID, nil
}
