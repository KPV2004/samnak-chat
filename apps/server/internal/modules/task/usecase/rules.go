package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// Notification kinds the task module produces (TASK-06). The web renders them
// from the notification's data.
const (
	KindAssigned = "task.assigned"
	KindDueSoon  = "task.due_soon"
	KindOverdue  = "task.overdue"
)

// AssignedInput is what the notification rule needs from task.assigned.
type AssignedInput struct {
	WorkspaceID string
	TaskID      string
	AssigneeID  string
	ActorID     string
	// UpdatedAt is the task's updated_at when it was assigned. It makes the
	// key of each assignment distinct and stable on redelivery.
	UpdatedAt time.Time
}

// DueKind says which due event a notification answers.
type DueKind string

const (
	DueSoon DueKind = KindDueSoon
	Overdue DueKind = KindOverdue
)

// DueInput is what the notification rule needs from task.due_soon and
// task.overdue.
type DueInput struct {
	WorkspaceID string
	TaskID      string
	AssigneeID  string
	DueAt       time.Time
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *Service) notifyData(t domain.Task) map[string]any {
	data := map[string]any{"task_id": t.ID, "channel_id": t.ChannelID, "title": t.Title}
	if t.MessageID != "" {
		data["message_id"] = t.MessageID
	}
	if t.DueAt != nil {
		data["due_at"] = stamp(*t.DueAt)
	}
	return data
}

// NotifyAssigned is the rule for task.assigned: it tells the assignee, unless
// they assigned themselves (TASK-06).
func (s *Service) NotifyAssigned(ctx context.Context, in AssignedInput) ([]app.Notification, error) {
	if in.AssigneeID == "" || in.AssigneeID == in.ActorID {
		return nil, nil
	}
	t, err := s.d.Repo.Get(ctx, in.WorkspaceID, in.TaskID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	title := "You were assigned a task"
	if name := s.names(ctx, in.ActorID)[in.ActorID]; name != "" {
		title = name + " assigned you a task"
	}
	data := s.notifyData(t)
	data["actor_id"] = in.ActorID
	return []app.Notification{{
		WorkspaceID: in.WorkspaceID, UserID: in.AssigneeID, Kind: KindAssigned, Title: title, Body: t.Title,
		Link: "/chat/" + t.ChannelID, Data: data,
		Key: KindAssigned + ":" + in.TaskID + ":" + in.AssigneeID + ":" + stamp(in.UpdatedAt),
	}}, nil
}

// NotifyDue is the rule for task.due_soon and task.overdue: it tells the
// assignee. An event that is stale by the time it is handled (the task was
// reassigned, rescheduled or finished) tells nobody.
func (s *Service) NotifyDue(ctx context.Context, kind DueKind, in DueInput) ([]app.Notification, error) {
	t, err := s.d.Repo.Get(ctx, in.WorkspaceID, in.TaskID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if t.Status == domain.StatusDone || t.AssigneeID == "" || t.AssigneeID != in.AssigneeID || t.DueAt == nil || !t.DueAt.Equal(in.DueAt) {
		return nil, nil
	}
	title := "Task due soon"
	if kind == Overdue {
		title = "Task overdue"
	}
	return []app.Notification{{
		WorkspaceID: in.WorkspaceID, UserID: in.AssigneeID, Kind: string(kind), Title: title, Body: t.Title,
		Link: "/chat/" + t.ChannelID, Data: s.notifyData(t),
		Key: string(kind) + ":" + in.TaskID + ":" + stamp(in.DueAt),
	}}, nil
}
