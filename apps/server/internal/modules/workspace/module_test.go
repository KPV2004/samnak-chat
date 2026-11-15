package workspace_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

type memberBody struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	JoinedAt    string `json:"joined_at"`
}

type memberList struct {
	Items      []memberBody `json:"items"`
	NextCursor string       `json:"next_cursor"`
}

func TestCreateWorkspaceMakesCreatorOwner(t *testing.T) {
	e := newEnv(t)
	alice := e.newUser("alice")

	slug := uniqueSlug("acme")
	r := e.call("POST", "/workspace", alice.Token, "", map[string]string{"name": "  Acme  ", "slug": slug})
	if r.Code != 201 {
		t.Fatalf("status = %d %s", r.Code, r.Body)
	}
	var w workspaceBody
	r.json(t, &w)
	if w.Name != "Acme" || w.Slug != slug || w.Role != "owner" || w.ID == "" {
		t.Fatalf("workspace = %+v", w)
	}

	list := e.call("GET", "/workspace", alice.Token, "", nil)
	var got struct {
		Items []workspaceBody `json:"items"`
	}
	list.json(t, &got)
	if list.Code != 200 || len(got.Items) != 1 || got.Items[0] != w {
		t.Fatalf("list = %d %s", list.Code, list.Body)
	}

	bob := e.newUser("bob")
	other := e.call("GET", "/workspace", bob.Token, "", nil)
	if other.Code != 200 || !strings.Contains(string(other.Body), `"items":[]`) {
		t.Fatalf("bob's list = %d %s", other.Code, other.Body)
	}
}

func TestCreateWorkspaceValidation(t *testing.T) {
	e := newEnv(t)
	alice := e.newUser("alice")
	taken := e.createWorkspace(alice, "First")

	cases := []struct {
		name string
		body map[string]string
		code int
		err  string
	}{
		{"slug too short", map[string]string{"name": "X", "slug": "ab"}, 400, "invalid"},
		{"slug upper case", map[string]string{"name": "X", "slug": "Acme-Team"}, 400, "invalid"},
		{"slug with space", map[string]string{"name": "X", "slug": "acme team"}, 400, "invalid"},
		{"empty name", map[string]string{"name": "  ", "slug": uniqueSlug("v")}, 400, "invalid"},
		{"duplicate slug", map[string]string{"name": "X", "slug": taken.Slug}, 409, "conflict"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := e.call("POST", "/workspace", alice.Token, "", c.body)
			if r.Code != c.code || r.errCode(t) != c.err {
				t.Fatalf("got %d %s", r.Code, r.Body)
			}
		})
	}
	if r := e.call("POST", "/workspace", "", "", map[string]string{"name": "X", "slug": "abc"}); r.Code != 401 {
		t.Fatalf("no token = %d", r.Code)
	}
}

func TestListMembersUsesDirectoryAndPaginates(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser("owner")
	ws := e.createWorkspace(owner, "Team")
	var others []user
	for i := range 4 {
		u := e.newUser(fmt.Sprintf("m%d", i))
		others = append(others, u)
		e.seedMember(ws, u, app.RoleMember, "active")
	}

	r := e.call("GET", "/workspace/members?limit=2", owner.Token, ws.ID, nil)
	var page1 memberList
	r.json(t, &page1)
	if r.Code != 200 || len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("page 1 = %d %s", r.Code, r.Body)
	}

	seen := map[string]memberBody{}
	for _, m := range page1.Items {
		seen[m.UserID] = m
	}
	cursor := page1.NextCursor
	for pages := 0; cursor != ""; pages++ {
		if pages > 5 {
			t.Fatal("pagination does not end")
		}
		var next memberList
		pr := e.call("GET", "/workspace/members?limit=2&cursor="+cursor, owner.Token, ws.ID, nil)
		pr.json(t, &next)
		if pr.Code != 200 {
			t.Fatalf("next page = %d %s", pr.Code, pr.Body)
		}
		for _, m := range next.Items {
			if _, dup := seen[m.UserID]; dup {
				t.Fatalf("duplicate member %s across pages", m.UserID)
			}
			seen[m.UserID] = m
		}
		cursor = next.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d members, want 5", len(seen))
	}
	if got := seen[owner.ID]; got.Role != "owner" || got.Email != owner.Email || got.DisplayName != "owner" || got.Status != "active" || got.JoinedAt == "" {
		t.Fatalf("owner = %+v", got)
	}

	if r := e.call("GET", "/workspace/members?cursor=%25%25bad", owner.Token, ws.ID, nil); r.Code != 400 {
		t.Fatalf("bad cursor = %d %s", r.Code, r.Body)
	}
	// A plain member may list members.
	if r := e.call("GET", "/workspace/members", others[0].Token, ws.ID, nil); r.Code != 200 {
		t.Fatalf("member list = %d", r.Code)
	}
}

func TestListMembersFallsBackWhenDirectoryFails(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser("owner")
	ws := e.createWorkspace(owner, "Team")
	e.dir.err = fmt.Errorf("auth is down")

	r := e.call("GET", "/workspace/members", owner.Token, ws.ID, nil)
	var l memberList
	r.json(t, &l)
	if r.Code != 200 || len(l.Items) != 1 || l.Items[0].Email != "" || l.Items[0].DisplayName != "" || l.Items[0].UserID != owner.ID {
		t.Fatalf("got %d %s", r.Code, r.Body)
	}
}

func TestMembersRequireAMemberOfThatWorkspace(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.newUser("alice"), e.newUser("bob")
	wsA, wsB := e.createWorkspace(alice, "A"), e.createWorkspace(bob, "B")

	// NFR-06: a user of workspace A gets 403 on workspace B.
	if r := e.call("GET", "/workspace/members", alice.Token, wsB.ID, nil); r.Code != 403 {
		t.Fatalf("list B as alice = %d %s", r.Code, r.Body)
	}
	if r := e.call("PATCH", "/workspace/members/"+bob.ID, alice.Token, wsB.ID, map[string]string{"role": "guest"}); r.Code != 403 {
		t.Fatalf("patch B as alice = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/members/"+bob.ID, alice.Token, wsB.ID, nil); r.Code != 403 {
		t.Fatalf("delete B as alice = %d", r.Code)
	}
	// With A's header the request is scoped to A: bob is not a member there.
	if r := e.call("PATCH", "/workspace/members/"+bob.ID, alice.Token, wsA.ID, map[string]string{"role": "guest"}); r.Code != 404 {
		t.Fatalf("patch bob in A = %d %s", r.Code, r.Body)
	}
	if r := e.call("DELETE", "/workspace/members/"+bob.ID, alice.Token, wsA.ID, nil); r.Code != 404 {
		t.Fatalf("delete bob in A = %d", r.Code)
	}
	var role string
	if err := e.pool.QueryRow(t.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, wsB.ID, bob.ID).Scan(&role); err != nil || role != "owner" {
		t.Fatalf("bob's membership in B changed: %q %v", role, err)
	}
	// Missing and malformed workspace headers.
	if r := e.call("GET", "/workspace/members", alice.Token, "", nil); r.Code != 400 {
		t.Fatalf("no header = %d", r.Code)
	}
	if r := e.call("GET", "/workspace/members", alice.Token, "not-a-uuid", nil); r.Code != 403 {
		t.Fatalf("malformed header = %d %s", r.Code, r.Body)
	}
	if r := e.call("GET", "/workspace/members", "", wsA.ID, nil); r.Code != 401 {
		t.Fatalf("no token = %d", r.Code)
	}
}

func TestUpdateMemberRules(t *testing.T) {
	e := newEnv(t)
	owner, admin, member, guest := e.newUser("owner"), e.newUser("admin"), e.newUser("member"), e.newUser("guest")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")
	e.seedMember(ws, member, app.RoleMember, "active")
	e.seedMember(ws, guest, app.RoleGuest, "active")

	patch := func(actor, target user, body map[string]string) resp {
		return e.call("PATCH", "/workspace/members/"+target.ID, actor.Token, ws.ID, body)
	}

	r := patch(admin, member, map[string]string{"role": "manager"})
	var m memberBody
	r.json(t, &m)
	if r.Code != 200 || m.Role != "manager" || m.UserID != member.ID || m.Email != member.Email || m.Status != "active" {
		t.Fatalf("admin promotes member = %d %s", r.Code, r.Body)
	}
	if r := patch(admin, member, map[string]string{"role": "owner"}); r.Code != 403 {
		t.Fatalf("admin grants owner = %d", r.Code)
	}
	if r := patch(admin, owner, map[string]string{"role": "member"}); r.Code != 403 {
		t.Fatalf("admin demotes owner = %d", r.Code)
	}
	if r := patch(member, guest, map[string]string{"role": "member"}); r.Code != 403 {
		t.Fatalf("member manages = %d", r.Code)
	}
	if r := patch(guest, member, map[string]string{"status": "deactivated"}); r.Code != 403 {
		t.Fatalf("guest manages = %d", r.Code)
	}
	if r := patch(owner, member, map[string]string{"role": "boss"}); r.Code != 400 {
		t.Fatalf("unknown role = %d %s", r.Code, r.Body)
	}
	if r := patch(owner, member, map[string]string{}); r.Code != 400 {
		t.Fatalf("empty patch = %d", r.Code)
	}
	if r := patch(owner, member, map[string]string{"status": "gone"}); r.Code != 400 {
		t.Fatalf("unknown status = %d", r.Code)
	}
	// Owner grants owner, then the new owner and the old one can demote each other.
	if r := patch(owner, admin, map[string]string{"role": "owner"}); r.Code != 200 {
		t.Fatalf("owner grants owner = %d %s", r.Code, r.Body)
	}
	if r := patch(admin, owner, map[string]string{"role": "admin"}); r.Code != 200 {
		t.Fatalf("second owner demotes first = %d %s", r.Code, r.Body)
	}
}

func TestLastOwnerStays(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser("owner")
	ws := e.createWorkspace(owner, "Team")
	self := "/workspace/members/" + owner.ID

	for name, body := range map[string]map[string]string{
		"demote":     {"role": "admin"},
		"deactivate": {"status": "deactivated"},
	} {
		if r := e.call("PATCH", self, owner.Token, ws.ID, body); r.Code != 409 || r.errCode(t) != "conflict" {
			t.Fatalf("%s last owner = %d %s", name, r.Code, r.Body)
		}
	}
	if r := e.call("DELETE", self, owner.Token, ws.ID, nil); r.Code != 409 {
		t.Fatalf("remove last owner = %d", r.Code)
	}
	if len(e.events.all()) != 0 {
		t.Fatalf("events published for rejected changes: %v", e.events.all())
	}
}

func TestConcurrentDemotionsKeepAnOwner(t *testing.T) {
	e := newEnv(t)
	a, b := e.newUser("a"), e.newUser("b")
	ws := e.createWorkspace(a, "Team")
	e.seedMember(ws, b, app.RoleOwner, "active")

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, pair := range [][2]user{{a, b}, {b, a}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.call("PATCH", "/workspace/members/"+pair[1].ID, pair[0].Token, ws.ID, map[string]string{"role": "admin"}).Code
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case 409, 403: // the loser may already be an admin when its turn comes
			conflict++
		}
	}
	var owners int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND role='owner' AND status='active'`, ws.ID).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners < 1 || ok != 1 || conflict != 1 {
		t.Fatalf("codes = %v, owners left = %d", codes, owners)
	}
}

func TestDeactivateCutsAccessAndPublishesEvent(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.newUser("owner"), e.newUser("bob")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, bob, app.RoleMember, "active")

	if r := e.call("GET", "/workspace/members", bob.Token, ws.ID, nil); r.Code != 200 {
		t.Fatalf("before = %d", r.Code)
	}
	r := e.call("PATCH", "/workspace/members/"+bob.ID, owner.Token, ws.ID, map[string]string{"status": "deactivated"})
	var m memberBody
	r.json(t, &m)
	if r.Code != 200 || m.Status != "deactivated" {
		t.Fatalf("deactivate = %d %s", r.Code, r.Body)
	}
	// WSP-08: the very next request is refused, and the workspace leaves his list.
	if r := e.call("GET", "/workspace/members", bob.Token, ws.ID, nil); r.Code != 403 {
		t.Fatalf("after = %d", r.Code)
	}
	var list struct {
		Items []workspaceBody `json:"items"`
	}
	e.call("GET", "/workspace", bob.Token, "", nil).json(t, &list)
	if len(list.Items) != 0 {
		t.Fatalf("deactivated member still lists the workspace: %+v", list.Items)
	}
	evts := e.events.all()
	if len(evts) != 1 {
		t.Fatalf("events = %v", evts)
	}
	got, ok := evts[0].(workspace.MemberDeactivated)
	if !ok || got.WorkspaceID != ws.ID || got.UserID != bob.ID || got.ActorID != owner.ID || got.EventName() != "workspace.member_deactivated" {
		t.Fatalf("event = %#v", evts[0])
	}
	// Reactivating restores access and publishes nothing.
	if r := e.call("PATCH", "/workspace/members/"+bob.ID, owner.Token, ws.ID, map[string]string{"status": "active"}); r.Code != 200 {
		t.Fatalf("reactivate = %d", r.Code)
	}
	if r := e.call("GET", "/workspace/members", bob.Token, ws.ID, nil); r.Code != 200 {
		t.Fatalf("after reactivate = %d", r.Code)
	}
	if len(e.events.all()) != 1 {
		t.Fatalf("events after reactivate = %v", e.events.all())
	}
}

func TestRemoveMember(t *testing.T) {
	e := newEnv(t)
	owner, admin, bob := e.newUser("owner"), e.newUser("admin"), e.newUser("bob")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")
	e.seedMember(ws, bob, app.RoleMember, "active")

	if r := e.call("DELETE", "/workspace/members/"+owner.ID, admin.Token, ws.ID, nil); r.Code != 403 {
		t.Fatalf("admin removes owner = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/members/"+admin.ID, bob.Token, ws.ID, nil); r.Code != 403 {
		t.Fatalf("member removes admin = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/members/"+bob.ID, admin.Token, ws.ID, nil); r.Code != 204 {
		t.Fatalf("admin removes member = %d %s", r.Code, r.Body)
	}
	if r := e.call("GET", "/workspace/members", bob.Token, ws.ID, nil); r.Code != 403 {
		t.Fatalf("removed member = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/members/"+bob.ID, admin.Token, ws.ID, nil); r.Code != 404 {
		t.Fatalf("remove twice = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/members/not-a-uuid", admin.Token, ws.ID, nil); r.Code != 404 {
		t.Fatalf("malformed id = %d", r.Code)
	}
	evts := e.events.all()
	if len(evts) != 1 {
		t.Fatalf("events = %v", evts)
	}
	got, ok := evts[0].(workspace.MemberRemoved)
	if !ok || got.WorkspaceID != ws.ID || got.UserID != bob.ID || got.ActorID != admin.ID || got.EventName() != "workspace.member_removed" {
		t.Fatalf("event = %#v", evts[0])
	}
}

func TestMembershipResolverAndAuthorizerAreProvided(t *testing.T) {
	e := newEnv(t)
	owner, bob, eve := e.newUser("owner"), e.newUser("bob"), e.newUser("eve")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, bob, app.RoleMember, "deactivated")

	res, ok := app.Lookup[app.MembershipResolver](e.svc)
	if !ok {
		t.Fatal("MembershipResolver not provided")
	}
	if role, err := res.Role(t.Context(), ws.ID, owner.ID); err != nil || role != app.RoleOwner {
		t.Fatalf("owner role = %q, %v", role, err)
	}
	for name, uid := range map[string]string{"deactivated": bob.ID, "stranger": eve.ID} {
		if _, err := res.Role(t.Context(), ws.ID, uid); !isForbidden(err) {
			t.Fatalf("%s: err = %v, want ErrForbidden", name, err)
		}
	}
	if _, err := res.Role(t.Context(), "not-a-uuid", owner.ID); !isForbidden(err) {
		t.Fatalf("malformed workspace: err = %v", err)
	}

	authz, ok := app.Lookup[workspace.Authorizer](e.svc)
	if !ok {
		t.Fatal("Authorizer not provided")
	}
	p := app.Principal{UserID: owner.ID, WorkspaceID: ws.ID, Role: app.RoleOwner}
	if !authz.Can(p, workspace.ActionMemberManage) {
		t.Fatal("owner cannot manage members")
	}
}
