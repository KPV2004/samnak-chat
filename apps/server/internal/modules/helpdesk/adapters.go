package helpdesk

import (
	"context"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

// permissions adapts workspace.Authorizer. Without it nobody is an admin.
type permissions struct{ s *app.Services }

var _ usecase.Permissions = permissions{}

func (p permissions) Can(pr app.Principal, action string) bool {
	a, ok := app.Lookup[workspace.Authorizer](p.s)
	return ok && a.Can(pr, workspace.Action(action))
}

// members adapts app.MembershipResolver. Without it nobody is an active member,
// so nobody can be made an agent.
type members struct{ s *app.Services }

var _ usecase.Members = members{}

func (m members) Active(ctx context.Context, workspaceID, userID string) bool {
	r, ok := app.Lookup[app.MembershipResolver](m.s)
	if !ok {
		return false
	}
	_, err := r.Role(ctx, workspaceID, userID)
	return err == nil
}
