package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

func TestValidateSlug(t *testing.T) {
	ok := []string{"abc", "my-team", "a1b2c3", "123", strings.Repeat("a", 40)}
	bad := []string{"", "ab", "My-Team", "has space", "under_score", "thai-ไทย", strings.Repeat("a", 41), "dot.dot"}
	for _, s := range ok {
		if err := domain.ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range bad {
		if err := domain.ValidateSlug(s); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("ValidateSlug(%q) = %v, want ErrInvalid", s, err)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	got, err := domain.NormalizeName("  Acme Co  ")
	if err != nil || got != "Acme Co" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, s := range []string{"", "   ", strings.Repeat("ก", 101)} {
		if _, err := domain.NormalizeName(s); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeName(%q) = %v, want ErrInvalid", s, err)
		}
	}
}

func member(role app.Role, status domain.Status) domain.Member {
	return domain.Member{WorkspaceID: "w", UserID: "u", Role: role, Status: status}
}

func ptr[T any](v T) *T { return &v }

func TestCheckChange(t *testing.T) {
	type tc struct {
		name      string
		actor     app.Role
		target    domain.Member
		role      *app.Role
		status    *domain.Status
		owners    int
		wantErr   error
		wantRole  app.Role
		wantState domain.Status
	}
	active, deact := domain.StatusActive, domain.StatusDeactivated
	cases := []tc{
		{name: "owner promotes member to admin", actor: app.RoleOwner, target: member(app.RoleMember, active), role: ptr(app.RoleAdmin), owners: 1, wantRole: app.RoleAdmin, wantState: active},
		{name: "admin changes member to manager", actor: app.RoleAdmin, target: member(app.RoleMember, active), role: ptr(app.RoleManager), owners: 1, wantRole: app.RoleManager, wantState: active},
		{name: "admin changes another admin", actor: app.RoleAdmin, target: member(app.RoleAdmin, active), role: ptr(app.RoleGuest), owners: 1, wantRole: app.RoleGuest, wantState: active},
		{name: "admin cannot grant owner", actor: app.RoleAdmin, target: member(app.RoleMember, active), role: ptr(app.RoleOwner), owners: 1, wantErr: app.ErrForbidden},
		{name: "admin cannot touch an owner", actor: app.RoleAdmin, target: member(app.RoleOwner, active), role: ptr(app.RoleMember), owners: 2, wantErr: app.ErrForbidden},
		{name: "admin cannot deactivate an owner", actor: app.RoleAdmin, target: member(app.RoleOwner, active), status: &deact, owners: 2, wantErr: app.ErrForbidden},
		{name: "owner grants owner", actor: app.RoleOwner, target: member(app.RoleAdmin, active), role: ptr(app.RoleOwner), owners: 1, wantRole: app.RoleOwner, wantState: active},
		{name: "owner demotes another owner", actor: app.RoleOwner, target: member(app.RoleOwner, active), role: ptr(app.RoleAdmin), owners: 2, wantRole: app.RoleAdmin, wantState: active},
		{name: "last owner cannot be demoted", actor: app.RoleOwner, target: member(app.RoleOwner, active), role: ptr(app.RoleAdmin), owners: 1, wantErr: app.ErrConflict},
		{name: "last owner cannot be deactivated", actor: app.RoleOwner, target: member(app.RoleOwner, active), status: &deact, owners: 1, wantErr: app.ErrConflict},
		{name: "owner may deactivate an owner when another remains", actor: app.RoleOwner, target: member(app.RoleOwner, active), status: &deact, owners: 2, wantRole: app.RoleOwner, wantState: deact},
		{name: "same role on last owner is fine", actor: app.RoleOwner, target: member(app.RoleOwner, active), role: ptr(app.RoleOwner), owners: 1, wantRole: app.RoleOwner, wantState: active},
		{name: "reactivate a member", actor: app.RoleAdmin, target: member(app.RoleMember, deact), status: &active, owners: 1, wantRole: app.RoleMember, wantState: active},
		{name: "deactivated owner is not counted as the last one", actor: app.RoleOwner, target: member(app.RoleOwner, deact), role: ptr(app.RoleAdmin), owners: 1, wantRole: app.RoleAdmin, wantState: deact},
		{name: "manager cannot manage", actor: app.RoleManager, target: member(app.RoleMember, active), role: ptr(app.RoleGuest), owners: 1, wantErr: app.ErrForbidden},
		{name: "member cannot manage", actor: app.RoleMember, target: member(app.RoleGuest, active), status: &deact, owners: 1, wantErr: app.ErrForbidden},
		{name: "guest cannot manage", actor: app.RoleGuest, target: member(app.RoleMember, active), role: ptr(app.RoleGuest), owners: 1, wantErr: app.ErrForbidden},
		{name: "unknown role", actor: app.RoleOwner, target: member(app.RoleMember, active), role: ptr(app.Role("boss")), owners: 1, wantErr: app.ErrInvalid},
		{name: "unknown status", actor: app.RoleOwner, target: member(app.RoleMember, active), status: ptr(domain.Status("gone")), owners: 1, wantErr: app.ErrInvalid},
		{name: "empty change", actor: app.RoleOwner, target: member(app.RoleMember, active), owners: 1, wantErr: app.ErrInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := domain.CheckChange(c.actor, c.target, c.role, c.status, c.owners)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Role != c.wantRole || got.Status != c.wantState {
				t.Fatalf("got %s/%s, want %s/%s", got.Role, got.Status, c.wantRole, c.wantState)
			}
		})
	}
}

func TestCheckRemove(t *testing.T) {
	active, deact := domain.StatusActive, domain.StatusDeactivated
	cases := []struct {
		name    string
		actor   app.Role
		target  domain.Member
		owners  int
		wantErr error
	}{
		{"admin removes member", app.RoleAdmin, member(app.RoleMember, active), 1, nil},
		{"admin removes deactivated admin", app.RoleAdmin, member(app.RoleAdmin, deact), 1, nil},
		{"admin cannot remove owner", app.RoleAdmin, member(app.RoleOwner, active), 2, app.ErrForbidden},
		{"owner removes owner when another remains", app.RoleOwner, member(app.RoleOwner, active), 2, nil},
		{"last owner cannot be removed", app.RoleOwner, member(app.RoleOwner, active), 1, app.ErrConflict},
		{"deactivated owner can be removed", app.RoleOwner, member(app.RoleOwner, deact), 1, nil},
		{"manager cannot remove", app.RoleManager, member(app.RoleMember, active), 1, app.ErrForbidden},
		{"member cannot remove", app.RoleMember, member(app.RoleGuest, active), 1, app.ErrForbidden},
		{"guest cannot remove", app.RoleGuest, member(app.RoleMember, active), 1, app.ErrForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := domain.CheckRemove(c.actor, c.target, c.owners)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestCanManage(t *testing.T) {
	want := map[app.Role]bool{app.RoleOwner: true, app.RoleAdmin: true}
	for _, r := range []app.Role{app.RoleOwner, app.RoleAdmin, app.RoleManager, app.RoleMember, app.RoleGuest, ""} {
		if got := domain.CanManage(r); got != want[r] {
			t.Errorf("CanManage(%q) = %v", r, got)
		}
	}
}
