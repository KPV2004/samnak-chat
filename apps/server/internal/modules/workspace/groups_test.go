package workspace_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

type groupBody struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	MemberCount int      `json:"member_count"`
	ManagerIDs  []string `json:"manager_ids"`
}

type groupList struct {
	Items []groupBody `json:"items"`
}

type groupMemberBody struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	IsManager   bool   `json:"is_manager"`
}

type groupMemberList struct {
	Items []groupMemberBody `json:"items"`
}

type putMember struct {
	UserID    string `json:"user_id"`
	IsManager bool   `json:"is_manager"`
}

// groupTeam is a workspace with an owner, an admin, a manager-role user and
// two plain members, all active.
type groupTeam struct {
	*env
	ws                        workspaceBody
	owner, admin, mgr, bob, c user
}

func newGroupTeam(t *testing.T) groupTeam {
	t.Helper()
	e := newEnv(t)
	g := groupTeam{env: e, owner: e.newUser("owner"), admin: e.newUser("admin"), mgr: e.newUser("mgr"), bob: e.newUser("bob"), c: e.newUser("carol")}
	g.ws = e.createWorkspace(g.owner, "Team")
	e.seedMember(g.ws, g.admin, app.RoleAdmin, "active")
	e.seedMember(g.ws, g.mgr, app.RoleManager, "active")
	e.seedMember(g.ws, g.bob, app.RoleMember, "active")
	e.seedMember(g.ws, g.c, app.RoleGuest, "active")
	return g
}

func (g groupTeam) create(actor user, name string) (resp, groupBody) {
	g.t.Helper()
	r := g.call("POST", "/workspace/groups", actor.Token, g.ws.ID, map[string]string{"name": name})
	var out groupBody
	if r.Code == 201 {
		r.json(g.t, &out)
	}
	return r, out
}

func (g groupTeam) put(actor user, groupID string, members ...putMember) resp {
	g.t.Helper()
	if members == nil {
		members = []putMember{}
	}
	return g.call("PUT", "/workspace/groups/"+groupID+"/members", actor.Token, g.ws.ID, map[string]any{"members": members})
}

func TestCreateListRenameDeleteGroup(t *testing.T) {
	g := newGroupTeam(t)

	r, sales := g.create(g.admin, "  Sales  ")
	if r.Code != 201 || sales.ID == "" || sales.Name != "Sales" || sales.MemberCount != 0 || sales.ManagerIDs == nil || len(sales.ManagerIDs) != 0 {
		t.Fatalf("create = %d %s", r.Code, r.Body)
	}
	if r, _ := g.create(g.owner, "Engineering"); r.Code != 201 {
		t.Fatalf("owner creates = %d %s", r.Code, r.Body)
	}

	// Any member, even a guest, lists; groups are ordered by name.
	for _, u := range []user{g.owner, g.bob, g.c} {
		r := g.call("GET", "/workspace/groups", u.Token, g.ws.ID, nil)
		var list groupList
		r.json(t, &list)
		if r.Code != 200 || len(list.Items) != 2 || list.Items[0].Name != "Engineering" || list.Items[1].ID != sales.ID {
			t.Fatalf("list = %d %s", r.Code, r.Body)
		}
	}

	r = g.call("PATCH", "/workspace/groups/"+sales.ID, g.admin.Token, g.ws.ID, map[string]string{"name": " Revenue "})
	var renamed groupBody
	r.json(t, &renamed)
	if r.Code != 200 || renamed.ID != sales.ID || renamed.Name != "Revenue" {
		t.Fatalf("rename = %d %s", r.Code, r.Body)
	}

	if r := g.call("DELETE", "/workspace/groups/"+sales.ID, g.admin.Token, g.ws.ID, nil); r.Code != 204 {
		t.Fatalf("delete = %d %s", r.Code, r.Body)
	}
	if r := g.call("DELETE", "/workspace/groups/"+sales.ID, g.admin.Token, g.ws.ID, nil); r.Code != 404 {
		t.Fatalf("delete twice = %d", r.Code)
	}
	if r := g.call("PATCH", "/workspace/groups/"+sales.ID, g.admin.Token, g.ws.ID, map[string]string{"name": "x"}); r.Code != 404 {
		t.Fatalf("rename deleted = %d", r.Code)
	}
}

func TestGroupManagementNeedsOwnerOrAdmin(t *testing.T) {
	g := newGroupTeam(t)
	_, grp := g.create(g.owner, "Sales")
	body := map[string]string{"name": "Other"}
	for _, u := range []user{g.mgr, g.bob, g.c} {
		for name, r := range map[string]resp{
			"create": g.call("POST", "/workspace/groups", u.Token, g.ws.ID, body),
			"rename": g.call("PATCH", "/workspace/groups/"+grp.ID, u.Token, g.ws.ID, body),
			"delete": g.call("DELETE", "/workspace/groups/"+grp.ID, u.Token, g.ws.ID, nil),
			"put":    g.put(u, grp.ID, putMember{UserID: u.ID}),
		} {
			if r.Code != 403 {
				t.Errorf("%s as %s = %d %s, want 403", name, u.Name, r.Code, r.Body)
			}
		}
	}
	// Nothing changed.
	var list groupList
	g.call("GET", "/workspace/groups", g.owner.Token, g.ws.ID, nil).json(t, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "Sales" || list.Items[0].MemberCount != 0 {
		t.Fatalf("list = %+v", list)
	}

	stranger := g.newUser("stranger")
	for _, r := range []resp{
		g.call("GET", "/workspace/groups", stranger.Token, g.ws.ID, nil),
		g.call("GET", "/workspace/groups/"+grp.ID+"/members", stranger.Token, g.ws.ID, nil),
		g.call("POST", "/workspace/groups", stranger.Token, g.ws.ID, body),
	} {
		if r.Code != 403 {
			t.Errorf("non-member = %d %s, want 403", r.Code, r.Body)
		}
	}
	if r := g.call("GET", "/workspace/groups", "", g.ws.ID, nil); r.Code != 401 {
		t.Errorf("no token = %d", r.Code)
	}
}

func TestGroupNameRules(t *testing.T) {
	g := newGroupTeam(t)
	_, sales := g.create(g.owner, "Sales")
	other := g.newUser("o2")
	otherWS := g.createWorkspace(other, "Other")

	for name, body := range map[string]string{"empty": "", "blank": "   ", "long": strings.Repeat("a", 101)} {
		if r, _ := g.create(g.owner, body); r.Code != 400 {
			t.Errorf("create %s = %d %s, want 400", name, r.Code, r.Body)
		}
		if r := g.call("PATCH", "/workspace/groups/"+sales.ID, g.owner.Token, g.ws.ID, map[string]string{"name": body}); r.Code != 400 {
			t.Errorf("rename %s = %d %s, want 400", name, r.Code, r.Body)
		}
	}
	if r, _ := g.create(g.owner, "SALES"); r.Code != 409 || r.errCode(t) != "conflict" {
		t.Errorf("duplicate ignoring case = %d %s", r.Code, r.Body)
	}
	_, support := g.create(g.owner, "Support")
	if r := g.call("PATCH", "/workspace/groups/"+support.ID, g.owner.Token, g.ws.ID, map[string]string{"name": "sales"}); r.Code != 409 {
		t.Errorf("rename onto a taken name = %d %s", r.Code, r.Body)
	}
	if r := g.call("PATCH", "/workspace/groups/"+support.ID, g.owner.Token, g.ws.ID, map[string]string{"name": "SUPPORT"}); r.Code != 200 {
		t.Errorf("changing only the case of its own name = %d %s", r.Code, r.Body)
	}
	if r := g.call("POST", "/workspace/groups", other.Token, otherWS.ID, map[string]string{"name": "Sales"}); r.Code != 201 {
		t.Errorf("same name in another workspace = %d %s", r.Code, r.Body)
	}
	// A group of another workspace is invisible.
	if r := g.call("DELETE", "/workspace/groups/"+sales.ID, other.Token, otherWS.ID, nil); r.Code != 404 {
		t.Errorf("delete across workspaces = %d", r.Code)
	}
	if r := g.call("GET", "/workspace/groups/"+sales.ID+"/members", other.Token, otherWS.ID, nil); r.Code != 404 {
		t.Errorf("members across workspaces = %d", r.Code)
	}
	if r := g.call("DELETE", "/workspace/groups/not-a-uuid", g.owner.Token, g.ws.ID, nil); r.Code != 404 {
		t.Errorf("malformed id = %d", r.Code)
	}
}

func TestPutGroupMembersReplacesTheSetAndListsNames(t *testing.T) {
	g := newGroupTeam(t)
	_, grp := g.create(g.owner, "Sales")

	r := g.put(g.admin, grp.ID, putMember{UserID: g.bob.ID}, putMember{UserID: g.mgr.ID, IsManager: true}, putMember{UserID: g.c.ID})
	if r.Code != 204 {
		t.Fatalf("put = %d %s", r.Code, r.Body)
	}
	var members groupMemberList
	r = g.call("GET", "/workspace/groups/"+grp.ID+"/members", g.c.Token, g.ws.ID, nil)
	r.json(t, &members)
	if r.Code != 200 || len(members.Items) != 3 {
		t.Fatalf("members = %d %s", r.Code, r.Body)
	}
	byID := map[string]groupMemberBody{}
	for _, m := range members.Items {
		byID[m.UserID] = m
	}
	if m := byID[g.mgr.ID]; !m.IsManager || m.DisplayName != "mgr" {
		t.Fatalf("manager = %+v", m)
	}
	if m := byID[g.bob.ID]; m.IsManager || m.DisplayName != "bob" {
		t.Fatalf("bob = %+v", m)
	}

	var list groupList
	g.call("GET", "/workspace/groups", g.bob.Token, g.ws.ID, nil).json(t, &list)
	if len(list.Items) != 1 || list.Items[0].MemberCount != 3 || !slices.Equal(list.Items[0].ManagerIDs, []string{g.mgr.ID}) {
		t.Fatalf("list = %+v", list)
	}

	// The second PUT replaces: bob out, carol promoted, owner in.
	if r := g.put(g.owner, grp.ID, putMember{UserID: g.mgr.ID, IsManager: true}, putMember{UserID: g.c.ID, IsManager: true}, putMember{UserID: g.owner.ID}); r.Code != 204 {
		t.Fatalf("second put = %d %s", r.Code, r.Body)
	}
	g.call("GET", "/workspace/groups/"+grp.ID+"/members", g.bob.Token, g.ws.ID, nil).json(t, &members)
	got := map[string]bool{}
	for _, m := range members.Items {
		got[m.UserID] = m.IsManager
	}
	if len(got) != 3 || !got[g.mgr.ID] || !got[g.c.ID] || got[g.owner.ID] {
		t.Fatalf("members after replace = %v", got)
	}

	if r := g.put(g.admin, grp.ID); r.Code != 204 {
		t.Fatalf("empty put = %d %s", r.Code, r.Body)
	}
	g.call("GET", "/workspace/groups/"+grp.ID+"/members", g.bob.Token, g.ws.ID, nil).json(t, &members)
	if len(members.Items) != 0 || members.Items == nil {
		t.Fatalf("members after empty put = %+v", members)
	}
}

func TestPutGroupMembersRejectsAnythingButActiveMembers(t *testing.T) {
	g := newGroupTeam(t)
	_, grp := g.create(g.owner, "Sales")
	if r := g.put(g.owner, grp.ID, putMember{UserID: g.bob.ID}); r.Code != 204 {
		t.Fatal(r.Code)
	}
	off, stranger, otherWS := g.newUser("off"), g.newUser("stranger"), g.newUser("o2")
	g.seedMember(g.ws, off, app.RoleMember, "deactivated")
	g.createWorkspace(otherWS, "Other")

	cases := map[string][]putMember{
		"deactivated member": {{UserID: g.bob.ID}, {UserID: off.ID}},
		"not a member":       {{UserID: g.bob.ID}, {UserID: stranger.ID}},
		"unknown user":       {{UserID: uuid.Must(uuid.NewV7()).String()}},
		"not a uuid":         {{UserID: "bob"}},
		"duplicate user":     {{UserID: g.bob.ID}, {UserID: g.bob.ID, IsManager: true}},
	}
	for name, members := range cases {
		if r := g.put(g.owner, grp.ID, members...); r.Code != 400 {
			t.Errorf("%s = %d %s, want 400", name, r.Code, r.Body)
		}
	}
	// A rejected PUT changes nothing: the one before it still stands.
	var got groupMemberList
	g.call("GET", "/workspace/groups/"+grp.ID+"/members", g.owner.Token, g.ws.ID, nil).json(t, &got)
	if len(got.Items) != 1 || got.Items[0].UserID != g.bob.ID {
		t.Fatalf("members = %+v", got)
	}
	if r := g.put(g.owner, uuid.Must(uuid.NewV7()).String(), putMember{UserID: g.bob.ID}); r.Code != 404 {
		t.Errorf("unknown group = %d", r.Code)
	}
}

func TestRemovingAMemberRemovesThemFromGroups(t *testing.T) {
	g := newGroupTeam(t)
	_, a := g.create(g.owner, "Alpha")
	_, b := g.create(g.owner, "Beta")
	for _, grp := range []groupBody{a, b} {
		if r := g.put(g.owner, grp.ID, putMember{UserID: g.bob.ID, IsManager: true}, putMember{UserID: g.c.ID}); r.Code != 204 {
			t.Fatal(r.Code, r.Body)
		}
	}
	if r := g.call("DELETE", "/workspace/members/"+g.bob.ID, g.admin.Token, g.ws.ID, nil); r.Code != 204 {
		t.Fatalf("remove = %d %s", r.Code, r.Body)
	}
	var list groupList
	g.call("GET", "/workspace/groups", g.owner.Token, g.ws.ID, nil).json(t, &list)
	for _, grp := range list.Items {
		if grp.MemberCount != 1 || len(grp.ManagerIDs) != 0 {
			t.Fatalf("group %s = %+v, want only carol", grp.Name, grp)
		}
	}
	var n int
	if err := g.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_group_members WHERE user_id = $1`, g.bob.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("bob still in %d groups, %v", n, err)
	}
}

func TestGroupDirectoryManagersOf(t *testing.T) {
	g := newGroupTeam(t)
	dir, ok := app.Lookup[workspace.GroupDirectory](g.svc)
	if !ok {
		t.Fatal("GroupDirectory not provided")
	}
	ctx := t.Context()
	if got, err := dir.ManagersOf(ctx, g.ws.ID, g.bob.ID); err != nil || len(got) != 0 {
		t.Fatalf("in no group: %v, %v", got, err)
	}
	_, zeta := g.create(g.owner, "Zeta")
	_, alpha := g.create(g.owner, "Alpha")
	if r := g.put(g.owner, zeta.ID, putMember{UserID: g.bob.ID}, putMember{UserID: g.mgr.ID, IsManager: true}); r.Code != 204 {
		t.Fatal(r.Code)
	}
	if r := g.put(g.owner, alpha.ID, putMember{UserID: g.bob.ID}, putMember{UserID: g.admin.ID, IsManager: true}, putMember{UserID: g.mgr.ID, IsManager: true}); r.Code != 204 {
		t.Fatal(r.Code)
	}
	// Alpha comes first; the manager of both groups appears once.
	got, err := dir.ManagersOf(ctx, g.ws.ID, g.bob.ID)
	if err != nil || len(got) != 2 || got[0] == got[1] || !slices.Contains(got, g.admin.ID) || !slices.Contains(got, g.mgr.ID) {
		t.Fatalf("ManagersOf(bob) = %v, %v", got, err)
	}
	// A manager is not their own default approver.
	got, err = dir.ManagersOf(ctx, g.ws.ID, g.mgr.ID)
	if err != nil || !slices.Equal(got, []string{g.admin.ID}) {
		t.Fatalf("ManagersOf(mgr) = %v, %v", got, err)
	}
}
