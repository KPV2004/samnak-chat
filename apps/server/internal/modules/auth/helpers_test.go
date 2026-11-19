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
	bus      *eventbus.Bus
	pool     *pgxpool.Pool
	jobs     *fakeJobs
	mail     *fakeMailer
}

func newEnv(t *testing.T, opts ...auth.Option) *env {
	t.Helper()
	return startEnv(t, true, opts...)
}

// startEnv serves the module with a fake job queue. withMailer false starts it
// with no Mailer, as a server without mail configuration (AUTH-09).
func startEnv(t *testing.T, withMailer bool, opts ...auth.Option) *env {
	t.Helper()
	pool := testPool(t)
	clk := newClock()
	mod := auth.New(append([]auth.Option{auth.WithClock(clk.Now)}, opts...)...)

	r := chi.NewRouter()
	api := httpx.NewAPI(r, "test")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.New(logger)
	e := &env{
		t: t, h: r, clock: clk, bus: bus, pool: pool,
		jobs: &fakeJobs{handlers: map[string]app.JobHandler{}}, mail: &fakeMailer{},
	}
	deps := &app.Deps{
		Logger:        logger,
		Services:      app.NewServices(),
		Bus:           bus,
		Notifications: app.NoopNotificationRegistry{},
		Tx:            database.NewTxManager(pool),
		Jobs:          e.jobs,
		Platform:      map[string]any{"db": pool, "jwt_secret": testSecret},
	}
	if withMailer {
		deps.Mailer = e.mail
	}
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatal(err)
	}
	e.services = deps.Services
	return e
}

// fakeJobs stands in for the River queue. Enqueue records the job; deliver runs
// the queued jobs of a kind once, as a worker would.
type fakeJobs struct {
	mu         sync.Mutex
	handlers   map[string]app.JobHandler
	queued     []queuedJob
	enqueueErr error
}

type queuedJob struct {
	kind string
	args json.RawMessage
}

var _ app.Jobs = (*fakeJobs)(nil)

func (j *fakeJobs) Handle(kind string, h app.JobHandler, _ app.JobOptions) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.handlers[kind] = h
}

func (j *fakeJobs) Every(time.Duration, string, any) {}

func (j *fakeJobs) Enqueue(_ context.Context, kind string, args any, _ app.EnqueueOptions) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.enqueueErr != nil {
		return j.enqueueErr
	}
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	j.queued = append(j.queued, queuedJob{kind: kind, args: b})
	return nil
}

// pending returns how many jobs of kind are waiting to run.
func (j *fakeJobs) pending(kind string) int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, q := range j.queued {
		if q.kind == kind {
			n++
		}
	}
	return n
}

// deliver runs and removes the queued jobs of kind. A handler error fails the
// test, since a worker would retry it.
func (j *fakeJobs) deliver(t *testing.T, kind string) {
	t.Helper()
	j.mu.Lock()
	var run, rest []queuedJob
	for _, q := range j.queued {
		if q.kind == kind {
			run = append(run, q)
		} else {
			rest = append(rest, q)
		}
	}
	j.queued = rest
	h := j.handlers[kind]
	j.mu.Unlock()
	for _, q := range run {
		if err := h(context.Background(), q.args); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

// fakeMailer records the mail it is asked to send.
type fakeMailer struct {
	mu   sync.Mutex
	sent []app.Mail
}

var _ app.Mailer = (*fakeMailer)(nil)

func (m *fakeMailer) Send(_ context.Context, mail app.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mail)
	return nil
}

func (m *fakeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// deliverMail runs the queued auth emails and returns the mails they sent.
func (e *env) deliverMail() []app.Mail {
	e.t.Helper()
	before := e.mail.count()
	e.jobs.deliver(e.t, "auth.send_email")
	e.mail.mu.Lock()
	defer e.mail.mu.Unlock()
	return append([]app.Mail(nil), e.mail.sent[before:]...)
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
