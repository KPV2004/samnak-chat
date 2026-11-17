package workspace_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
)

// repoEnv is a workspace with an owner, talking to the repository directly.
type repoEnv struct {
	*env
	repo *postgres.Repo
	ws   workspaceBody
}

func newRepoEnv(t *testing.T) repoEnv {
	t.Helper()
	e := newEnv(t)
	ws := e.createWorkspace(e.newUser("owner"), "Team")
	return repoEnv{env: e, repo: postgres.New(e.pool), ws: ws}
}

func (r repoEnv) group(name string) string {
	r.t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	if err := r.repo.InsertGroup(r.t.Context(), domain.Group{ID: id, WorkspaceID: r.ws.ID, Name: name, CreatedAt: time.Now().UTC()}); err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r repoEnv) member(name string, status string) user {
	r.t.Helper()
	u := r.newUser(name)
	r.seedMember(r.ws, u, app.RoleMember, status)
	return u
}

func TestRepoGroupsListGetAndCounts(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	b, a := r.group("beta"), r.group("Alpha")
	m1, m2, m3 := r.member("m1", "active"), r.member("m2", "active"), r.member("m3", "active")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, a, []usecase.GroupMember{
		{UserID: m1.ID}, {UserID: m2.ID, IsManager: true}, {UserID: m3.ID, IsManager: true},
	}, t0)
	if err != nil {
		t.Fatal(err)
	}
	// m3 became manager before m2: the manager list follows manager_since.
	if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, a, []usecase.GroupMember{
		{UserID: m1.ID}, {UserID: m2.ID, IsManager: true}, {UserID: m3.ID, IsManager: false},
	}, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, a, []usecase.GroupMember{
		{UserID: m1.ID}, {UserID: m2.ID, IsManager: true}, {UserID: m3.ID, IsManager: true},
	}, t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	list, err := r.repo.ListGroups(ctx, r.ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != a || list[1].ID != b {
		t.Fatalf("list = %+v, want Alpha then beta", list)
	}
	if list[0].MemberCount != 3 || !slices.Equal(list[0].ManagerIDs, []string{m2.ID, m3.ID}) {
		t.Fatalf("Alpha = %+v, want 3 members and managers [m2 m3] (m2 kept its manager_since)", list[0])
	}
	if list[1].MemberCount != 0 || list[1].ManagerIDs == nil || len(list[1].ManagerIDs) != 0 {
		t.Fatalf("beta = %+v, want empty and non-nil managers", list[1])
	}
	got, err := r.repo.GetGroup(ctx, r.ws.ID, a)
	if err != nil || got.Name != "Alpha" || got.MemberCount != 3 {
		t.Fatalf("GetGroup = %+v, %v", got, err)
	}
}

func TestRepoGroupNamesAreUniquePerWorkspaceIgnoringCase(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	r.group("Sales")
	err := r.repo.InsertGroup(ctx, domain.Group{ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: r.ws.ID, Name: "SALES", CreatedAt: time.Now()})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("duplicate = %v, want ErrConflict", err)
	}
	other := r.createWorkspace(r.newUser("o2"), "Other")
	if err := r.repo.InsertGroup(ctx, domain.Group{ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: other.ID, Name: "Sales", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("same name in another workspace: %v", err)
	}
	b := r.group("Support")
	if err := r.repo.RenameGroup(ctx, r.ws.ID, b, "sales"); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("rename onto a taken name = %v, want ErrConflict", err)
	}
	if err := r.repo.RenameGroup(ctx, r.ws.ID, b, "Care"); err != nil {
		t.Fatal(err)
	}
}

func TestRepoGroupsAreScopedToTheirWorkspace(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	g := r.group("Sales")
	other := r.createWorkspace(r.newUser("o2"), "Other")
	for name, err := range map[string]error{
		"get":     func() error { _, e := r.repo.GetGroup(ctx, other.ID, g); return e }(),
		"rename":  r.repo.RenameGroup(ctx, other.ID, g, "x"),
		"delete":  r.repo.DeleteGroup(ctx, other.ID, g),
		"members": func() error { _, e := r.repo.ListGroupMembers(ctx, other.ID, g); return e }(),
		"replace": r.repo.ReplaceGroupMembers(ctx, other.ID, g, nil, time.Now()),
		"bad id":  r.repo.DeleteGroup(ctx, r.ws.ID, "not-a-uuid"),
	} {
		if !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := r.repo.GetGroup(ctx, r.ws.ID, g); err != nil {
		t.Fatalf("the group must be untouched: %v", err)
	}
}

func TestRepoReplaceGroupMembersReplacesTheWholeSet(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	g := r.group("Sales")
	a, b, c := r.member("a", "active"), r.member("b", "active"), r.member("c", "active")
	now := time.Now().UTC()
	if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, g, []usecase.GroupMember{{UserID: a.ID, IsManager: true}, {UserID: b.ID}}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, g, []usecase.GroupMember{{UserID: b.ID, IsManager: true}, {UserID: c.ID}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := r.repo.ListGroupMembers(ctx, r.ws.ID, g)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{b.ID: true, c.ID: false}
	if len(got) != len(want) {
		t.Fatalf("members = %+v, want b and c only", got)
	}
	for _, m := range got {
		if mgr, ok := want[m.UserID]; !ok || mgr != m.IsManager {
			t.Fatalf("members = %+v", got)
		}
	}
	if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, g, nil, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.repo.ListGroupMembers(ctx, r.ws.ID, g); len(got) != 0 {
		t.Fatalf("after an empty replace: %+v", got)
	}
}

func TestRepoActiveMemberIDs(t *testing.T) {
	r := newRepoEnv(t)
	active, off, stranger := r.member("a", "active"), r.member("b", "deactivated"), r.newUser("s")
	other := r.createWorkspace(r.newUser("o2"), "Other")
	got, err := r.repo.ActiveMemberIDs(t.Context(), r.ws.ID, []string{active.ID, off.ID, stranger.ID})
	if err != nil || !slices.Equal(got, []string{active.ID}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := r.repo.ActiveMemberIDs(t.Context(), other.ID, []string{active.ID}); err != nil || len(got) != 0 {
		t.Fatalf("other workspace: %v, %v", got, err)
	}
}

func TestRepoManagersOfOrdersDedupesAndSkipsSelfAndInactive(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	zeta, alpha, solo := r.group("Zeta"), r.group("alpha"), r.group("Solo")
	me, m1, m2, m3, gone, nobody := r.member("me", "active"), r.member("m1", "active"), r.member("m2", "active"), r.member("m3", "active"), r.member("gone", "active"), r.member("nobody", "active")
	now := time.Now().UTC()
	set := func(g string, at time.Time, ms ...usecase.GroupMember) {
		t.Helper()
		if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, g, ms, at); err != nil {
			t.Fatal(err)
		}
	}
	// alpha: m2 became manager before m1; Zeta: m3, then m1 again (duplicate); gone is a manager but deactivated.
	set(alpha, now, usecase.GroupMember{UserID: me.ID}, usecase.GroupMember{UserID: m2.ID, IsManager: true})
	set(alpha, now.Add(time.Second), usecase.GroupMember{UserID: me.ID}, usecase.GroupMember{UserID: m2.ID, IsManager: true}, usecase.GroupMember{UserID: m1.ID, IsManager: true}, usecase.GroupMember{UserID: gone.ID, IsManager: true})
	set(zeta, now, usecase.GroupMember{UserID: me.ID, IsManager: true}, usecase.GroupMember{UserID: m3.ID, IsManager: true}, usecase.GroupMember{UserID: m1.ID, IsManager: true})
	set(solo, now, usecase.GroupMember{UserID: nobody.ID})
	if _, err := r.pool.Exec(ctx, `UPDATE workspace_members SET status = 'deactivated' WHERE workspace_id = $1 AND user_id = $2`, r.ws.ID, gone.ID); err != nil {
		t.Fatal(err)
	}

	got, err := r.repo.ManagersOf(ctx, r.ws.ID, me.ID)
	if err != nil {
		t.Fatal(err)
	}
	// alpha: m2, m1 (gone skipped); Zeta: m3, m1 (m1 already listed); me excluded.
	if want := []string{m2.ID, m1.ID, m3.ID}; !slices.Equal(got, want) {
		t.Fatalf("ManagersOf = %v, want %v", got, want)
	}
	if got, err := r.repo.ManagersOf(ctx, r.ws.ID, nobody.ID); err != nil || len(got) != 0 {
		t.Fatalf("a member of a group without managers: %v, %v", got, err)
	}
	if got, err := r.repo.ManagersOf(ctx, r.ws.ID, m1.ID); err != nil || !slices.Equal(got, []string{m2.ID, me.ID, m3.ID}) {
		t.Fatalf("a manager sees the other managers only: %v, %v", got, err)
	}
	other := r.createWorkspace(r.newUser("o2"), "Other")
	if got, err := r.repo.ManagersOf(ctx, other.ID, me.ID); err != nil || len(got) != 0 {
		t.Fatalf("other workspace: %v, %v", got, err)
	}
}

func TestRepoDeleteGroupAndMemberships(t *testing.T) {
	r := newRepoEnv(t)
	ctx := t.Context()
	g1, g2 := r.group("One"), r.group("Two")
	u, v := r.member("u", "active"), r.member("v", "active")
	now := time.Now().UTC()
	both := []usecase.GroupMember{{UserID: u.ID, IsManager: true}, {UserID: v.ID}}
	for _, g := range []string{g1, g2} {
		if err := r.repo.ReplaceGroupMembers(ctx, r.ws.ID, g, both, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.repo.DeleteGroupMemberships(ctx, r.ws.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{g1, g2} {
		got, err := r.repo.ListGroupMembers(ctx, r.ws.ID, g)
		if err != nil || len(got) != 1 || got[0].UserID != v.ID {
			t.Fatalf("group %s members = %+v, %v", g, got, err)
		}
	}
	if err := r.repo.DeleteGroup(ctx, r.ws.ID, g1); err != nil {
		t.Fatal(err)
	}
	if _, err := r.repo.GetGroup(ctx, r.ws.ID, g1); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleted group: %v", err)
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM workspace_group_members WHERE group_id = $1`, g1).Scan(&n); err != nil || n != 0 {
		t.Fatalf("memberships of the deleted group = %d, %v", n, err)
	}
	if err := r.repo.DeleteGroupMemberships(ctx, r.ws.ID, uuid.Must(uuid.NewV7()).String()); err != nil {
		t.Fatalf("removing nothing must not fail: %v", err)
	}
}
