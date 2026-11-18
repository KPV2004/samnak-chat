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

type env struct {
	t     *testing.T
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

	uc := usecase.New(usecase.Deps{
		Tx: database.NewTxManager(pool), Repo: postgres.New(pool), Permissions: rolePerms{}, Members: activeMembers{roles},
		Log: slog.New(slog.DiscardHandler),
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
