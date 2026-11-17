package task

import (
	"context"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

// publisher implements usecase.Events on the bus. The use cases call it
// inside their transaction, so with the outbox each event is stored with the
// write and delivered after the commit. An error is returned, because it
// must roll the write back.
type publisher struct{ bus app.Publisher }

var _ usecase.Events = publisher{}

func (p publisher) publish(ctx context.Context, evt app.Event) error {
	if p.bus == nil {
		return nil
	}
	return p.bus.Publish(ctx, evt)
}

func (p publisher) Created(ctx context.Context, t domain.Task) error {
	return p.publish(ctx, TaskCreated{
		WorkspaceID: t.WorkspaceID, TaskID: t.ID, ChannelID: t.ChannelID, CreatorID: t.CreatorID, AssigneeID: t.AssigneeID,
	})
}

func (p publisher) Assigned(ctx context.Context, t domain.Task, actorID string) error {
	return p.publish(ctx, TaskAssigned{
		WorkspaceID: t.WorkspaceID, TaskID: t.ID, AssigneeID: t.AssigneeID, ActorID: actorID, UpdatedAt: t.UpdatedAt.UTC(),
	})
}

func (p publisher) StatusChanged(ctx context.Context, t domain.Task, from domain.Status, actorID string) error {
	return p.publish(ctx, TaskStatusChanged{
		WorkspaceID: t.WorkspaceID, TaskID: t.ID, From: string(from), To: string(t.Status), ActorID: actorID,
	})
}

func (p publisher) Updated(ctx context.Context, t domain.Task) error {
	return p.publish(ctx, TaskUpdated{WorkspaceID: t.WorkspaceID, TaskID: t.ID})
}

func (p publisher) DueSoon(ctx context.Context, d usecase.Due) error {
	return p.publish(ctx, TaskDueSoon{WorkspaceID: d.WorkspaceID, TaskID: d.TaskID, AssigneeID: d.AssigneeID, DueAt: d.DueAt.UTC()})
}

func (p publisher) Overdue(ctx context.Context, d usecase.Due) error {
	return p.publish(ctx, TaskOverdue{WorkspaceID: d.WorkspaceID, TaskID: d.TaskID, AssigneeID: d.AssigneeID, DueAt: d.DueAt.UTC()})
}
