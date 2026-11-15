package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

// api is a client for the server under test.
type api struct {
	t   *testing.T
	url string
}

type reply struct {
	Status int
	Body   map[string]any
}

// str reads a top-level string field.
func (r reply) str(key string) string {
	s, _ := r.Body[key].(string)
	return s
}

// items reads the "items" array of a list response.
func (r reply) items() []map[string]any {
	raw, _ := r.Body["items"].([]any)
	out := make([]map[string]any, len(raw))
	for i, it := range raw {
		out[i], _ = it.(map[string]any)
	}
	return out
}

// errCode reads error.code of an error response.
func (r reply) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

type opts struct {
	token     string
	workspace string
	body      any
}

func (a api) do(method, path string, o opts) reply {
	a.t.Helper()
	var body io.Reader
	if o.body != nil {
		b, err := json.Marshal(o.body)
		if err != nil {
			a.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.url+"/api/v1"+path, body)
	if err != nil {
		a.t.Fatal(err)
	}
	if o.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if o.token != "" {
		req.Header.Set("Authorization", "Bearer "+o.token)
	}
	if o.workspace != "" {
		req.Header.Set("X-Workspace-ID", o.workspace)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	r := reply{Status: res.StatusCode}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r.Body); err != nil {
			a.t.Fatalf("%s %s: response is not a JSON object: %q", method, path, raw)
		}
	}
	return r
}

func (a api) want(r reply, status int) reply {
	a.t.Helper()
	if r.Status != status {
		a.t.Fatalf("status = %d, want %d: %v", r.Status, status, r.Body)
	}
	return r
}

// user is a registered and logged-in account.
type user struct {
	id, email, access, refresh string
}

func (a api) signUp(name string) user {
	a.t.Helper()
	email := name + "@example.com"
	reg := a.want(a.do("POST", "/auth/register", opts{body: map[string]any{
		"email": email, "password": "correct-horse-battery", "display_name": name,
	}}), 201)
	login := a.want(a.do("POST", "/auth/login", opts{body: map[string]any{
		"email": email, "password": "correct-horse-battery", "device_name": name + "'s laptop", "platform": "web",
	}}), 200)
	return user{id: reg.str("id"), email: email, access: login.str("access_token"), refresh: login.str("refresh_token")}
}

// TestSRSM1Acceptance runs the M1 acceptance flow of SRS 16.3 against the
// real modules, Postgres and Redis: two users sign up, one creates a workspace
// and invites the other, who joins; a user outside it gets 403; and a rotated
// refresh token cannot be used again.
func TestSRSM1Acceptance(t *testing.T) {
	pool := startPostgres(t)
	rdb := startRedis(t)
	h, err := newHandler(testLogger(), []string{"auth", "workspace"}, []app.Module{auth.New(), workspace.New()}, infra{
		Tx:       database.NewTxManager(pool),
		Platform: map[string]any{"db": pool, "jwt_secret": "test-secret-test-secret-test-secret"},
	}.withRedis(rdb))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := api{t: t, url: srv.URL}

	alice, bob, carol := c.signUp("alice"), c.signUp("bob"), c.signUp("carol")

	// Alice creates a workspace and invites Bob by email.
	ws := c.want(c.do("POST", "/workspace", opts{token: alice.access, body: map[string]any{"name": "Acme", "slug": "acme"}}), 201)
	wsID := ws.str("id")
	if ws.str("role") != "owner" {
		t.Fatalf("creator role = %q, want owner", ws.str("role"))
	}
	invite := c.want(c.do("POST", "/workspace/invites", opts{
		token: alice.access, workspace: wsID, body: map[string]any{"email": bob.email, "role": "member"},
	}), 201)
	token := invite.str("token")
	if token == "" {
		t.Fatal("create invite did not return the token")
	}

	// Bob accepts and sees both members.
	joined := c.want(c.do("POST", "/workspace/invites/accept", opts{token: bob.access, body: map[string]any{"token": token}}), 200)
	if joined.str("id") != wsID || joined.str("role") != "member" {
		t.Fatalf("accepted workspace = %v, want %s as member", joined.Body, wsID)
	}
	members := c.want(c.do("GET", "/workspace/members", opts{token: bob.access, workspace: wsID}), 200).items()
	roles := map[string]string{}
	for _, m := range members {
		roles[m["user_id"].(string)], _ = m["role"].(string)
	}
	if len(roles) != 2 || roles[alice.id] != "owner" || roles[bob.id] != "member" {
		t.Fatalf("members = %v, want alice as owner and bob as member", members)
	}

	// Carol is outside the workspace: every workspace call is 403.
	for _, tc := range []struct{ name, method, path string }{
		{"members", "GET", "/workspace/members"},
		{"invites", "GET", "/workspace/invites"},
		{"create invite", "POST", "/workspace/invites"},
		{"change member", "PATCH", "/workspace/members/" + bob.id},
		{"remove member", "DELETE", "/workspace/members/" + bob.id},
	} {
		body := map[string]any{}
		switch tc.method {
		case "POST":
			body = map[string]any{"role": "member"}
		case "PATCH":
			body = map[string]any{"role": "admin"}
		}
		r := c.do(tc.method, tc.path, opts{token: carol.access, workspace: wsID, body: body})
		if r.Status != 403 || r.errCode() != "forbidden" {
			t.Errorf("outsider %s: status = %d %v, want 403 forbidden", tc.name, r.Status, r.Body)
		}
	}
	// Nothing changed: Bob is still a member.
	if got := len(c.want(c.do("GET", "/workspace/members", opts{token: alice.access, workspace: wsID}), 200).items()); got != 2 {
		t.Fatalf("members after the outsider's calls = %d, want 2", got)
	}

	// A rotated refresh token cannot be used again.
	rotated := c.want(c.do("POST", "/auth/refresh", opts{body: map[string]any{"refresh_token": bob.refresh}}), 200)
	if rotated.str("refresh_token") == "" || rotated.str("refresh_token") == bob.refresh {
		t.Fatal("refresh did not rotate the token")
	}
	if r := c.do("POST", "/auth/refresh", opts{body: map[string]any{"refresh_token": bob.refresh}}); r.Status != 401 {
		t.Fatalf("reused refresh token: status = %d %v, want 401", r.Status, r.Body)
	}
	// The reuse revoked the device, so the new pair is dead too (AUTH-03).
	if r := c.do("POST", "/auth/refresh", opts{body: map[string]any{"refresh_token": rotated.str("refresh_token")}}); r.Status != 401 {
		t.Fatalf("refresh token of a revoked device: status = %d %v, want 401", r.Status, r.Body)
	}
}
