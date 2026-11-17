package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// Update edits a task the caller can read and may edit: its creator, its
// assignee, the owner or an admin (TASK-05). It publishes task.assigned,
// task.status_changed and task.updated for what changed, in the same
// transaction as the write (TASK-08). An edit that changes nothing writes and
// publishes nothing.
func (s *Service) Update(ctx context.Context, p app.Principal, id string, patch domain.Patch) (domain.Task, error) {
	if patch.AssigneeSet && patch.AssigneeID != "" && !validID(patch.AssigneeID) {
		return domain.Task{}, invalid("assignee_id is not a valid ID")
	}
	var stored domain.Task
	err := s.d.Tx.Do(ctx, func(ctx context.Context) error {
		t, err := s.d.Repo.GetForUpdate(ctx, p.WorkspaceID, id)
		if t, err = s.readable(ctx, p, t, err); err != nil {
			return err
		}
		if !t.CanEdit(p) {
			return fmt.Errorf("only the creator, the assignee or an admin can edit this task: %w", app.ErrForbidden)
		}
		// A new assignee must be a member who can read the channel; an
		// assignee nobody changed is left alone even if they have left since.
		if patch.AssigneeSet && patch.AssigneeID != "" && patch.AssigneeID != t.AssigneeID {
			if err := s.checkAssignee(ctx, p.WorkspaceID, t.ChannelID, patch.AssigneeID); err != nil {
				return err
			}
		}
		out, err := t.Apply(patch, s.clock())
		if err != nil {
			return err
		}
		if !out.Any() {
			stored = t
			return nil
		}
		if stored, err = s.d.Repo.Update(ctx, t); err != nil {
			return err
		}
		return s.publishEdit(ctx, p, stored, out)
	})
	if err != nil {
		return domain.Task{}, err
	}
	return stored, nil
}

func (s *Service) publishEdit(ctx context.Context, p app.Principal, t domain.Task, out domain.Outcome) error {
	if out.Assigned {
		if err := s.publish(func(ev Events) error { return ev.Assigned(ctx, t, p.UserID) }); err != nil {
			return err
		}
	}
	if out.StatusChanged {
		if err := s.publish(func(ev Events) error { return ev.StatusChanged(ctx, t, out.From, p.UserID) }); err != nil {
			return err
		}
	}
	if out.Updated {
		return s.publish(func(ev Events) error { return ev.Updated(ctx, t) })
	}
	return nil
}
