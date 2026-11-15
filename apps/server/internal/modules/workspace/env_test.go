package workspace_test

import (
	"bytes"
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// sharedDB starts one Postgres container for the package and migrates it.
// Tests isolate themselves by creating their own users and workspaces.
func sharedDB(t *testing.T) string {
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
		if err := migrations.Run(ctx, db, "workspace", workspace.Migrations); err != nil {
			dbErr = err.Error()
			return
		}
		dbURL = url
	})
	if dbErr != "" {
		t.Skip("skipping integration test: " + dbErr)
	}
	return dbURL
}

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, tok string) (app.Identity, error) {
	if id, ok := f[tok]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

type fakeDirectory struct {
	mu    sync.Mutex
	users map[string]auth.UserSummary
	err   error
}

func (d *fakeDirectory) Users(_ context.Context, ids []string) ([]auth.UserSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	var out []auth.UserSummary
	for _, id := range ids {
		if u, ok := d.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

type eventLog struct {
	mu   sync.Mutex
	list []app.Event
}

func (l *eventLog) add(_ context.Context, e app.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, e)
	return nil
}

func (l *eventLog) all() []app.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]app.Event(nil), l.list...)
}

// testWriter sends log output to the test log, so a 500 explains itself.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

type user struct {
	ID, Token, Email, Name string
}

type env struct {
	t      *testing.T
	h      http.Handler
	pool   *pgxpool.Pool
	dir    *fakeDirectory
	auth   fakeAuth
	events *eventLog
	svc    *app.Services
}

// newEnv wires the workspace module to a real database through the Registry,
// with a fake Authenticator and a fake auth.Directory in Services.
func newEnv(t *testing.T) *env {
	t.Helper()
	url := sharedDB(t)
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	e := &env{t: t, pool: pool, dir: &fakeDirectory{users: map[string]auth.UserSummary{}}, auth: fakeAuth{}, events: &eventLog{}, svc: app.NewServices()}
	app.Provide[app.Authenticator](e.svc, e.auth)
	app.Provide[auth.Directory](e.svc, e.dir)

	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	bus := eventbus.New(logger)
	bus.Subscribe("workspace.member_removed", e.events.add)
	bus.Subscribe("workspace.member_deactivated", e.events.add)

	mod := workspace.New()
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"workspace"})
	if err != nil {
		t.Fatal(err)
	}
	deps := &app.Deps{
		Logger: logger, Services: e.svc, Bus: bus, Tx: database.NewTxManager(pool),
		Notifications: app.NoopNotificationRegistry{}, Platform: map[string]any{"db": pool},
	}
	r := httpx.NewRouter(logger)
	var startErr error
	r.Route("/api/v1", func(v1 chi.Router) {
		startErr = reg.Start(mods, deps, httpx.NewAPI(v1, "test"))
	})
	if startErr != nil {
		t.Fatal(startErr)
	}
	e.h = r
	return e
}

func (e *env) newUser(name string) user {
	e.t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	u := user{ID: id, Token: "tok-" + id, Email: name + "-" + id[len(id)-6:] + "@example.com", Name: name}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.dir.mu.Lock()
	e.dir.users[id] = auth.UserSummary{ID: id, Email: u.Email, DisplayName: name}
	e.dir.mu.Unlock()
	return u
}

type resp struct {
	Code int
	Body []byte
}

func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
}

func (r resp) errCode(t *testing.T) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	r.json(t, &b)
	return b.Error.Code
}

// call sends a request to /api/v1<path>. token and ws may be empty.
func (e *env) call(method, path, token, ws string, body any) resp {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ws != "" {
		req.Header.Set("X-Workspace-ID", ws)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{Code: rec.Code, Body: rec.Body.Bytes()}
}

type workspaceBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

var slugSeq struct {
	sync.Mutex
	n int
}

func uniqueSlug(prefix string) string {
	slugSeq.Lock()
	defer slugSeq.Unlock()
	slugSeq.n++
	return fmt.Sprintf("%s-%s", prefix, uuid.Must(uuid.NewV7()).String()[24:32]) + fmt.Sprint(slugSeq.n%10)
}

// createWorkspace creates a workspace through the API as owner.
func (e *env) createWorkspace(owner user, name string) workspaceBody {
	e.t.Helper()
	r := e.call("POST", "/workspace", owner.Token, "", map[string]string{"name": name, "slug": uniqueSlug("t")})
	if r.Code != 201 {
		e.t.Fatalf("create workspace = %d %s", r.Code, r.Body)
	}
	var w workspaceBody
	r.json(e.t, &w)
	return w
}

// seedMember inserts a membership directly, as a stand-in for the invite flow.
func (e *env) seedMember(ws workspaceBody, u user, role app.Role, status string) {
	e.t.Helper()
	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO workspace_members (workspace_id, user_id, role, status) VALUES ($1, $2, $3, $4)`,
		ws.ID, u.ID, string(role), status)
	if err != nil {
		e.t.Fatal(err)
	}
}
