package workspace_test

import (
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

func isForbidden(err error) bool { return errors.Is(err, app.ErrForbidden) }

func TestCanMatrix(t *testing.T) {
	a := workspace.NewAuthorizer()
	roles := []app.Role{app.RoleOwner, app.RoleAdmin, app.RoleManager, app.RoleMember, app.RoleGuest}
	want := map[workspace.Action]map[app.Role]bool{
		workspace.ActionMemberList:   {app.RoleOwner: true, app.RoleAdmin: true, app.RoleManager: true, app.RoleMember: true, app.RoleGuest: true},
		workspace.ActionMemberManage: {app.RoleOwner: true, app.RoleAdmin: true},
		workspace.ActionInviteManage: {app.RoleOwner: true, app.RoleAdmin: true},
	}
	for action, byRole := range want {
		for _, r := range roles {
			p := app.Principal{UserID: "u", WorkspaceID: "w", Role: r}
			if got := a.Can(p, action); got != byRole[r] {
				t.Errorf("Can(%s, %s) = %v, want %v", r, action, got, byRole[r])
			}
		}
	}
}

// api.go has no constant for the group action yet, so the test spells it out.
func TestGroupManageIsForOwnersAndAdmins(t *testing.T) {
	a := workspace.NewAuthorizer()
	const action workspace.Action = "workspace.group.manage"
	for r, want := range map[app.Role]bool{app.RoleOwner: true, app.RoleAdmin: true, app.RoleManager: false, app.RoleMember: false, app.RoleGuest: false} {
		if got := a.Can(app.Principal{UserID: "u", WorkspaceID: "w", Role: r}, action); got != want {
			t.Errorf("Can(%s, %s) = %v, want %v", r, action, got, want)
		}
	}
}

func TestCanDeniesWithoutWorkspaceOrRoleOrAction(t *testing.T) {
	a := workspace.NewAuthorizer()
	if a.Can(app.Principal{UserID: "u", Role: app.RoleOwner}, workspace.ActionMemberManage) {
		t.Error("a principal with no workspace must be denied")
	}
	if a.Can(app.Principal{UserID: "u", WorkspaceID: "w"}, workspace.ActionMemberList) {
		t.Error("a principal with no role must be denied")
	}
	if a.Can(app.Principal{UserID: "u", WorkspaceID: "w", Role: app.RoleOwner}, "nobody.registered.this") {
		t.Error("an unregistered action must be denied")
	}
}

func TestLaterModulesRegisterTheirOwnActions(t *testing.T) {
	a := workspace.NewAuthorizer()
	const action workspace.Action = "approval.type.manage"
	if err := a.Register(action, app.RoleOwner, app.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := a.Register(action, app.RoleAdmin, app.RoleOwner); err != nil {
		t.Fatalf("registering the same roles again must be fine: %v", err)
	}
	if err := a.Register(action, app.RoleMember); err == nil {
		t.Fatal("registering different roles for a taken action must fail")
	}
	if err := a.Register("", app.RoleOwner); err == nil {
		t.Fatal("empty action must fail")
	}
	if err := a.Register("x.y", "boss"); err == nil {
		t.Fatal("unknown role must fail")
	}
	if !a.Can(app.Principal{WorkspaceID: "w", Role: app.RoleAdmin}, action) || a.Can(app.Principal{WorkspaceID: "w", Role: app.RoleMember}, action) {
		t.Fatal("registered action not enforced")
	}
}
