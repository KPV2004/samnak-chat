package postgres

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
)

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, u := range ids {
		out[i] = u.String()
	}
	return out
}

func toGroupView(g gen.WorkspaceGroup, memberCount int64, managers []uuid.UUID) usecase.GroupView {
	return usecase.GroupView{
		Group:       domain.Group{ID: g.ID.String(), WorkspaceID: g.WorkspaceID.String(), Name: g.Name, CreatedAt: g.CreatedAt.UTC()},
		MemberCount: int(memberCount),
		ManagerIDs:  uuidStrings(managers),
	}
}

func (r *Repo) InsertGroup(ctx context.Context, g domain.Group) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	gid, err := id(g.ID)
	if err != nil {
		return err
	}
	wid, err := id(g.WorkspaceID)
	if err != nil {
		return err
	}
	return wrap(q.InsertGroup(ctx, gen.InsertGroupParams{ID: gid, WorkspaceID: wid, Name: g.Name, CreatedAt: g.CreatedAt.UTC()}), "insert group")
}

func (r *Repo) ListGroups(ctx context.Context, workspaceID string) ([]usecase.GroupView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGroups(ctx, wid)
	if err != nil {
		return nil, wrap(err, "list groups")
	}
	out := make([]usecase.GroupView, len(rows))
	for i, row := range rows {
		out[i] = toGroupView(gen.WorkspaceGroup{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, CreatedAt: row.CreatedAt}, row.MemberCount, row.ManagerIds)
	}
	return out, nil
}

func (r *Repo) GetGroup(ctx context.Context, workspaceID, groupID string) (usecase.GroupView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return usecase.GroupView{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return usecase.GroupView{}, err
	}
	gid, err := id(groupID)
	if err != nil {
		return usecase.GroupView{}, err
	}
	row, err := q.GetGroup(ctx, gen.GetGroupParams{WorkspaceID: wid, ID: gid})
	if err != nil {
		return usecase.GroupView{}, wrap(err, "group")
	}
	return toGroupView(gen.WorkspaceGroup{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, CreatedAt: row.CreatedAt}, row.MemberCount, row.ManagerIds), nil
}

func (r *Repo) RenameGroup(ctx context.Context, workspaceID, groupID, name string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	gid, err := id(groupID)
	if err != nil {
		return err
	}
	n, err := q.RenameGroup(ctx, gen.RenameGroupParams{WorkspaceID: wid, ID: gid, Name: name})
	if err != nil {
		return wrap(err, "rename group")
	}
	if n == 0 {
		return fmt.Errorf("group: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	gid, err := id(groupID)
	if err != nil {
		return err
	}
	n, err := q.DeleteGroup(ctx, gen.DeleteGroupParams{WorkspaceID: wid, ID: gid})
	if err != nil {
		return wrap(err, "delete group")
	}
	if n == 0 {
		return fmt.Errorf("group: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) ListGroupMembers(ctx context.Context, workspaceID, groupID string) ([]usecase.GroupMember, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	gid, err := id(groupID)
	if err != nil {
		return nil, err
	}
	// Reading the group first scopes the member rows to the workspace.
	if _, err := q.GetGroup(ctx, gen.GetGroupParams{WorkspaceID: wid, ID: gid}); err != nil {
		return nil, wrap(err, "group")
	}
	rows, err := q.ListGroupMembers(ctx, gid)
	if err != nil {
		return nil, wrap(err, "list group members")
	}
	out := make([]usecase.GroupMember, len(rows))
	for i, row := range rows {
		out[i] = usecase.GroupMember{UserID: row.UserID.String(), IsManager: row.IsManager}
	}
	return out, nil
}

func (r *Repo) ReplaceGroupMembers(ctx context.Context, workspaceID, groupID string, members []usecase.GroupMember, now time.Time) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	gid, err := id(groupID)
	if err != nil {
		return err
	}
	if _, err := q.LockGroup(ctx, gen.LockGroupParams{WorkspaceID: wid, ID: gid}); err != nil {
		return wrap(err, "group")
	}
	keep := make([]uuid.UUID, len(members))
	for i, m := range members {
		if keep[i], err = id(m.UserID); err != nil {
			return err
		}
	}
	if err := q.DeleteGroupMembersNotIn(ctx, gen.DeleteGroupMembersNotInParams{GroupID: gid, Keep: keep}); err != nil {
		return wrap(err, "clear group members")
	}
	now = now.UTC()
	for i, m := range members {
		err := q.UpsertGroupMember(ctx, gen.UpsertGroupMemberParams{GroupID: gid, UserID: keep[i], IsManager: m.IsManager, Now: now})
		if err != nil {
			return wrap(err, "set group member")
		}
	}
	return nil
}

func (r *Repo) ActiveMemberIDs(ctx context.Context, workspaceID string, ids []string) ([]string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	parsed := make([]uuid.UUID, len(ids))
	for i, s := range ids {
		if parsed[i], err = id(s); err != nil {
			return nil, err
		}
	}
	rows, err := q.FilterActiveMembers(ctx, gen.FilterActiveMembersParams{WorkspaceID: wid, Ids: parsed})
	if err != nil {
		return nil, wrap(err, "filter active members")
	}
	return uuidStrings(rows), nil
}

func (r *Repo) DeleteGroupMemberships(ctx context.Context, workspaceID, userID string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	uid, err := id(userID)
	if err != nil {
		return err
	}
	return wrap(q.DeleteGroupMemberships(ctx, gen.DeleteGroupMembershipsParams{WorkspaceID: wid, UserID: uid}), "delete group memberships")
}

func (r *Repo) ManagersOf(ctx context.Context, workspaceID, userID string) ([]string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ManagersOf(ctx, gen.ManagersOfParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return nil, wrap(err, "managers of")
	}
	out := make([]string, 0, len(rows))
	for _, s := range uuidStrings(rows) {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}
