package domain

import (
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Status is whether a member can use the workspace.
type Status string

const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
)

// Member is a user's membership in one workspace.
type Member struct {
	WorkspaceID string
	UserID      string
	Role        app.Role
	Status      Status
	JoinedAt    time.Time
}

// ValidRole reports whether r is one of the five workspace roles.
func ValidRole(r app.Role) bool {
	switch r {
	case app.RoleOwner, app.RoleAdmin, app.RoleManager, app.RoleMember, app.RoleGuest:
		return true
	}
	return false
}

// ValidStatus reports whether s is a member status.
func ValidStatus(s Status) bool { return s == StatusActive || s == StatusDeactivated }

// CanManage reports whether a role may manage members and invites: owners
// and admins only (SRS section 2).
func CanManage(r app.Role) bool { return r == app.RoleOwner || r == app.RoleAdmin }

// CheckChange applies a role and/or status change to target on behalf of
// actor and returns the resulting member. Nil role or status keeps the
// current value. activeOwners is the number of active owners now.
//
// Rules (WSP-03, WSP-04):
//   - only owners and admins manage members;
//   - only an owner may grant the owner role or change an owner;
//   - an active owner who is the last one cannot be demoted or deactivated.
func CheckChange(actor app.Role, target Member, role *app.Role, status *Status, activeOwners int) (Member, error) {
	if !CanManage(actor) {
		return Member{}, fmt.Errorf("only owners and admins manage members: %w", app.ErrForbidden)
	}
	if role == nil && status == nil {
		return Member{}, fmt.Errorf("role or status is required: %w", app.ErrInvalid)
	}
	next := target
	if role != nil {
		if !ValidRole(*role) {
			return Member{}, fmt.Errorf("unknown role %q: %w", *role, app.ErrInvalid)
		}
		next.Role = *role
	}
	if status != nil {
		if !ValidStatus(*status) {
			return Member{}, fmt.Errorf("unknown status %q: %w", *status, app.ErrInvalid)
		}
		next.Status = *status
	}
	if (target.Role == app.RoleOwner || next.Role == app.RoleOwner) && actor != app.RoleOwner {
		return Member{}, fmt.Errorf("only an owner may grant or change the owner role: %w", app.ErrForbidden)
	}
	if isActiveOwner(target) && !isActiveOwner(next) && activeOwners <= 1 {
		return Member{}, errLastOwner()
	}
	return next, nil
}

// CheckRemove decides whether actor may remove target from the workspace.
func CheckRemove(actor app.Role, target Member, activeOwners int) error {
	if !CanManage(actor) {
		return fmt.Errorf("only owners and admins manage members: %w", app.ErrForbidden)
	}
	if target.Role == app.RoleOwner && actor != app.RoleOwner {
		return fmt.Errorf("only an owner may remove an owner: %w", app.ErrForbidden)
	}
	if isActiveOwner(target) && activeOwners <= 1 {
		return errLastOwner()
	}
	return nil
}

func isActiveOwner(m Member) bool { return m.Role == app.RoleOwner && m.Status == StatusActive }

func errLastOwner() error {
	return fmt.Errorf("a workspace must keep at least one active owner: %w", app.ErrConflict)
}
