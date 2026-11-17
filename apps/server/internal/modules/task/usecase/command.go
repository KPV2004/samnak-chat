package usecase

import (
	"context"
	"errors"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

func isNotFound(err error) bool { return errors.Is(err, app.ErrNotFound) }

// CreateFromCommand runs "/task <title>" for the caller in a channel: the
// first "<@user>" token sets the assignee and the rest is the title
// (TASK-01). The task's card is the reply, so nothing is returned.
func (s *Service) CreateFromCommand(ctx context.Context, p app.Principal, in app.CommandInput) error {
	title, assigneeID, err := domain.ParseCommand(in.Args)
	if err != nil {
		return err
	}
	_, err = s.Create(ctx, p, CreateInput{ChannelID: in.ChannelID, Title: title, AssigneeID: assigneeID})
	return err
}
