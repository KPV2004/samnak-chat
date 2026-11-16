// Package acceptance boots the real modules, hub and database and proves the
// milestone outcomes of SRS 16.3 end to end.
package acceptance

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/jobs"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/realtime"
	redisclient "github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
)

const testSecret = "acceptance-secret-acceptance-secret"

// moduleNames is what cmd/server enables for M2: MODULES_ENABLED= (set, empty),
// which means core modules only.
var moduleNames = []string{"auth", "workspace", "chat"}

// notifyModuleNames is MODULES_ENABLED unset: the core modules plus the default
// notify module, as cmd/server starts them.
var notifyModuleNames = []string{"auth", "workspace", "chat", "notify"}

var (
	pgURL    string
	redisURL string
)

// TestMain starts the containers once for the package. A test skips with a
// clear message when Docker or -short left the URLs empty.
func TestMain(m *testing.M) {
	flag.Parse()
	var cleanup []func()
	if !testing.Short() && dockerAvailable() {
		cleanup = startContainers()
	}
	code := m.Run()
	for _, fn := range cleanup {
		fn()
	}
	os.Exit(code)
}

func dockerAvailable() bool {
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return true
	}
	return os.Getenv("DOCKER_HOST") != ""
}

func startContainers() []func() {
	ctx := context.Background()
	var cleanup []func()

	pg, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "acceptance: postgres container unavailable, tests will skip: %v\n", err)
	} else {
		cleanup = append(cleanup, func() { _ = testcontainers.TerminateContainer(pg) })
		url, err := pg.ConnectionString(ctx, "sslmode=disable")
		if err == nil {
			err = migrate(url)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "acceptance: postgres is not usable, tests will skip: %v\n", err)
		} else {
			pgURL = url
		}
	}

	rd, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintf(os.Stderr, "acceptance: redis container unavailable, tests will skip: %v\n", err)
	} else {
		cleanup = append(cleanup, func() { _ = testcontainers.TerminateContainer(rd) })
		url, err := rd.ConnectionString(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "acceptance: redis connection string: %v\n", err)
		} else {
			redisURL = url
		}
	}
	return cleanup
}

// migrate applies the platform, River and module migrations, as cmd/migrate
// does. The notify set and River's tables are needed by the M3 instances, which
// run the optional notify module and the job queue.
func migrate(url string) error {
	ctx := context.Background()
	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	pool, err := database.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := migrations.Run(ctx, db, "platform", migrations.Platform); err != nil {
		return fmt.Errorf("platform migrations: %w", err)
	}
	// River's migrator needs a pgx pool; goose works on database/sql.
	if err := migrations.River(ctx, pool); err != nil {
		return fmt.Errorf("river migrations: %w", err)
	}
	sets := []struct {
		owner string
		fsys  fs.FS
	}{
		{"auth", auth.Migrations},
		{"workspace", workspace.Migrations},
		{"chat", chat.Migrations},
		{"notify", notify.Migrations},
	}
	for _, s := range sets {
		if err := migrations.Run(ctx, db, s.owner, s.fsys); err != nil {
			return fmt.Errorf("%s migrations: %w", s.owner, err)
		}
	}
	return nil
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if pgURL == "" {
		t.Skip("skipping acceptance test: Postgres container is unavailable (-short or Docker unavailable)")
	}
	pool, err := database.Open(context.Background(), pgURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testRedis(t *testing.T) *goredis.Client {
	t.Helper()
	if redisURL == "" {
		t.Skip("skipping acceptance test: Redis container is unavailable (-short or Docker unavailable)")
	}
	client, err := redisclient.Open(context.Background(), redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// instance is one server process: the real registry, hub and router over an
// httptest server. Two instances sharing Postgres and Redis are how the
// deployment runs behind a load balancer.
type instance struct {
	t      *testing.T
	url    string
	hub    *realtime.Hub
	bus    *eventbus.Bus
	events chan app.Event
	// jobs and tx are what modules get as Deps.Jobs and Deps.Tx; nil jobs
	// means the instance runs without a queue.
	jobs *jobs.Queue
	tx   app.TxManager
}

// instanceConfig selects the parts of cmd/server a test needs. The zero value
// is the M2 server: core modules, no rule registry and no job queue.
type instanceConfig struct {
	// modules is the equivalent of MODULES_ENABLED; nil means moduleNames.
	modules []string
	// notifications is Deps.Notifications. cmd/server always passes
	// app.NewNotificationRules(); nil keeps M2's noop registry.
	notifications app.NotificationRegistry
	// jobs builds Deps.Jobs from platform/jobs and starts it after the
	// modules, as cmd/server does.
	jobs bool
	// jobHandlers registers extra job kinds before the queue starts, which is
	// when handlers must be known. It stands in for a module's Init.
	jobHandlers func(*jobs.Queue)
	// sweepInterval shortens the no-worker sweep. Zero keeps the default.
	sweepInterval time.Duration
}

func newInstance(t *testing.T, pool *pgxpool.Pool, rdb *goredis.Client) *instance {
	t.Helper()
	return newInstanceWith(t, pool, rdb, instanceConfig{notifications: app.NoopNotificationRegistry{}})
}

// newInstanceWith boots the modules cmd/migrate has migrated and cmd/server
// would enable. The job queue is built before Init (which registers handlers)
// and started after every module has started, the same order as cmd/server.
func newInstanceWith(t *testing.T, pool *pgxpool.Pool, rdb *goredis.Client, cfg instanceConfig) *instance {
	t.Helper()
	logger := testLogger()
	services := app.NewServices()
	// A long idle timeout keeps the test from being closed as idle.
	hub := realtime.NewHub(logger, services, rdb, realtime.WithIdleTimeout(10*time.Minute))

	modulesEnabled := cfg.modules
	if modulesEnabled == nil {
		modulesEnabled = moduleNames
	}
	rules := cfg.notifications
	if rules == nil {
		rules = app.NoopNotificationRegistry{}
	}

	var queue *jobs.Queue
	if cfg.jobs {
		queue = jobs.New(pool, logger)
		if cfg.sweepInterval > 0 {
			queue.SetSweepInterval(cfg.sweepInterval)
		}
	}
	tx := database.NewTxManager(pool)

	bus := eventbus.New(logger)
	deps := &app.Deps{
		Logger:        logger,
		Services:      services,
		Bus:           bus,
		Notifications: rules,
		Tx:            tx,
		Realtime:      hub,
		Platform:      map[string]any{"db": pool, "redis": rdb, "jwt_secret": testSecret},
	}
	// A nil *Queue in the interface would defeat modules' Deps.Jobs == nil check.
	if queue != nil {
		deps.Jobs = queue
	}

	// Every module is available, as in cmd/server's modules slice; the
	// enabled list is what MODULES_ENABLED selects.
	reg := app.NewRegistry(auth.New(), workspace.New(), chat.New(), notify.New())
	mods, err := reg.Enable(modulesEnabled)
	if err != nil {
		t.Fatal(err)
	}

	r := httpx.NewRouter(logger)
	var startErr error
	r.Route("/api/v1", func(v1 chi.Router) {
		api := httpx.NewAPI(v1, "acceptance")
		httpx.RegisterMeta(api, "samnak-chat", "acceptance", modulesEnabled)
		startErr = reg.Start(mods, deps, api)
	})
	if startErr != nil {
		t.Fatal(startErr)
	}
	hub.Mount(r)

	if queue != nil {
		if cfg.jobHandlers != nil {
			cfg.jobHandlers(queue)
		}
		if err := queue.Start(context.Background()); err != nil {
			t.Fatalf("start jobs: %v", err)
		}
		// Stops after the HTTP server closes, like cmd/server.
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := queue.Stop(ctx); err != nil {
				t.Errorf("jobs stop: %v", err)
			}
		})
	}

	// This is the wiring cmd/server does for AUTH-08: a revoked device's
	// sockets close on this instance, or on every instance through Redis.
	events := make(chan app.Event, 64)
	bus.Subscribe(auth.DeviceRevoked{}.EventName(), func(ctx context.Context, evt app.Event) error {
		select {
		case events <- evt:
		default:
		}
		e, ok := evt.(auth.DeviceRevoked)
		if !ok {
			return nil
		}
		if e.DeviceID != "" {
			return hub.DisconnectDevice(ctx, e.DeviceID)
		}
		return hub.DisconnectUser(ctx, e.UserID)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := hub.Close(ctx); err != nil {
			t.Errorf("hub close: %v", err)
		}
	})
	return &instance{t: t, url: srv.URL, hub: hub, bus: bus, events: events, jobs: queue, tx: tx}
}

// testLogger discards logs unless a test asks for them.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// disconnectEvents drains the auth.device_revoked events seen so far.
func (in *instance) disconnectEvents() []auth.DeviceRevoked {
	var out []auth.DeviceRevoked
	for {
		select {
		case evt := <-in.events:
			if e, ok := evt.(auth.DeviceRevoked); ok {
				out = append(out, e)
			}
		default:
			return out
		}
	}
}

// wsURL is the WebSocket endpoint of the server.
func (in *instance) wsURL() string {
	return "ws" + strings.TrimPrefix(in.url, "http") + "/api/v1/ws"
}

// dial opens a socket without authenticating it.
func (in *instance) dial(t *testing.T) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, in.wsURL(), nil)
	if err != nil {
		t.Fatalf("dial %s: %v", in.wsURL(), err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

// connect dials, authenticates and waits for the ready frame.
func (in *instance) connect(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	c := in.dial(t)
	write(t, c, "auth", map[string]string{"access_token": token})
	env := recv(t, c, 5*time.Second)
	if env.Type != "ready" {
		t.Fatalf("first frame after auth = %q, want ready", env.Type)
	}
	return c
}

func write(t *testing.T, c *websocket.Conn, typ string, data any) {
	t.Helper()
	env, err := app.NewEnvelope(typ, data)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		env.Data = nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write %s: %v", typ, err)
	}
}

func recv(t *testing.T, c *websocket.Conn, timeout time.Duration) app.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read after %s: %v", timeout, err)
	}
	var env app.Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("frame %q is not an envelope: %v", b, err)
	}
	return env
}

// recvType reads until an envelope of type want arrives.
func recvType(t *testing.T, c *websocket.Conn, want string, timeout time.Duration) app.Envelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			t.Fatalf("no %q frame within %s", want, timeout)
		}
		env := recv(t, c, left)
		if env.Type == want {
			return env
		}
	}
}

// drainToPong reads and returns every frame queued before the reply to a ping
// this sends now. Frames the server enqueued before this call are ordered
// before the pong, so a duplicate message event shows up here.
func drainToPong(t *testing.T, c *websocket.Conn) []app.Envelope {
	t.Helper()
	write(t, c, "ping", nil)
	var frames []app.Envelope
	for {
		env := recv(t, c, 5*time.Second)
		if env.Type == "pong" {
			return frames
		}
		frames = append(frames, env)
	}
}

// expectClose waits for the server to close the socket and returns its code.
func expectClose(t *testing.T, c *websocket.Conn, timeout time.Duration) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		_, _, err := c.Read(ctx)
		if err == nil {
			continue // drain frames queued before the close
		}
		if ctx.Err() != nil {
			t.Fatalf("connection was not closed within %s: %v", timeout, err)
		}
		code := websocket.CloseStatus(err)
		if code == -1 {
			t.Fatalf("connection ended without a close frame: %v", err)
		}
		return code
	}
}

// client calls the HTTP API of one instance.
type client struct {
	t   *testing.T
	url string
}

type call struct {
	token     string
	workspace string
	body      any
}

type reply struct {
	Status int
	Body   map[string]any
}

func (r reply) str(key string) string {
	s, _ := r.Body[key].(string)
	return s
}

func (r reply) items() []map[string]any {
	raw, _ := r.Body["items"].([]any)
	out := make([]map[string]any, len(raw))
	for i, it := range raw {
		out[i], _ = it.(map[string]any)
	}
	return out
}

func (r reply) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (c client) do(method, path string, in call) reply {
	c.t.Helper()
	req, body := c.request(method, path, in)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	r := reply{Status: res.StatusCode}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r.Body); err != nil {
			c.t.Fatalf("%s %s: response is not a JSON object: %q", method, path, raw)
		}
	}
	return r
}

// status calls the API and returns only the status code, for endpoints whose
// 404 body is chi's plain text rather than JSON.
func (c client) status(method, path string, in call) int {
	c.t.Helper()
	req, body := c.request(method, path, in)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode
}

// request builds an API call. A non-nil body must already be JSON.
func (c client) request(method, path string, in call) (*http.Request, io.Reader) {
	c.t.Helper()
	var body io.Reader
	if in.body != nil {
		b, err := json.Marshal(in.body)
		if err != nil {
			c.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url+"/api/v1"+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if in.token != "" {
		req.Header.Set("Authorization", "Bearer "+in.token)
	}
	if in.workspace != "" {
		req.Header.Set("X-Workspace-ID", in.workspace)
	}
	return req, body
}

func (c client) want(r reply, status int) reply {
	c.t.Helper()
	if r.Status != status {
		c.t.Fatalf("status = %d, want %d: %v", r.Status, status, r.Body)
	}
	return r
}
