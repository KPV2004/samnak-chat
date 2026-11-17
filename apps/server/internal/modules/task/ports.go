package task

import (
	"context"
	"errors"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

// members adapts app.MembershipResolver to usecase.Members. Without the
// service nobody is a member, so no task can be assigned.
type members struct{ r app.MembershipResolver }

var _ usecase.Members = members{}

func (m members) IsActiveMember(ctx context.Context, workspaceID, userID string) (bool, error) {
	if m.r == nil {
		return false, nil
	}
	if _, err := m.r.Role(ctx, workspaceID, userID); err != nil {
		if errors.Is(err, app.ErrForbidden) || errors.Is(err, app.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// directory adapts auth.Directory; without it names are empty.
type directory struct{ d auth.Directory }

var _ usecase.Directory = directory{}

func (d directory) Names(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if d.d == nil {
		return out
	}
	users, err := d.d.Users(ctx, ids)
	if err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = u.DisplayName
	}
	return out
}
