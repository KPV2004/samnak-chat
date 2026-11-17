package task

import "time"

// The events below are published in the transaction that wrote the task, so
// each is delivered after the commit, at least once (PLT-10). Handlers must
// be idempotent. The module declares them in Init.

// TaskCreated is published when a task is created (TASK-08).
type TaskCreated struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
	ChannelID   string `json:"channel_id"`
	CreatorID   string `json:"creator_id"`
	AssigneeID  string `json:"assignee_id"` // empty when unassigned
}

func (TaskCreated) EventName() string { return "task.created" }

// TaskAssigned is published when a task is created with an assignee and when
// its assignee changes to another person (TASK-08).
type TaskAssigned struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
	AssigneeID  string `json:"assignee_id"`
	ActorID     string `json:"actor_id"`
	// UpdatedAt is the task's updated_at at the assignment, so each
	// assignment has its own identity.
	UpdatedAt time.Time `json:"updated_at"`
}

func (TaskAssigned) EventName() string { return "task.assigned" }

// TaskStatusChanged is published when the status changes (TASK-08).
type TaskStatusChanged struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
	From        string `json:"from"`
	To          string `json:"to"`
	ActorID     string `json:"actor_id"`
}

func (TaskStatusChanged) EventName() string { return "task.status_changed" }

// TaskUpdated is published when any other field changes: title, description,
// priority, due date, or clearing the assignee.
type TaskUpdated struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
}

func (TaskUpdated) EventName() string { return "task.updated" }

// TaskDueSoon is published once per due date, when the task is due within 24
// hours (TASK-06, TASK-08).
type TaskDueSoon struct {
	WorkspaceID string    `json:"workspace_id"`
	TaskID      string    `json:"task_id"`
	AssigneeID  string    `json:"assignee_id"`
	DueAt       time.Time `json:"due_at"`
}

func (TaskDueSoon) EventName() string { return "task.due_soon" }

// TaskOverdue is published once per due date, when the due date has passed and
// the task is not done (TASK-06, TASK-08).
type TaskOverdue struct {
	WorkspaceID string    `json:"workspace_id"`
	TaskID      string    `json:"task_id"`
	AssigneeID  string    `json:"assignee_id"`
	DueAt       time.Time `json:"due_at"`
}

func (TaskOverdue) EventName() string { return "task.overdue" }
