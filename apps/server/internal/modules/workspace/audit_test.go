package workspace_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type auditBody struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Data       map[string]any `json:"data"`
	CreatedAt  string         `json:"created_at"`
}

type auditList struct {
	Items      []auditBody `json:"items"`
	NextCursor string      `json:"next_cursor"`
}

// listAudit calls GET /workspace/audit with query (for example "?limit=2")
// as actor in ws.
func (e *env) listAudit(actor user, ws workspaceBody, query string) resp {
	e.t.Helper()
	return e.call("GET", "/workspace/audit"+query, actor.Token, ws.ID, nil)
}

// auditRows reads the first page of the audit log of ws as actor.
func (e *env) auditRows(actor user, ws workspaceBody) []auditBody {
	e.t.Helper()
	r := e.listAudit(actor, ws, "?limit=100")
	if r.Code != 200 {
		e.t.Fatalf("audit = %d %s", r.Code, r.Body)
	}
	var l auditList
	r.json(e.t, &l)
	return l.Items
}

// auditCount counts the audit rows of ws, read straight from the database.
func (e *env) auditCount(ws workspaceBody) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM workspace_audit WHERE workspace_id=$1`, ws.ID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// failAuditWrites makes every audit insert into ws fail, so the change it
// belongs to must roll back with it. The trigger is dropped at cleanup.
func (e *env) failAuditWrites(ws workspaceBody) {
	e.t.Helper()
	fn := "fail_audit_" + strings.ReplaceAll(ws.ID, "-", "")
	stmts := []string{
		fmt.Sprintf(`CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.workspace_id = '%[2]s' THEN
    RAISE EXCEPTION 'audit is down';
  END IF;
  RETURN NEW;
END $$`, fn, ws.ID),
		fmt.Sprintf(`CREATE TRIGGER %[1]s BEFORE INSERT ON workspace_audit FOR EACH ROW EXECUTE FUNCTION %[1]s()`, fn),
	}
	for _, s := range stmts {
		if _, err := e.pool.Exec(context.Background(), s); err != nil {
			e.t.Fatal(err)
		}
	}
	e.t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %[1]s ON workspace_audit`, fn))
		_, _ = e.pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %[1]s()`, fn))
	})
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestEveryAdminActionWritesOneAuditRow(t *testing.T) {
	e := newEnv(t)
	owner, admin, bob, carol := e.newUser("owner"), e.newUser("admin"), e.newUser("bob"), e.newUser("carol")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")
	e.seedMember(ws, bob, app.RoleMember, "active")
	e.seedMember(ws, carol, app.RoleMember, "active")

	r, inv := e.createInvite(admin, ws, map[string]any{"role": "member"})
	if r.Code != 201 {
		t.Fatalf("invite = %d %s", r.Code, r.Body)
	}
	if r := e.call("DELETE", "/workspace/invites/"+inv.ID, admin.Token, ws.ID, nil); r.Code != 204 {
		t.Fatalf("revoke = %d %s", r.Code, r.Body)
	}
	if r := e.call("PATCH", "/workspace/members/"+bob.ID, admin.Token, ws.ID, map[string]string{"role": "manager"}); r.Code != 200 {
		t.Fatalf("role = %d %s", r.Code, r.Body)
	}
	var grp struct {
		ID string `json:"id"`
	}
	r = e.call("POST", "/workspace/groups", admin.Token, ws.ID, map[string]string{"name": "Sales"})
	r.json(t, &grp)
	if r.Code != 201 {
		t.Fatalf("create group = %d %s", r.Code, r.Body)
	}
	if r := e.call("PATCH", "/workspace/groups/"+grp.ID, admin.Token, ws.ID, map[string]string{"name": "Sales EU"}); r.Code != 200 {
		t.Fatalf("rename group = %d %s", r.Code, r.Body)
	}
	if r := e.call("PUT", "/workspace/groups/"+grp.ID+"/members", admin.Token, ws.ID, map[string]any{"members": []map[string]any{{"user_id": bob.ID, "is_manager": true}}}); r.Code != 204 {
		t.Fatalf("set members = %d %s", r.Code, r.Body)
	}
	if r := e.call("DELETE", "/workspace/groups/"+grp.ID, admin.Token, ws.ID, nil); r.Code != 204 {
		t.Fatalf("delete group = %d %s", r.Code, r.Body)
	}
	if r := e.call("PATCH", "/workspace/members/"+bob.ID, admin.Token, ws.ID, map[string]string{"status": "deactivated"}); r.Code != 200 {
		t.Fatalf("deactivate = %d %s", r.Code, r.Body)
	}
	if r := e.call("DELETE", "/workspace/members/"+carol.ID, admin.Token, ws.ID, nil); r.Code != 204 {
		t.Fatalf("remove = %d %s", r.Code, r.Body)
	}

	// Newest first.
	want := []struct {
		action, targetType, targetID string
		data                         map[string]any
	}{
		{"member.removed", "member", carol.ID, map[string]any{}},
		{"member.updated", "member", bob.ID, map[string]any{"active": map[string]any{"from": true, "to": false}}},
		{"group.deleted", "group", grp.ID, map[string]any{}},
		{"group.members_set", "group", grp.ID, map[string]any{}},
		{"group.updated", "group", grp.ID, map[string]any{}},
		{"group.created", "group", grp.ID, map[string]any{}},
		{"member.updated", "member", bob.ID, map[string]any{"role": map[string]any{"from": "member", "to": "manager"}}},
		{"invite.revoked", "invite", inv.ID, map[string]any{}},
		{"invite.created", "invite", inv.ID, map[string]any{}},
	}
	rows := e.auditRows(owner, ws)
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		row := rows[i]
		if row.Action != w.action || row.TargetType != w.targetType || row.TargetID != w.targetID || row.ActorID != admin.ID {
			t.Errorf("row %d = %+v, want %s on %s %s by admin", i, row, w.action, w.targetType, w.targetID)
		}
		if row.ID == "" || row.CreatedAt == "" {
			t.Errorf("row %d has no id or created_at: %+v", i, row)
		}
		if !reflect.DeepEqual(row.Data, w.data) {
			t.Errorf("row %d data = %v, want %v", i, row.Data, w.data)
		}
	}
}

func TestRejectedChangesWriteNoAuditRow(t *testing.T) {
	e := newEnv(t)
	owner, admin := e.newUser("owner"), e.newUser("admin")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")
	if r := e.call("POST", "/workspace/groups", owner.Token, ws.ID, map[string]string{"name": "Sales"}); r.Code != 201 {
		t.Fatalf("create group = %d %s", r.Code, r.Body)
	}
	before := e.auditCount(ws)

	cases := []struct {
		name string
		do   func() resp
	}{
		{"last owner demoted", func() resp {
			return e.call("PATCH", "/workspace/members/"+owner.ID, owner.Token, ws.ID, map[string]string{"role": "admin"})
		}},
		{"admin grants owner", func() resp {
			return e.call("PATCH", "/workspace/members/"+admin.ID, admin.Token, ws.ID, map[string]string{"role": "owner"})
		}},
		{"unknown member removed", func() resp {
			return e.call("DELETE", "/workspace/members/"+uuid.NewString(), owner.Token, ws.ID, nil)
		}},
		{"unknown invite revoked", func() resp {
			return e.call("DELETE", "/workspace/invites/"+uuid.NewString(), owner.Token, ws.ID, nil)
		}},
		{"duplicate group name", func() resp {
			return e.call("POST", "/workspace/groups", owner.Token, ws.ID, map[string]string{"name": "sales"})
		}},
	}
	for _, c := range cases {
		if r := c.do(); r.Code < 400 {
			t.Errorf("%s: status = %d %s, want an error", c.name, r.Code, r.Body)
		}
	}
	if after := e.auditCount(ws); after != before {
		t.Fatalf("audit rows went from %d to %d for rejected changes", before, after)
	}
}

func TestAuditRowRollsBackWithItsChange(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.newUser("owner"), e.newUser("bob")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, bob, app.RoleMember, "active")
	if r := e.call("POST", "/workspace/groups", owner.Token, ws.ID, map[string]string{"name": "Existing"}); r.Code != 201 {
		t.Fatalf("create group = %d %s", r.Code, r.Body)
	}
	var existing string
	if err := e.pool.QueryRow(t.Context(), `SELECT id::text FROM workspace_groups WHERE workspace_id=$1`, ws.ID).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	e.failAuditWrites(ws)

	cases := []struct {
		name string
		do   func() resp
		kept func() bool
	}{
		{
			name: "invite.created",
			do: func() resp {
				r, _ := e.createInvite(owner, ws, map[string]any{"role": "member"})
				return r
			},
			kept: func() bool {
				return e.count(`SELECT count(*) FROM workspace_invites WHERE workspace_id=$1`, ws.ID) == 0
			},
		},
		{
			name: "member.updated",
			do: func() resp {
				return e.call("PATCH", "/workspace/members/"+bob.ID, owner.Token, ws.ID, map[string]string{"role": "guest"})
			},
			kept: func() bool {
				return e.count(`SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND user_id=$2 AND role='member'`, ws.ID, bob.ID) == 1
			},
		},
		{
			name: "member.removed",
			do: func() resp {
				return e.call("DELETE", "/workspace/members/"+bob.ID, owner.Token, ws.ID, nil)
			},
			kept: func() bool {
				return e.count(`SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, ws.ID, bob.ID) == 1
			},
		},
		{
			name: "group.created",
			do: func() resp {
				return e.call("POST", "/workspace/groups", owner.Token, ws.ID, map[string]string{"name": "New"})
			},
			kept: func() bool { return e.count(`SELECT count(*) FROM workspace_groups WHERE workspace_id=$1`, ws.ID) == 1 },
		},
		{
			name: "group.deleted",
			do: func() resp {
				return e.call("DELETE", "/workspace/groups/"+existing, owner.Token, ws.ID, nil)
			},
			kept: func() bool {
				return e.count(`SELECT count(*) FROM workspace_groups WHERE workspace_id=$1 AND id=$2`, ws.ID, existing) == 1
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if r := c.do(); r.Code != 500 {
				t.Fatalf("status = %d %s, want 500 when the audit row fails", r.Code, r.Body)
			}
			if !c.kept() {
				t.Fatal("the change was kept although its audit row was not written")
			}
		})
	}
}

func TestAuditListIsForOwnersAndAdminsOfThatWorkspace(t *testing.T) {
	e := newEnv(t)
	owner, admin, manager, member, guest := e.newUser("owner"), e.newUser("admin"), e.newUser("manager"), e.newUser("member"), e.newUser("guest")
	eve := e.newUser("eve")
	wsA := e.createWorkspace(owner, "A")
	e.seedMember(wsA, admin, app.RoleAdmin, "active")
	e.seedMember(wsA, manager, app.RoleManager, "active")
	e.seedMember(wsA, member, app.RoleMember, "active")
	e.seedMember(wsA, guest, app.RoleGuest, "active")
	wsB := e.createWorkspace(eve, "B")

	invA, _ := e.createInvite(owner, wsA, map[string]any{"role": "member"})
	if invA.Code != 201 {
		t.Fatalf("invite in A = %d", invA.Code)
	}
	var groupB struct {
		ID string `json:"id"`
	}
	r := e.call("POST", "/workspace/groups", eve.Token, wsB.ID, map[string]string{"name": "Secret"})
	r.json(t, &groupB)
	if r.Code != 201 {
		t.Fatalf("group in B = %d %s", r.Code, r.Body)
	}

	for _, u := range []user{owner, admin} {
		if r := e.listAudit(u, wsA, ""); r.Code != 200 {
			t.Fatalf("%s lists A = %d %s", u.Name, r.Code, r.Body)
		}
	}
	for _, u := range []user{manager, member, guest, eve} {
		if r := e.listAudit(u, wsA, ""); r.Code != 403 {
			t.Fatalf("%s lists A = %d %s, want 403", u.Name, r.Code, r.Body)
		}
	}
	// NFR-06 and WSP-06: the owner of A cannot read B through A's log, and
	// B's rows never appear in A's list.
	if r := e.listAudit(owner, wsB, ""); r.Code != 403 {
		t.Fatalf("owner of A lists B = %d %s", r.Code, r.Body)
	}
	if r := e.call("GET", "/workspace/audit", owner.Token, "", nil); r.Code != 400 {
		t.Fatalf("no header = %d", r.Code)
	}
	if r := e.call("GET", "/workspace/audit", "", wsA.ID, nil); r.Code != 401 {
		t.Fatalf("no token = %d", r.Code)
	}

	for _, row := range e.auditRows(owner, wsA) {
		if row.TargetID == groupB.ID {
			t.Fatalf("A's log shows B's group: %+v", row)
		}
	}
	rowsB := e.auditRows(eve, wsB)
	if len(rowsB) != 1 || rowsB[0].Action != "group.created" || rowsB[0].TargetID != groupB.ID {
		t.Fatalf("B's log = %+v", rowsB)
	}
}

func TestAuditPagesAreNewestFirst(t *testing.T) {
	e := newEnv(t)
	owner, admin := e.newUser("owner"), e.newUser("admin")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")

	var created []string
	for range 5 {
		r, inv := e.createInvite(admin, ws, map[string]any{"role": "member"})
		if r.Code != 201 {
			t.Fatalf("invite = %d %s", r.Code, r.Body)
		}
		created = append(created, inv.ID)
	}
	var want []string
	for i := len(created) - 1; i >= 0; i-- {
		want = append(want, created[i])
	}

	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("pagination does not end")
		}
		q := "?limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		r := e.listAudit(owner, ws, q)
		var page auditList
		r.json(t, &page)
		if r.Code != 200 || len(page.Items) > 2 {
			t.Fatalf("page = %d %s", r.Code, r.Body)
		}
		for _, it := range page.Items {
			got = append(got, it.TargetID)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pages gave %v, want %v", got, want)
	}

	if r := e.listAudit(owner, ws, "?cursor=%25%25bad"); r.Code != 400 {
		t.Fatalf("bad cursor = %d %s", r.Code, r.Body)
	}
	if r := e.listAudit(owner, ws, "?limit=101"); r.Code != 400 || r.errCode(t) != "invalid" {
		t.Fatalf("limit 101 = %d %s", r.Code, r.Body)
	}
}
