package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// CreateInput is what a caller chooses when creating a task. The workspace and
// the creator come from the Principal.
type CreateInput struct {
	ChannelID       string
	Title           string
	Description     string
	AssigneeID      string
	DueAt           *time.Time
	Priority        domain.Priority
	SourceMessageID string
}

// Create stores a task in a channel the caller can read and publishes
// task.created, and task.assigned when it has an assignee, in the same
// transaction (TASK-01, TASK-02, TASK-08). The card is posted later by the
// handler of task.created.
func (s *Service) Create(ctx context.Context, p app.Principal, in CreateInput) (domain.Task, error) {
	if !validID(in.ChannelID) {
		return domain.Task{}, invalid("channel_id is not a valid ID")
	}
	if in.AssigneeID != "" && !validID(in.AssigneeID) {
		return domain.Task{}, invalid("assignee_id is not a valid ID")
	}
	if in.SourceMessageID != "" && !validID(in.SourceMessageID) {
		return domain.Task{}, invalid("source_message_id is not a valid ID")
	}
	task, err := domain.New(domain.NewInput{
		WorkspaceID: p.WorkspaceID, ChannelID: in.ChannelID, CreatorID: p.UserID, Title: in.Title, Description: in.Description,
		AssigneeID: in.AssigneeID, DueAt: in.DueAt, Priority: in.Priority, SourceMessageID: in.SourceMessageID,
	}, newID(), s.clock())
	if err != nil {
		return domain.Task{}, err
	}
	ok, err := s.canRead(ctx, p.WorkspaceID, in.ChannelID, p.UserID)
	if err != nil {
		return domain.Task{}, err
	}
	if !ok {
		return domain.Task{}, fmt.Errorf("channel %q: %w", in.ChannelID, app.ErrNotFound)
	}
	if task.AssigneeID != "" {
		if err := s.checkAssignee(ctx, p.WorkspaceID, task.ChannelID, task.AssigneeID); err != nil {
			return domain.Task{}, err
		}
	}

	var stored domain.Task
	err = s.d.Tx.Do(ctx, func(ctx context.Context) error {
		var err error
		if stored, err = s.d.Repo.Insert(ctx, task); err != nil {
			return err
		}
		if err := s.publish(func(ev Events) error { return ev.Created(ctx, stored) }); err != nil {
			return err
		}
		if stored.AssigneeID != "" {
			return s.publish(func(ev Events) error { return ev.Assigned(ctx, stored, p.UserID) })
		}
		return nil
	})
	if err != nil {
		return domain.Task{}, err
	}
	return stored, nil
}

// publish calls fn when events are configured.
func (s *Service) publish(fn func(Events) error) error {
	if s.d.Events == nil {
		return nil
	}
	return fn(s.d.Events)
}
