// Package workspace owns workspaces, members, roles and invites (SRS section 6).
// This file and events.go are the only parts other modules may import.
package workspace

import (
	"context"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Action names something a user may do, written "<module>.<thing>.<verb>".
type Action string

// Actions of the workspace module.
const (
	// ActionMemberList lets every member see who is in the workspace.
	ActionMemberList Action = "workspace.member.list"
	// ActionMemberManage covers changing roles, deactivating and removing members.
	ActionMemberManage Action = "workspace.member.manage"
	// ActionInviteManage covers creating, listing and revoking invites.
	ActionInviteManage Action = "workspace.invite.manage"
)

// Authorizer is the one shared permission check (WSP-07). workspace provides
// it through app.Services; look it up with app.Lookup[workspace.Authorizer]
// and treat a missing service as "deny".
//
// Can answers from the principal's role in its workspace. Rules that depend
// on the resource, such as "only the named approver", belong in the owning
// module's domain layer, after Can has passed.
type Authorizer interface {
	Can(p app.Principal, action Action) bool
}

// ActionRegistry is how a later module adds its own actions. Call Register
// from Module.Init, because every module's Provide runs before any Init:
//
//	if reg, ok := app.Lookup[workspace.ActionRegistry](d.Services); ok {
//		err := reg.Register("approval.type.manage", app.RoleOwner, app.RoleAdmin)
//	}
//
// An action nobody registered is denied for every role.
type ActionRegistry interface {
	// Register allows action for the given roles. Registering the same action
	// again with the same roles is fine; with different roles it is an error.
	Register(action Action, roles ...app.Role) error
}

// GroupDirectory answers questions about groups (departments or teams) and
// their managers (WSP-09). workspace provides it through app.Services; a
// consumer must handle Lookup returning false.
type GroupDirectory interface {
	// ManagersOf returns the managers of every group userID belongs to in
	// the workspace, without userID itself, without duplicates, ordered by
	// group name and then by when they became manager. approval uses the
	// first one as the default approver (APR-02). Empty when there is none.
	ManagersOf(ctx context.Context, workspaceID, userID string) ([]string, error)
}
