package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

// maxGroupMembers bounds one PUT so a request stays a request.
const maxGroupMembers = 1000

// GroupMemberView is a group member with the name the auth module knows.
type GroupMemberView struct {
	GroupMember
	DisplayName string
}

func requireGroupManager(p app.Principal) error {
	if !domain.CanManage(p.Role) {
		return fmt.Errorf("only owners and admins manage groups: %w", app.ErrForbidden)
	}
	return nil
}

func nameTaken(err error, name string) error {
	if errors.Is(err, app.ErrConflict) {
		return fmt.Errorf("a group named %q already exists: %w", name, app.ErrConflict)
	}
	return err
}

// ListGroups returns the groups of the principal's workspace (WSP-09).
func (s *Service) ListGroups(ctx context.Context, p app.Principal) ([]GroupView, error) {
	return s.repo.ListGroups(ctx, p.WorkspaceID)
}

// CreateGroup makes an empty group. Names are unique per workspace, ignoring case.
func (s *Service) CreateGroup(ctx context.Context, p app.Principal, name string) (GroupView, error) {
	if err := requireGroupManager(p); err != nil {
		return GroupView{}, err
	}
	name, err := domain.NormalizeGroupName(name)
	if err != nil {
		return GroupView{}, err
	}
	g := domain.Group{ID: newID(), WorkspaceID: p.WorkspaceID, Name: name, CreatedAt: s.now().UTC()}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.InsertGroup(ctx, g); err != nil {
			return err
		}
		return s.record(ctx, p, domain.AuditGroupCreated, domain.TargetGroup, g.ID, nil)
	})
	if err != nil {
		return GroupView{}, nameTaken(err, name)
	}
	return GroupView{Group: g, ManagerIDs: []string{}}, nil
}

// RenameGroup changes a group's name.
func (s *Service) RenameGroup(ctx context.Context, p app.Principal, groupID, name string) (GroupView, error) {
	if err := requireGroupManager(p); err != nil {
		return GroupView{}, err
	}
	name, err := domain.NormalizeGroupName(name)
	if err != nil {
		return GroupView{}, err
	}
	var g GroupView
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.RenameGroup(ctx, p.WorkspaceID, groupID, name); err != nil {
			return err
		}
		if err := s.record(ctx, p, domain.AuditGroupUpdated, domain.TargetGroup, groupID, nil); err != nil {
			return err
		}
		g, err = s.repo.GetGroup(ctx, p.WorkspaceID, groupID)
		return err
	})
	if err != nil {
		return GroupView{}, nameTaken(err, name)
	}
	return g, nil
}

// DeleteGroup deletes a group and its memberships.
func (s *Service) DeleteGroup(ctx context.Context, p app.Principal, groupID string) error {
	if err := requireGroupManager(p); err != nil {
		return err
	}
	return s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.DeleteGroup(ctx, p.WorkspaceID, groupID); err != nil {
			return err
		}
		return s.record(ctx, p, domain.AuditGroupDeleted, domain.TargetGroup, groupID, nil)
	})
}

// ListGroupMembers returns who is in a group, with names. When the directory
// is unavailable the names stay empty, like in ListMembers.
func (s *Service) ListGroupMembers(ctx context.Context, p app.Principal, groupID string) ([]GroupMemberView, error) {
	members, err := s.repo.ListGroupMembers(ctx, p.WorkspaceID, groupID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	users, err := s.dir.Users(ctx, ids)
	if err != nil {
		s.log.WarnContext(ctx, "workspace: user directory unavailable, listing group members without names", "error", err)
		users = nil
	}
	out := make([]GroupMemberView, len(members))
	for i, m := range members {
		out[i] = GroupMemberView{GroupMember: m, DisplayName: users[m.UserID].DisplayName}
	}
	return out, nil
}

// ReplaceGroupMembers makes members the whole set of a group (WSP-09). Every
// user must be an active member of the workspace. It holds the workspace
// lock that removing and deactivating members also take, so nobody can
// leave the workspace between the check and the write.
func (s *Service) ReplaceGroupMembers(ctx context.Context, p app.Principal, groupID string, members []GroupMember) error {
	if err := requireGroupManager(p); err != nil {
		return err
	}
	if len(members) > maxGroupMembers {
		return fmt.Errorf("a group has at most %d members: %w", maxGroupMembers, app.ErrInvalid)
	}
	ids := make([]string, len(members))
	for i, m := range members {
		u, err := uuid.Parse(m.UserID)
		if err != nil {
			return fmt.Errorf("user_id %q is not a user ID: %w", m.UserID, app.ErrInvalid)
		}
		// Canonical form, so the comparison with the stored IDs below holds.
		members[i].UserID = u.String()
		ids[i] = u.String()
		if slices.Contains(ids[:i], ids[i]) {
			return fmt.Errorf("user %s is listed twice: %w", ids[i], app.ErrInvalid)
		}
	}
	return s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.LockWorkspace(ctx, p.WorkspaceID); err != nil {
			return err
		}
		if _, err := s.repo.GetGroup(ctx, p.WorkspaceID, groupID); err != nil {
			return err
		}
		active, err := s.repo.ActiveMemberIDs(ctx, p.WorkspaceID, ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !slices.Contains(active, id) {
				return fmt.Errorf("user %s is not an active member of the workspace: %w", id, app.ErrInvalid)
			}
		}
		if err := s.repo.ReplaceGroupMembers(ctx, p.WorkspaceID, groupID, members, s.now().UTC()); err != nil {
			return err
		}
		return s.record(ctx, p, domain.AuditGroupMembersSet, domain.TargetGroup, groupID, nil)
	})
}

// ManagersOf is the workspace.GroupDirectory behaviour (WSP-09, APR-02).
func (s *Service) ManagersOf(ctx context.Context, workspaceID, userID string) ([]string, error) {
	return s.repo.ManagersOf(ctx, workspaceID, userID)
}
