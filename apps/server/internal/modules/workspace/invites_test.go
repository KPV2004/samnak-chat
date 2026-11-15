package workspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type inviteBody struct {
	ID        string  `json:"id"`
	Email     *string `json:"email"`
	Role      string  `json:"role"`
	ExpiresAt string  `json:"expires_at"`
	CreatedAt string  `json:"created_at"`
	Token     string  `json:"token"`
}

func (e *env) createInvite(actor user, ws workspaceBody, body map[string]any) (resp, inviteBody) {
	e.t.Helper()
	r := e.call("POST", "/workspace/invites", actor.Token, ws.ID, body)
	var inv inviteBody
	if r.Code == 201 {
		r.json(e.t, &inv)
	}
	return r, inv
}

func (e *env) accept(u user, token string) resp {
	e.t.Helper()
	return e.call("POST", "/workspace/invites/accept", u.Token, "", map[string]string{"token": token})
}

func TestCreateInviteReturnsTokenOnceAndStoresOnlyItsHash(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser("owner")
	ws := e.createWorkspace(owner, "Team")

	r, inv := e.createInvite(owner, ws, map[string]any{"email": "  New.Person@Example.com ", "role": "member"})
	if r.Code != 201 {
		t.Fatalf("create = %d %s", r.Code, r.Body)
	}
	if len(inv.Token) < 43 || inv.Role != "member" || inv.Email == nil || *inv.Email != "new.person@example.com" || inv.ID == "" {
		t.Fatalf("invite = %+v", inv)
	}
	exp, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	created, err := time.Parse(time.RFC3339, inv.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if d := exp.Sub(created); d != 7*24*time.Hour {
		t.Fatalf("expires %v after creation, want 7 days", d)
	}

	sum := sha256.Sum256([]byte(inv.Token))
	var stored string
	if err := e.pool.QueryRow(t.Context(), `SELECT token_hash FROM workspace_invites WHERE id = $1`, inv.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != hex.EncodeToString(sum[:]) || strings.Contains(stored, inv.Token) {
		t.Fatalf("stored token_hash = %q", stored)
	}

	list := e.call("GET", "/workspace/invites", owner.Token, ws.ID, nil)
	if list.Code != 200 || strings.Contains(string(list.Body), inv.Token) || strings.Contains(string(list.Body), `"token"`) {
		t.Fatalf("list leaks the token or fails: %d %s", list.Code, list.Body)
	}
	var items struct {
		Items []inviteBody `json:"items"`
	}
	list.json(t, &items)
	if len(items.Items) != 1 || items.Items[0].ID != inv.ID {
		t.Fatalf("list = %s", list.Body)
	}

	// A link invite has no email.
	r, link := e.createInvite(owner, ws, map[string]any{"role": "guest"})
	if r.Code != 201 || link.Email != nil || !strings.Contains(string(r.Body), `"email":null`) {
		t.Fatalf("link invite = %d %s", r.Code, r.Body)
	}
}

func TestCreateInviteRules(t *testing.T) {
	e := newEnv(t)
	owner, admin, manager, member, guest := e.newUser("owner"), e.newUser("admin"), e.newUser("manager"), e.newUser("member"), e.newUser("guest")
	ws := e.createWorkspace(owner, "Team")
	e.seedMember(ws, admin, app.RoleAdmin, "active")
	e.seedMember(ws, manager, app.RoleManager, "active")
	e.seedMember(ws, member, app.RoleMember, "active")
	e.seedMember(ws, guest, app.RoleGuest, "active")

	if r, _ := e.createInvite(admin, ws, map[string]any{"role": "admin"}); r.Code != 201 {
		t.Fatalf("admin invites admin = %d %s", r.Code, r.Body)
	}
	for name, u := range map[string]user{"manager": manager, "member": member, "guest": guest} {
		if r, _ := e.createInvite(u, ws, map[string]any{"role": "member"}); r.Code != 403 {
			t.Fatalf("%s creates invite = %d", name, r.Code)
		}
		if r := e.call("GET", "/workspace/invites", u.Token, ws.ID, nil); r.Code != 403 {
			t.Fatalf("%s lists invites = %d", name, r.Code)
		}
	}
	bad := map[string]map[string]any{
		"owner role":   {"role": "owner"},
		"manager role": {"role": "manager"},
		"no role":      {},
		"unknown role": {"role": "boss"},
		"bad email":    {"role": "member", "email": "not-an-email"},
	}
	for name, body := range bad {
		if r, _ := e.createInvite(owner, ws, body); r.Code != 400 {
			t.Fatalf("%s = %d %s", name, r.Code, r.Body)
		}
	}
	var n int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_invites WHERE workspace_id = $1 AND role = 'owner'`, ws.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("owner invites stored: %d %v", n, err)
	}
}

func TestAcceptLinkInviteOnce(t *testing.T) {
	e := newEnv(t)
	owner, bob, carol := e.newUser("owner"), e.newUser("bob"), e.newUser("carol")
	ws := e.createWorkspace(owner, "Team")
	_, inv := e.createInvite(owner, ws, map[string]any{"role": "guest"})

	r := e.accept(bob, inv.Token)
	var w workspaceBody
	r.json(t, &w)
	if r.Code != 200 || w.ID != ws.ID || w.Role != "guest" || w.Slug != ws.Slug {
		t.Fatalf("accept = %d %s", r.Code, r.Body)
	}
	if r := e.call("GET", "/workspace/members", bob.Token, ws.ID, nil); r.Code != 200 {
		t.Fatalf("new member lists members = %d", r.Code)
	}
	if r := e.accept(carol, inv.Token); r.Code != 404 {
		t.Fatalf("used invite = %d %s", r.Code, r.Body)
	}
	if left := e.call("GET", "/workspace/invites", owner.Token, ws.ID, nil); strings.Contains(string(left.Body), inv.ID) {
		t.Fatalf("used invite still listed: %s", left.Body)
	}
	if r := e.accept(carol, "no-such-token"); r.Code != 404 {
		t.Fatalf("unknown token = %d", r.Code)
	}
	if r := e.call("POST", "/workspace/invites/accept", "", "", map[string]string{"token": inv.Token}); r.Code != 401 {
		t.Fatalf("anonymous accept = %d", r.Code)
	}
}

func TestAcceptEmailInviteChecksEmail(t *testing.T) {
	e := newEnv(t)
	owner, bob, carol := e.newUser("owner"), e.newUser("bob"), e.newUser("carol")
	ws := e.createWorkspace(owner, "Team")
	_, inv := e.createInvite(owner, ws, map[string]any{"email": strings.ToUpper(bob.Email), "role": "member"})

	if r := e.accept(carol, inv.Token); r.Code != 403 {
		t.Fatalf("other email = %d %s", r.Code, r.Body)
	}
	// The failed attempt does not burn the invite.
	if r := e.accept(bob, inv.Token); r.Code != 200 {
		t.Fatalf("right email = %d %s", r.Code, r.Body)
	}

	_, inv2 := e.createInvite(owner, ws, map[string]any{"email": carol.Email, "role": "member"})
	e.dir.err = fmt.Errorf("auth is down")
	if r := e.accept(carol, inv2.Token); r.Code != 403 {
		t.Fatalf("email cannot be verified = %d %s", r.Code, r.Body)
	}
}

func TestAcceptRejectsExpiredRevokedAndDuplicateMembers(t *testing.T) {
	e := newEnv(t)
	owner, bob, dave := e.newUser("owner"), e.newUser("bob"), e.newUser("dave")
	ws := e.createWorkspace(owner, "Team")

	_, expired := e.createInvite(owner, ws, map[string]any{"role": "member"})
	if _, err := e.pool.Exec(t.Context(), `UPDATE workspace_invites SET expires_at = now() - interval '1 second' WHERE id = $1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	if r := e.accept(bob, expired.Token); r.Code != 404 {
		t.Fatalf("expired = %d", r.Code)
	}
	if list := e.call("GET", "/workspace/invites", owner.Token, ws.ID, nil); strings.Contains(string(list.Body), expired.ID) {
		t.Fatalf("expired invite listed: %s", list.Body)
	}

	_, revoked := e.createInvite(owner, ws, map[string]any{"role": "member"})
	if r := e.call("DELETE", "/workspace/invites/"+revoked.ID, owner.Token, ws.ID, nil); r.Code != 204 {
		t.Fatalf("revoke = %d %s", r.Code, r.Body)
	}
	if r := e.accept(bob, revoked.Token); r.Code != 404 {
		t.Fatalf("revoked = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/invites/"+revoked.ID, owner.Token, ws.ID, nil); r.Code != 404 {
		t.Fatalf("revoke twice = %d", r.Code)
	}

	// Already a member: 409, and the invite stays usable for someone else.
	_, inv := e.createInvite(owner, ws, map[string]any{"role": "admin"})
	if r := e.accept(owner, inv.Token); r.Code != 409 || r.errCode(t) != "conflict" {
		t.Fatalf("owner accepts own workspace invite = %d %s", r.Code, r.Body)
	}
	e.seedMember(ws, dave, app.RoleMember, "deactivated")
	if r := e.accept(dave, inv.Token); r.Code != 409 {
		t.Fatalf("deactivated member accepts = %d", r.Code)
	}
	if r := e.accept(bob, inv.Token); r.Code != 200 {
		t.Fatalf("invite after rejected attempts = %d %s", r.Code, r.Body)
	}
	var role string
	if err := e.pool.QueryRow(t.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, ws.ID, owner.ID).Scan(&role); err != nil || role != "owner" {
		t.Fatalf("owner's role changed to %q (%v)", role, err)
	}
}

func TestInvitesAreScopedToTheirWorkspace(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.newUser("alice"), e.newUser("bob")
	wsA, wsB := e.createWorkspace(alice, "A"), e.createWorkspace(bob, "B")
	_, invB := e.createInvite(bob, wsB, map[string]any{"role": "member"})

	if r, _ := e.createInvite(alice, wsB, map[string]any{"role": "member"}); r.Code != 403 {
		t.Fatalf("alice creates in B = %d", r.Code)
	}
	if r := e.call("GET", "/workspace/invites", alice.Token, wsB.ID, nil); r.Code != 403 {
		t.Fatalf("alice lists B = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/invites/"+invB.ID, alice.Token, wsB.ID, nil); r.Code != 403 {
		t.Fatalf("alice revokes in B = %d", r.Code)
	}
	// Using A's workspace, B's invite ID is simply unknown.
	if r := e.call("DELETE", "/workspace/invites/"+invB.ID, alice.Token, wsA.ID, nil); r.Code != 404 {
		t.Fatalf("alice revokes B's invite through A = %d", r.Code)
	}
	if r := e.call("DELETE", "/workspace/invites/not-a-uuid", alice.Token, wsA.ID, nil); r.Code != 404 {
		t.Fatalf("malformed invite id = %d", r.Code)
	}
	var revoked bool
	if err := e.pool.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL FROM workspace_invites WHERE id=$1`, invB.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("B's invite was revoked: %v %v", revoked, err)
	}
	if r := e.accept(alice, invB.Token); r.Code != 200 {
		t.Fatalf("alice accepts B's invite = %d %s", r.Code, r.Body)
	}
	// Alice is in both workspaces now and switches by header alone (WSP-05).
	for _, ws := range []workspaceBody{wsA, wsB} {
		if r := e.call("GET", "/workspace/members", alice.Token, ws.ID, nil); r.Code != 200 {
			t.Fatalf("members of %s = %d", ws.Name, r.Code)
		}
	}
}

func TestConcurrentAcceptsOfOneLinkAdmitOnlyOne(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser("owner")
	ws := e.createWorkspace(owner, "Team")
	_, inv := e.createInvite(owner, ws, map[string]any{"role": "member"})
	users := []user{e.newUser("u1"), e.newUser("u2"), e.newUser("u3"), e.newUser("u4")}

	codes := make(chan int, len(users))
	for _, u := range users {
		go func() { codes <- e.accept(u, inv.Token).Code }()
	}
	ok := 0
	for range users {
		if <-codes == 200 {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d users accepted a single-use invite", ok)
	}
}
