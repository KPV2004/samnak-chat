package auth_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	authmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const testSecret = "test-secret-test-secret-test-secret"

var (
	pgOnce sync.Once
	pgPool *pgxpool.Pool
	pgErr  error
	pgURL  string
	// The container lives until the test binary exits; Ryuk removes it.
)

// testPool returns a pool on a Postgres container shared by every test of the
// package, with the auth migrations applied. Tests isolate themselves with
// unique emails.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	pgOnce.Do(func() {
		ctx := context.Background()
		c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
		)
		if err != nil {
			pgErr = fmt.Errorf("start postgres: %w", err)
			return
		}
		pgURL, pgErr = c.ConnectionString(ctx, "sslmode=disable")
		if pgErr != nil {
			return
		}
		db, err := sql.Open("pgx", pgURL)
		if err != nil {
			pgErr = err
			return
		}
		defer func() { _ = db.Close() }()
		if pgErr = migrations.Run(ctx, db, "auth", authmigrations.FS); pgErr != nil {
			return
		}
		pgPool, pgErr = database.Open(ctx, pgURL)
	})
	if pgErr != nil {
		t.Skipf("skipping integration test: %v", pgErr)
	}
	return pgPool
}

// clock is a settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock { return &clock{now: time.Now().UTC().Truncate(time.Second)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// env is one auth module served through a Huma API.
type env struct {
	t        *testing.T
	h        http.Handler
	clock    *clock
	services *app.Services
	pool     *pgxpool.Pool
}

func newEnv(t *testing.T, opts ...auth.Option) *env {
	t.Helper()
	pool := testPool(t)
	clk := newClock()
	mod := auth.New(append([]auth.Option{auth.WithClock(clk.Now)}, opts...)...)

	r := chi.NewRouter()
	api := httpx.NewAPI(r, "test")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := &app.Deps{
		Logger:        logger,
		Services:      app.NewServices(),
		Bus:           eventbus.New(logger),
		Notifications: app.NoopNotificationRegistry{},
		Tx:            database.NewTxManager(pool),
		Platform:      map[string]any{"db": pool, "jwt_secret": testSecret},
	}
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, h: r, clock: clk, services: deps.Services, pool: pool}
}

var emailSeq atomic.Int64

// uniqueEmail keeps tests that share the database apart.
func uniqueEmail() string {
	return fmt.Sprintf("user%d-%d@example.com", time.Now().UnixNano(), emailSeq.Add(1))
}

type resp struct {
	Status int
	Body   []byte
}

func (r resp) JSON() map[string]any {
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		panic(fmt.Sprintf("response is not a JSON object: %q", r.Body))
	}
	return m
}

func (r resp) String(path ...string) string {
	var cur any = r.JSON()
	for _, p := range path {
		cur = cur.(map[string]any)[p]
	}
	s, _ := cur.(string)
	return s
}

type req struct {
	Token string
	IP    string
	Body  any
}

func (e *env) do(method, path string, in req) resp {
	e.t.Helper()
	var body io.Reader
	if in.Body != nil {
		b, err := json.Marshal(in.Body)
		if err != nil {
			e.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, body)
	if in.Body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if in.Token != "" {
		r.Header.Set("Authorization", "Bearer "+in.Token)
	}
	ip := in.IP
	if ip == "" {
		ip = "192.0.2.1"
	}
	r.RemoteAddr = ip + ":4321"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{Status: rec.Code, Body: rec.Body.Bytes()}
}

type account struct {
	Email, Password string
}

// register creates an account and returns it.
func (e *env) register(name string) account {
	e.t.Helper()
	a := account{Email: uniqueEmail(), Password: "correct horse battery"}
	r := e.do("POST", "/auth/register", req{Body: map[string]any{
		"email": a.Email, "password": a.Password, "display_name": name,
	}})
	if r.Status != 201 {
		e.t.Fatalf("register: %d %s", r.Status, r.Body)
	}
	return a
}

// session is the result of a login.
type session struct {
	Access, Refresh, DeviceID string
}

func (e *env) login(a account, device string) session {
	e.t.Helper()
	r := e.do("POST", "/auth/login", req{Body: map[string]any{
		"email": a.Email, "password": a.Password, "device_name": device, "platform": "web",
	}})
	if r.Status != 200 {
		e.t.Fatalf("login: %d %s", r.Status, r.Body)
	}
	return session{Access: r.String("access_token"), Refresh: r.String("refresh_token"), DeviceID: r.String("device_id")}
}
