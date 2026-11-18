package httpadapter_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	dbmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// testPool starts one Postgres container for the package and migrates the
// helpdesk set. The handlers run on the real repository so the SQL is covered
// by the same requests.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	dbOnce.Do(func() {
		ctx := context.Background()
		c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("test"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
		)
		if err != nil {
			dbErr = fmt.Sprintf("cannot start postgres container: %v", err)
			return
		}
		url, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			dbErr = err.Error()
			return
		}
		db, err := sql.Open("pgx", url)
		if err != nil {
			dbErr = err.Error()
			return
		}
		defer func() { _ = db.Close() }()
		if err := dbmigrations.Run(ctx, db, "helpdesk", migrations.FS); err != nil {
			dbErr = err.Error()
			return
		}
		dbURL = url
	})
	if dbErr != "" {
		t.Fatal(dbErr)
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	if id, ok := f[token]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

// fakeRoles is the workspace membership: user → role, per workspace.
type fakeRoles map[string]app.Role

func (f fakeRoles) Role(_ context.Context, workspaceID, userID string) (app.Role, error) {
	if r, ok := f[workspaceID+"/"+userID]; ok {
		return r, nil
	}
	return "", fmt.Errorf("not a member: %w", app.ErrForbidden)
}

// rolePerms grants helpdesk.admin to owners and admins, as the workspace
// module does.
type rolePerms struct{}

func (rolePerms) Can(p app.Principal, action string) bool {
	return action == usecase.ActionAdmin && (p.Role == app.RoleOwner || p.Role == app.RoleAdmin)
}

// activeMembers answers Members from the same membership table.
type activeMembers struct{ roles fakeRoles }

func (m activeMembers) Active(ctx context.Context, workspaceID, userID string) bool {
	_, err := m.roles.Role(ctx, workspaceID, userID)
	return err == nil
}

// fakeStore presigns without a network and knows which objects exist.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string]int64
}

func (f *fakeStore) PresignPut(_ context.Context, key, _ string, _ int64, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/put/" + key, Method: "PUT", Headers: map[string]string{}, ExpiresAt: time.Now().Add(ttl)}, nil
}

func (f *fakeStore) PresignGet(_ context.Context, key, _ string, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/get/" + key, Method: "GET", ExpiresAt: time.Now().Add(ttl)}, nil
}

func (f *fakeStore) Stat(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if size, ok := f.objects[key]; ok {
		return size, nil
	}
	return 0, fmt.Errorf("object: %w", app.ErrNotFound)
}

type env struct {
	t     *testing.T
	store *fakeStore
	api   humatest.TestAPI
	ws    string
	admin string
	bob   string // a member
	carol string // a member
	out   string // not a member
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testPool(t)
	e := &env{t: t, ws: newID(), admin: newID(), bob: newID(), carol: newID(), out: newID()}
	roles := fakeRoles{
		e.ws + "/" + e.admin: app.RoleAdmin, e.ws + "/" + e.bob: app.RoleMember, e.ws + "/" + e.carol: app.RoleMember,
	}
	services := app.NewServices()
	app.Provide[app.Authenticator](services, fakeAuth{
		"admin": {UserID: e.admin, DeviceID: "dev-a"}, "bob": {UserID: e.bob, DeviceID: "dev-b"},
		"carol": {UserID: e.carol, DeviceID: "dev-c"}, "out": {UserID: e.out, DeviceID: "dev-x"},
	})
	app.Provide[app.MembershipResolver](services, roles)

	e.store = &fakeStore{objects: map[string]int64{}}
	uc := usecase.New(usecase.Deps{
		Tx: database.NewTxManager(pool), Repo: postgres.New(pool), Permissions: rolePerms{}, Members: activeMembers{roles},
		Storage: e.store, Log: slog.New(slog.DiscardHandler),
	})
	_, api := humatest.New(t)
	// The module mounts its routes under /helpdesk; the group does the same here.
	httpadapter.New(uc, slog.New(slog.DiscardHandler)).Register(huma.NewGroup(api, "/helpdesk"), services)
	e.api = api
	return e
}

// headers returns the auth and workspace headers of one user.
func (e *env) headers(token string) []any {
	return []any{"Authorization: Bearer " + token, "X-Workspace-ID: " + e.ws}
}

// resp is a response from the API under test.
type resp struct{ *httptest.ResponseRecorder }

// json decodes the response body into v.
func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", r.Body.String(), err)
	}
}

// call sends one request as the token's user; body is JSON or nil.
func (e *env) call(method, path, token string, body any) resp {
	e.t.Helper()
	args := e.headers(token)
	if body != nil {
		args = append(args, body)
	}
	switch method {
	case http.MethodGet:
		return resp{e.api.Get(path, args...)}
	case http.MethodPost:
		return resp{e.api.Post(path, args...)}
	case http.MethodPatch:
		return resp{e.api.Patch(path, args...)}
	case http.MethodPut:
		return resp{e.api.Put(path, args...)}
	}
	e.t.Fatalf("unknown method %s", method)
	return resp{}
}

func TestOnlyAdminsCreateQueues(t *testing.T) {
	e := newEnv(t)
	if r := e.call(http.MethodPost, "/helpdesk/queues", "bob", map[string]any{"name": "IT", "prefix": "IT"}); r.Code != http.StatusForbidden {
		t.Fatalf("member creating a queue = %d, want 403", r.Code)
	}
	r := e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "it"})
	if r.Code != http.StatusCreated {
		t.Fatalf("admin creating a queue = %d: %s", r.Code, r.Body.String())
	}
	var q struct {
		ID         string   `json:"id"`
		Prefix     string   `json:"prefix"`
		AgentIDs   []string `json:"agent_ids"`
		Categories []any    `json:"categories"`
	}
	r.json(t, &q)
	if q.Prefix != "IT" || q.ID == "" || q.AgentIDs == nil || q.Categories == nil {
		t.Fatalf("queue = %+v; agent_ids and categories must be arrays", q)
	}
}

func TestPrefixRules(t *testing.T) {
	e := newEnv(t)
	e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT"})
	if r := e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "Again", "prefix": "IT"}); r.Code != http.StatusConflict {
		t.Errorf("duplicate prefix = %d, want 409", r.Code)
	}
	if r := e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "Bad", "prefix": "I"}); r.Code != http.StatusBadRequest {
		t.Errorf("one-letter prefix = %d, want 400", r.Code)
	}
}

func TestMembersListActiveQueuesAndNonMembersAreRefused(t *testing.T) {
	e := newEnv(t)
	e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT"})
	r := e.call(http.MethodGet, "/helpdesk/queues", "bob", nil)
	if r.Code != http.StatusOK {
		t.Fatalf("member listing = %d: %s", r.Code, r.Body.String())
	}
	var list struct {
		Items []struct {
			Prefix string `json:"prefix"`
		} `json:"items"`
	}
	r.json(t, &list)
	if len(list.Items) != 1 || list.Items[0].Prefix != "IT" {
		t.Fatalf("items = %+v", list.Items)
	}
	if r := e.call(http.MethodGet, "/helpdesk/queues", "out", nil); r.Code != http.StatusForbidden {
		t.Errorf("non-member listing = %d, want 403", r.Code)
	}
	if r := e.call(http.MethodGet, "/helpdesk/queues", "", nil); r.Code != http.StatusUnauthorized {
		t.Errorf("no token listing = %d, want 401", r.Code)
	}
}

func TestRenameClearChannelAndArchive(t *testing.T) {
	e := newEnv(t)
	created := e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT", "channel_id": newID()})
	var q struct {
		ID        string  `json:"id"`
		ChannelID *string `json:"channel_id"`
	}
	created.json(t, &q)
	if q.ChannelID == nil {
		t.Fatal("channel_id not stored")
	}

	r := e.call(http.MethodPatch, "/helpdesk/queues/"+q.ID, "admin", map[string]any{"name": "Service desk", "channel_id": nil})
	if r.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", r.Code, r.Body.String())
	}
	var patched struct {
		Name      string  `json:"name"`
		ChannelID *string `json:"channel_id"`
	}
	r.json(t, &patched)
	if patched.Name != "Service desk" || patched.ChannelID != nil {
		t.Fatalf("patched = %+v; channel_id null must clear it", patched)
	}

	if r := e.call(http.MethodPatch, "/helpdesk/queues/"+q.ID, "admin", map[string]any{"archived": true}); r.Code != http.StatusOK {
		t.Fatalf("archive = %d", r.Code)
	}
	if list := e.call(http.MethodGet, "/helpdesk/queues", "bob", nil); strings.Contains(list.Body.String(), q.ID) {
		t.Errorf("archived queue still listed: %s", list.Body.String())
	}
	if r := e.call(http.MethodPatch, "/helpdesk/queues/"+newID(), "admin", map[string]any{"name": "X"}); r.Code != http.StatusNotFound {
		t.Errorf("unknown queue = %d, want 404", r.Code)
	}
}

func TestAgentsAreActiveMembers(t *testing.T) {
	e := newEnv(t)
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT"}).json(t, &q)

	if r := e.call(http.MethodPut, "/helpdesk/queues/"+q.ID+"/agents", "admin", map[string]any{"user_ids": []string{e.carol, e.out}}); r.Code != http.StatusBadRequest {
		t.Errorf("a non-member agent = %d, want 400", r.Code)
	}
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+q.ID+"/agents", "bob", map[string]any{"user_ids": []string{e.carol}}); r.Code != http.StatusForbidden {
		t.Errorf("member setting agents = %d, want 403", r.Code)
	}
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+q.ID+"/agents", "admin", map[string]any{"user_ids": []string{e.carol}}); r.Code != http.StatusNoContent {
		t.Fatalf("set agents = %d: %s", r.Code, r.Body.String())
	}
	var list struct {
		Items []struct {
			ID       string   `json:"id"`
			AgentIDs []string `json:"agent_ids"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/queues", "bob", nil).json(t, &list)
	if len(list.Items) != 1 || len(list.Items[0].AgentIDs) != 1 || list.Items[0].AgentIDs[0] != e.carol {
		t.Fatalf("items = %+v, want carol as the only agent", list.Items)
	}
}

func TestCategoriesAndTheDefault(t *testing.T) {
	e := newEnv(t)
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT"}).json(t, &q)

	first := e.call(http.MethodPost, "/helpdesk/categories", "admin", map[string]any{"queue_id": q.ID, "name": "Laptop", "is_default": true})
	if first.Code != http.StatusCreated {
		t.Fatalf("create category = %d: %s", first.Code, first.Body.String())
	}
	second := e.call(http.MethodPost, "/helpdesk/categories", "admin", map[string]any{"queue_id": q.ID, "name": "Email", "is_default": true})
	if second.Code != http.StatusCreated {
		t.Fatalf("second default = %d: %s", second.Code, second.Body.String())
	}
	var secondCat struct {
		ID string `json:"id"`
	}
	second.json(t, &secondCat)

	var list struct {
		Items []struct {
			Categories []struct {
				ID        string `json:"id"`
				IsDefault bool   `json:"is_default"`
			} `json:"categories"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/queues", "bob", nil).json(t, &list)
	var defaults []string
	for _, c := range list.Items[0].Categories {
		if c.IsDefault {
			defaults = append(defaults, c.ID)
		}
	}
	if len(defaults) != 1 || defaults[0] != secondCat.ID {
		t.Fatalf("defaults = %v, want only the latest one", defaults)
	}

	if r := e.call(http.MethodPost, "/helpdesk/categories", "admin", map[string]any{"queue_id": newID(), "name": "Ghost"}); r.Code != http.StatusBadRequest {
		t.Errorf("unknown queue = %d, want 400", r.Code)
	}
	if r := e.call(http.MethodPatch, "/helpdesk/categories/"+secondCat.ID, "admin", map[string]any{"archived": true}); r.Code != http.StatusOK {
		t.Fatalf("archive = %d", r.Code)
	}
	e.call(http.MethodGet, "/helpdesk/queues", "bob", nil).json(t, &list)
	if n := len(list.Items[0].Categories); n != 1 {
		t.Errorf("categories after archiving the default = %d, want 1", n)
	}
	if r := e.call(http.MethodPatch, "/helpdesk/categories/"+secondCat.ID, "bob", map[string]any{"name": "X"}); r.Code != http.StatusForbidden {
		t.Errorf("member editing a category = %d, want 403", r.Code)
	}
}

// laptopCategory makes an IT queue with a Laptop category and returns the
// category's ID.
func (e *env) laptopCategory(t *testing.T) string {
	t.Helper()
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", "admin", map[string]any{"name": "IT", "prefix": "IT"}).json(t, &q)
	var c struct {
		ID string `json:"id"`
	}
	r := e.call(http.MethodPost, "/helpdesk/categories", "admin", map[string]any{"queue_id": q.ID, "name": "Laptop"})
	if r.Code != http.StatusCreated {
		t.Fatalf("create category = %d: %s", r.Code, r.Body.String())
	}
	r.json(t, &c)
	return c.ID
}

func TestMembersCreateTicketsWithAKeyAndStatusNew(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)

	r := e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Printer jams"})
	if r.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", r.Code, r.Body.String())
	}
	var tk struct {
		Key         string  `json:"key"`
		Number      int64   `json:"number"`
		Status      string  `json:"status"`
		Priority    string  `json:"priority"`
		RequesterID string  `json:"requester_id"`
		AssigneeID  *string `json:"assignee_id"`
		Description string  `json:"description"`
	}
	r.json(t, &tk)
	if tk.Key != "IT-0001" || tk.Number != 1 || tk.Status != "new" || tk.Priority != "normal" {
		t.Errorf("ticket = %+v", tk)
	}
	if tk.RequesterID != e.bob || tk.AssigneeID != nil || tk.Description != "" {
		t.Errorf("ticket = %+v; the requester is the caller and nobody is assigned", tk)
	}

	second := e.call(http.MethodPost, "/helpdesk/tickets", "carol", map[string]any{"category_id": cat, "title": "VPN", "priority": "urgent"})
	second.json(t, &tk)
	if tk.Key != "IT-0002" || tk.Priority != "urgent" {
		t.Errorf("second ticket = %+v, want IT-0002 with urgent priority", tk)
	}
}

func TestCreateRefusesBadInput(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)
	cases := []struct {
		name  string
		token string
		body  map[string]any
		want  int
	}{
		{"no category", "bob", map[string]any{"title": "x"}, http.StatusBadRequest},
		{"blank title", "bob", map[string]any{"category_id": cat, "title": " "}, http.StatusBadRequest},
		{"unknown priority", "bob", map[string]any{"category_id": cat, "title": "x", "priority": "critical"}, http.StatusBadRequest},
		{"source message without chat", "bob", map[string]any{"category_id": cat, "title": "x", "source_message_id": newID()}, http.StatusBadRequest},
		{"not a member", "out", map[string]any{"category_id": cat, "title": "x"}, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if r := e.call(http.MethodPost, "/helpdesk/tickets", c.token, c.body); r.Code != c.want {
				t.Errorf("= %d, want %d: %s", r.Code, c.want, r.Body.String())
			}
		})
	}
}

func TestListAndDetailFollowTheVisibilityRules(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)
	var tk struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Printer jams"}).json(t, &tk)
	if tk.ID == "" {
		t.Fatal("ticket not created")
	}
	// carol becomes the agent of the IT queue.
	var queues struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/queues", "admin", nil).json(t, &queues)
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+queues.Items[0].ID+"/agents", "admin", map[string]any{"user_ids": []string{e.carol}}); r.Code != http.StatusNoContent {
		t.Fatalf("set agents = %d", r.Code)
	}

	var page struct {
		Items []struct {
			Key string `json:"key"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	e.call(http.MethodGet, "/helpdesk/tickets", "carol", nil).json(t, &page)
	if len(page.Items) != 1 || page.Items[0].Key != "IT-0001" || page.NextCursor != "" {
		t.Fatalf("agent's list = %+v", page)
	}
	if r := e.call(http.MethodGet, "/helpdesk/tickets?status=bogus", "carol", nil); r.Code != http.StatusBadRequest {
		t.Errorf("bad status filter = %d, want 400", r.Code)
	}
	if r := e.call(http.MethodGet, "/helpdesk/tickets?cursor=nope", "carol", nil); r.Code != http.StatusBadRequest {
		t.Errorf("bad cursor = %d, want 400", r.Code)
	}

	// Bob is the requester of IT-0001, so his own list shows it.
	e.call(http.MethodGet, "/helpdesk/tickets", "bob", nil).json(t, &page)
	if len(page.Items) != 1 {
		t.Errorf("requester's list = %+v", page)
	}

	var detail struct {
		Key     string `json:"key"`
		History []struct {
			Action string `json:"action"`
		} `json:"history"`
	}
	r := e.call(http.MethodGet, "/helpdesk/tickets/"+tk.ID, "bob", nil)
	if r.Code != http.StatusOK {
		t.Fatalf("requester's detail = %d: %s", r.Code, r.Body.String())
	}
	r.json(t, &detail)
	if detail.Key != "IT-0001" || len(detail.History) != 1 || detail.History[0].Action != "created" {
		t.Errorf("detail = %+v", detail)
	}
	if r := e.call(http.MethodGet, "/helpdesk/tickets/"+tk.ID, "out", nil); r.Code != http.StatusForbidden {
		t.Errorf("stranger's detail = %d, want 403 (not a member)", r.Code)
	}
	if r := e.call(http.MethodGet, "/helpdesk/tickets/"+newID(), "admin", nil); r.Code != http.StatusNotFound {
		t.Errorf("missing ticket = %d, want 404", r.Code)
	}
}

func TestClaimMoveAndEditOverHTTP(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)
	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/queues", "admin", nil).json(t, &list)
	queue := list.Items[0].ID
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+queue+"/agents", "admin", map[string]any{"user_ids": []string{e.carol}}); r.Code != http.StatusNoContent {
		t.Fatalf("set agents = %d", r.Code)
	}
	var tk struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Printer jams"}).json(t, &tk)
	base := "/helpdesk/tickets/" + tk.ID

	if r := e.call(http.MethodPost, base+"/assign", "bob", map[string]any{"assignee_id": e.bob}); r.Code != http.StatusForbidden {
		t.Errorf("requester claiming = %d, want 403", r.Code)
	}
	var claimed struct {
		Status     string  `json:"status"`
		AssigneeID *string `json:"assignee_id"`
	}
	r := e.call(http.MethodPost, base+"/assign", "carol", map[string]any{"assignee_id": e.carol})
	if r.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", r.Code, r.Body.String())
	}
	r.json(t, &claimed)
	if claimed.Status != "open" || claimed.AssigneeID == nil || *claimed.AssigneeID != e.carol {
		t.Fatalf("claimed = %+v", claimed)
	}
	if r := e.call(http.MethodPost, base+"/status", "carol", map[string]any{"status": "in_progress"}); r.Code != http.StatusOK {
		t.Fatalf("start work = %d: %s", r.Code, r.Body.String())
	}
	if r := e.call(http.MethodPost, base+"/status", "bob", map[string]any{"status": "waiting"}); r.Code != http.StatusForbidden {
		t.Errorf("requester putting it on hold = %d, want 403", r.Code)
	}
	if r := e.call(http.MethodPost, base+"/status", "carol", map[string]any{"status": "closed"}); r.Code != http.StatusConflict {
		t.Errorf("in_progress -> closed = %d, want 409", r.Code)
	}
	if r := e.call(http.MethodPost, base+"/status", "carol", map[string]any{"status": "bogus"}); r.Code != http.StatusBadRequest {
		t.Errorf("bogus status = %d, want 400", r.Code)
	}
	if r := e.call(http.MethodPatch, base, "bob", map[string]any{"priority": "high"}); r.Code != http.StatusForbidden {
		t.Errorf("requester editing = %d, want 403", r.Code)
	}
	var edited struct {
		Priority string `json:"priority"`
	}
	e.call(http.MethodPatch, base, "carol", map[string]any{"priority": "high"}).json(t, &edited)
	if edited.Priority != "high" {
		t.Errorf("priority = %q", edited.Priority)
	}
	if r := e.call(http.MethodPost, base+"/status", "carol", map[string]any{"status": "resolved"}); r.Code != http.StatusOK {
		t.Fatalf("resolve = %d", r.Code)
	}
	if r := e.call(http.MethodPost, base+"/status", "bob", map[string]any{"status": "closed"}); r.Code != http.StatusOK {
		t.Fatalf("requester closes = %d: %s", r.Code, r.Body.String())
	}
	if r := e.call(http.MethodPost, base+"/status", "bob", map[string]any{"status": "open"}); r.Code != http.StatusConflict {
		t.Errorf("reopen a closed ticket = %d, want 409", r.Code)
	}
	if r := e.call(http.MethodPost, base+"/assign", "outside-member-token", map[string]any{"assignee_id": e.carol}); r.Code != http.StatusUnauthorized {
		t.Errorf("unknown token = %d, want 401", r.Code)
	}

	var detail struct {
		History []struct {
			Action string `json:"action"`
		} `json:"history"`
	}
	e.call(http.MethodGet, base, "bob", nil).json(t, &detail)
	actions := map[string]bool{}
	for _, h := range detail.History {
		actions[h.Action] = true
	}
	for _, want := range []string{"created", "assigned", "status_changed", "priority_changed"} {
		if !actions[want] {
			t.Errorf("history has no %s line: %+v", want, detail.History)
		}
	}
}

func TestCommentsAndTheirVisibilityOverHTTP(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)
	var tk struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Printer jams"}).json(t, &tk)
	base := "/helpdesk/tickets/" + tk.ID + "/comments"

	if r := e.call(http.MethodPost, base, "bob", map[string]any{"body": "Still jams"}); r.Code != http.StatusCreated {
		t.Fatalf("public comment = %d: %s", r.Code, r.Body.String())
	}
	if r := e.call(http.MethodPost, base, "bob", map[string]any{"body": "secret", "internal": true}); r.Code != http.StatusForbidden {
		t.Errorf("requester internal = %d, want 403", r.Code)
	}
	if r := e.call(http.MethodPost, base, "bob", map[string]any{"body": "  "}); r.Code != http.StatusBadRequest {
		t.Errorf("blank comment = %d, want 400", r.Code)
	}
	if r := e.call(http.MethodPost, base, "carol", map[string]any{"body": "Who?"}); r.Code != http.StatusNotFound {
		t.Errorf("a member who may not see the ticket = %d, want 404", r.Code)
	}

	// Make carol the agent, then she may write an internal note.
	var queues struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/queues", "admin", nil).json(t, &queues)
	e.call(http.MethodPut, "/helpdesk/queues/"+queues.Items[0].ID+"/agents", "admin", map[string]any{"user_ids": []string{e.carol}})
	if r := e.call(http.MethodPost, base, "carol", map[string]any{"body": "Check the driver", "internal": true}); r.Code != http.StatusCreated {
		t.Fatalf("agent internal = %d: %s", r.Code, r.Body.String())
	}

	var asRequester struct {
		Comments []struct {
			Body     string `json:"body"`
			Internal bool   `json:"internal"`
		} `json:"comments"`
		History []struct {
			Internal bool `json:"internal"`
		} `json:"history"`
	}
	e.call(http.MethodGet, "/helpdesk/tickets/"+tk.ID, "bob", nil).json(t, &asRequester)
	if len(asRequester.Comments) != 1 || asRequester.Comments[0].Body != "Still jams" {
		t.Errorf("requester's comments = %+v", asRequester.Comments)
	}
	for _, h := range asRequester.History {
		if h.Internal {
			t.Errorf("requester sees an internal history line")
		}
	}
	var asAgent struct {
		Comments []struct {
			Internal bool `json:"internal"`
		} `json:"comments"`
	}
	e.call(http.MethodGet, "/helpdesk/tickets/"+tk.ID, "carol", nil).json(t, &asAgent)
	if len(asAgent.Comments) != 2 {
		t.Errorf("agent's comments = %+v, want both", asAgent.Comments)
	}
}

func TestUploadAttachAndDownloadOverHTTP(t *testing.T) {
	e := newEnv(t)
	cat := e.laptopCategory(t)

	r := e.call(http.MethodPost, "/helpdesk/uploads", "bob", map[string]any{"file_name": "log.txt", "size": 12, "mime_type": "text/plain"})
	if r.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", r.Code, r.Body.String())
	}
	var up struct {
		AttachmentID string `json:"attachment_id"`
		Upload       struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"upload"`
	}
	r.json(t, &up)
	if up.AttachmentID == "" || up.Upload.Method != "PUT" || !strings.Contains(up.Upload.URL, "helpdesk/"+e.ws+"/"+up.AttachmentID) {
		t.Fatalf("upload = %+v", up)
	}
	if r := e.call(http.MethodPost, "/helpdesk/uploads", "bob", map[string]any{"file_name": "x", "size": 0, "mime_type": "text/plain"}); r.Code != http.StatusBadRequest {
		t.Errorf("empty file = %d, want 400", r.Code)
	}

	// The client puts the bytes into storage; the fake store learns of them.
	e.store.objects["helpdesk/"+e.ws+"/"+up.AttachmentID] = 12

	var tk struct {
		ID string `json:"id"`
	}
	r = e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Logs", "attachment_ids": []string{up.AttachmentID}})
	if r.Code != http.StatusCreated {
		t.Fatalf("create with a file = %d: %s", r.Code, r.Body.String())
	}
	r.json(t, &tk)

	var detail struct {
		Attachments []struct {
			ID       string `json:"id"`
			FileName string `json:"file_name"`
		} `json:"attachments"`
	}
	e.call(http.MethodGet, "/helpdesk/tickets/"+tk.ID, "bob", nil).json(t, &detail)
	if len(detail.Attachments) != 1 || detail.Attachments[0].FileName != "log.txt" {
		t.Fatalf("attachments = %+v", detail.Attachments)
	}
	fileID := detail.Attachments[0].ID

	if r := e.call(http.MethodGet, "/helpdesk/attachments/"+fileID, "bob", nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "store.test/get/") {
		t.Errorf("requester's download = %d: %s", r.Code, r.Body.String())
	}
	if r := e.call(http.MethodGet, "/helpdesk/attachments/"+fileID, "carol", nil); r.Code != http.StatusNotFound {
		t.Errorf("member who cannot see the ticket = %d, want 404", r.Code)
	}
	if r := e.call(http.MethodGet, "/helpdesk/attachments/"+fileID, "out", nil); r.Code != http.StatusForbidden {
		t.Errorf("non-member = %d, want 403", r.Code)
	}
	if r := e.call(http.MethodPost, "/helpdesk/tickets", "bob", map[string]any{"category_id": cat, "title": "Again", "attachment_ids": []string{up.AttachmentID}}); r.Code != http.StatusBadRequest {
		t.Errorf("reusing the file = %d, want 400", r.Code)
	}
}
