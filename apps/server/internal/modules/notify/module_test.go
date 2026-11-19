package notify_test

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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	dbmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// sharedDB starts one Postgres container for the package and migrates the
// notify set. Tests isolate themselves with fresh workspace and user IDs.
func sharedDB(t *testing.T) *pgxpool.Pool {
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
		if err := dbmigrations.Run(ctx, db, "notify", notify.Migrations); err != nil {
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

// thingHappened is an event of a hypothetical source module. Kind and
// ChannelID are what its rule copies into the notices it produces.
type thingHappened struct {
	WorkspaceID string
	Key         string
	Kind        string
	ChannelID   string
}

func (thingHappened) EventName() string { return "test.thing" }

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	if id, ok := f[token]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

// fakeDirectory is auth.Directory with a time zone per user ID.
type fakeDirectory map[string]string

func (f fakeDirectory) Users(_ context.Context, ids []string) ([]auth.UserSummary, error) {
	var out []auth.UserSummary
	for _, id := range ids {
		if z, ok := f[id]; ok {
			out = append(out, auth.UserSummary{ID: id, Timezone: z})
		}
	}
	return out, nil
}

type fakeMembers map[string]app.Role

func (f fakeMembers) Role(_ context.Context, workspaceID, userID string) (app.Role, error) {
	if r, ok := f[workspaceID+"/"+userID]; ok {
		return r, nil
	}
	return "", fmt.Errorf("not a member: %w", app.ErrForbidden)
}

// delivery is one SendToUsers call.
type delivery struct {
	Users []string
	Type  string
	Data  json.RawMessage
}

// fakeRealtime records deliveries instead of holding connections.
type fakeRealtime struct {
	mu   sync.Mutex
	list []delivery
}

func (f *fakeRealtime) SendToUsers(_ context.Context, ids []string, env app.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = append(f.list, delivery{Users: append([]string(nil), ids...), Type: env.Type, Data: env.Data})
	return nil
}

func (f *fakeRealtime) Online(context.Context, []string) ([]string, error) { return nil, nil }
func (f *fakeRealtime) DisconnectDevice(context.Context, string) error     { return nil }
func (f *fakeRealtime) Handle(string, app.ClientHandler)                   {}

func (f *fakeRealtime) of(typ string) []delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []delivery
	for _, d := range f.list {
		if d.Type == typ {
			out = append(out, d)
		}
	}
	return out
}

func (f *fakeRealtime) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = nil
}

// fakeJobs records what the module registers and can run one handler.
type fakeJobs struct {
	mu       sync.Mutex
	handlers map[string]app.JobHandler
	every    map[string]time.Duration
}

func (f *fakeJobs) Handle(kind string, h app.JobHandler, _ app.JobOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handlers == nil {
		f.handlers = map[string]app.JobHandler{}
	}
	f.handlers[kind] = h
}

func (f *fakeJobs) Every(interval time.Duration, kind string, _ any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.every == nil {
		f.every = map[string]time.Duration{}
	}
	f.every[kind] = interval
}

func (f *fakeJobs) Enqueue(context.Context, string, any, app.EnqueueOptions) error { return nil }

func (f *fakeJobs) run(kind string) error {
	f.mu.Lock()
	h := f.handlers[kind]
	f.mu.Unlock()
	if h == nil {
		return fmt.Errorf("no handler for %q", kind)
	}
	return h(context.Background(), json.RawMessage(`null`))
}

func (f *fakeJobs) interval(kind string) (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.every[kind]
	return d, ok
}

func slogNew(w io.Writer) *slog.Logger { return slog.New(slog.NewTextHandler(w, nil)) }

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

type user struct {
	ID, Token string
}

type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	h       http.Handler
	bus     *eventbus.Bus
	rt      *fakeRealtime
	jobs    *fakeJobs
	rules   *app.NotificationRules
	auth    fakeAuth
	members fakeMembers
	zones   fakeDirectory
	deps    *app.Deps
	reg     *app.Registry
	ws      string
	alice   user
	bob     user
}

func newEnv(t *testing.T) *env { return newEnvWith(t, &fakeRealtime{}, &fakeJobs{}) }

func newEnvWith(t *testing.T, rt *fakeRealtime, jobs *fakeJobs) *env {
	t.Helper()
	pool := sharedDB(t)
	e := &env{
		t: t, pool: pool, rt: rt, jobs: jobs, rules: app.NewNotificationRules(),
		auth: fakeAuth{}, members: fakeMembers{}, zones: fakeDirectory{}, ws: newID(),
	}
	e.alice = e.newUser()
	e.bob = e.newUser()

	services := app.NewServices()
	app.Provide[app.Authenticator](services, e.auth)
	app.Provide[app.MembershipResolver](services, e.members)
	app.Provide[auth.Directory](services, e.zones)

	logger := slogNew(testWriter{t})
	e.bus = eventbus.New(logger)
	deps := &app.Deps{
		Logger: logger, Services: services, Bus: e.bus, Notifications: e.rules,
		Tx: database.NewTxManager(pool), Platform: map[string]any{"db": pool},
	}
	if rt != nil {
		deps.Realtime = rt
	}
	if jobs != nil {
		deps.Jobs = jobs
	}
	e.deps = deps
	e.reg = app.NewRegistry(notify.New())
	return e
}

// start runs the registry with the notify module, after the caller registered
// rules as a source module's Init would.
func (e *env) start() {
	e.t.Helper()
	mods, err := e.reg.Enable([]string{"notify"})
	if err != nil {
		e.t.Fatal(err)
	}
	router := httpx.NewRouter(slogNew(testWriter{e.t}))
	var startErr error
	router.Route("/api/v1", func(v1 chi.Router) {
		startErr = e.reg.Start(mods, e.deps, httpx.NewAPI(v1, "test"))
	})
	if startErr != nil {
		e.t.Fatal(startErr)
	}
	e.h = router
}

func (e *env) newUser() user {
	id := newID()
	u := user{ID: id, Token: "tok-" + id}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.members[e.ws+"/"+id] = app.RoleMember
	return u
}

// mentionRule notifies one user for one key, as a source module would.
func (e *env) mentionRule(u user, kind, key string) {
	e.rules.On("test.thing", func(_ context.Context, evt app.Event) ([]app.Notification, error) {
		ev, ok := evt.(thingHappened)
		if !ok {
			return nil, fmt.Errorf("unexpected event %T", evt)
		}
		return []app.Notification{{
			WorkspaceID: ev.WorkspaceID, UserID: u.ID, Kind: kind,
			Title: "Ada in #general", Body: "@alice hi", Link: "/chat/c1",
			Data: map[string]any{"channel_id": "c1"}, Key: ev.Key,
		}}, nil
	})
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

func (e *env) call(method, path string, u user, body any) resp {
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
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	if e.ws != "" {
		req.Header.Set("X-Workspace-ID", e.ws)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{Code: rec.Code, Body: rec.Body.Bytes()}
}

type notificationBody struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Link      string         `json:"link"`
	Data      map[string]any `json:"data"`
	ReadAt    *string        `json:"read_at"`
	CreatedAt string         `json:"created_at"`
}

type listBody struct {
	Items   []notificationBody `json:"items"`
	HasMore bool               `json:"has_more"`
}

func (e *env) list(u user, query string) listBody {
	e.t.Helper()
	r := e.call("GET", "/notify/notifications"+query, u, nil)
	if r.Code != 200 {
		e.t.Fatalf("list = %d %s", r.Code, r.Body)
	}
	var page listBody
	r.json(e.t, &page)
	return page
}

func (e *env) unread(u user) int64 {
	e.t.Helper()
	r := e.call("GET", "/notify/unread-count", u, nil)
	if r.Code != 200 {
		e.t.Fatalf("unread-count = %d %s", r.Code, r.Body)
	}
	var body struct {
		Count int64 `json:"count"`
	}
	r.json(e.t, &body)
	return body.Count
}

func TestPublishStoresAndDeliversToTheUser(t *testing.T) {
	e := newEnv(t)
	e.mentionRule(e.alice, "chat.mention", "")
	e.start()

	evt := thingHappened{WorkspaceID: e.ws, Key: "chat.message:m1"}
	if err := e.bus.Publish(context.Background(), evt); err != nil {
		t.Fatal(err)
	}

	frames := e.rt.of(notify.EventCreated)
	if len(frames) != 1 {
		t.Fatalf("notification.created frames = %d, want 1", len(frames))
	}
	if len(frames[0].Users) != 1 || frames[0].Users[0] != e.alice.ID {
		t.Fatalf("frame went to %v, want only %s", frames[0].Users, e.alice.ID)
	}
	var data notify.CreatedEvent
	if err := json.Unmarshal(frames[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.WorkspaceID != e.ws || data.UnreadCount != 1 || data.Notification.Kind != "chat.mention" {
		t.Fatalf("frame data = %+v", data)
	}
	if data.Notification.Data["channel_id"] != "c1" || data.Notification.Link != "/chat/c1" {
		t.Fatalf("notification = %+v", data.Notification)
	}

	if got := e.unread(e.alice); got != 1 {
		t.Fatalf("alice unread = %d, want 1", got)
	}
	if got := e.unread(e.bob); got != 0 {
		t.Fatalf("bob unread = %d, want 0", got)
	}
	page := e.list(e.alice, "")
	if len(page.Items) != 1 || page.Items[0].Kind != "chat.mention" || page.Items[0].ReadAt != nil {
		t.Fatalf("alice list = %+v", page)
	}
	if other := e.list(e.bob, ""); len(other.Items) != 0 {
		t.Fatalf("bob saw %d notifications, want 0", len(other.Items))
	}

	// A redelivered event adds nothing and sends nothing.
	if err := e.bus.Publish(context.Background(), evt); err != nil {
		t.Fatal(err)
	}
	if got := e.unread(e.alice); got != 1 {
		t.Fatalf("unread after a repeat = %d, want 1", got)
	}
	if got := len(e.rt.of(notify.EventCreated)); got != 1 {
		t.Fatalf("frames after a repeat = %d, want 1", got)
	}
}

func TestBadRulesDoNotStopTheEvent(t *testing.T) {
	e := newEnv(t)
	e.rules.On("test.thing", func(context.Context, app.Event) ([]app.Notification, error) {
		panic("boom")
	})
	e.rules.On("test.thing", func(context.Context, app.Event) ([]app.Notification, error) {
		return nil, fmt.Errorf("nope")
	})
	e.rules.On("test.thing", func(context.Context, app.Event) ([]app.Notification, error) {
		return []app.Notification{{UserID: e.alice.ID, Kind: "chat.mention"}}, nil
	})
	e.rules.On("test.thing", func(_ context.Context, evt app.Event) ([]app.Notification, error) {
		ev := evt.(thingHappened)
		return []app.Notification{{
			WorkspaceID: ev.WorkspaceID, UserID: e.bob.ID, Kind: "chat.dm", Key: ev.Key,
		}}, nil
	})
	e.start()

	if err := e.bus.Publish(context.Background(), thingHappened{WorkspaceID: e.ws, Key: "chat.message:m2"}); err != nil {
		t.Fatal(err)
	}
	if got := e.unread(e.bob); got != 1 {
		t.Fatalf("bob unread = %d, want 1", got)
	}
	if got := e.unread(e.alice); got != 0 {
		t.Fatalf("alice unread = %d, want 0 from the invalid rule", got)
	}
}

func TestMarkReadAndReadAllTellTheUsersDevices(t *testing.T) {
	e := newEnv(t)
	e.mentionRule(e.alice, "chat.mention", "")
	e.start()
	for i := range 2 {
		evt := thingHappened{WorkspaceID: e.ws, Key: fmt.Sprintf("chat.message:m%d", i)}
		if err := e.bus.Publish(context.Background(), evt); err != nil {
			t.Fatal(err)
		}
	}
	e.rt.reset()

	page := e.list(e.alice, "")
	if len(page.Items) != 2 {
		t.Fatalf("list = %d items, want 2", len(page.Items))
	}
	id := page.Items[0].ID

	if r := e.call("POST", "/notify/notifications/"+id+"/read", e.bob, nil); r.Code != 404 {
		t.Fatalf("bob marking alice's notification = %d, want 404", r.Code)
	}
	if r := e.call("POST", "/notify/notifications/"+id+"/read", e.alice, nil); r.Code != 204 {
		t.Fatalf("mark read = %d %s", r.Code, r.Body)
	}
	if got := e.unread(e.alice); got != 1 {
		t.Fatalf("unread after one read = %d, want 1", got)
	}
	frames := e.rt.of(notify.EventRead)
	if len(frames) != 1 {
		t.Fatalf("notification.read frames = %d, want 1", len(frames))
	}
	if len(frames[0].Users) != 1 || frames[0].Users[0] != e.alice.ID {
		t.Fatalf("read frame went to %v, want only %s", frames[0].Users, e.alice.ID)
	}
	var data notify.ReadEvent
	if err := json.Unmarshal(frames[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.All || data.UnreadCount != 1 || len(data.IDs) != 1 || data.IDs[0] != id || data.WorkspaceID != e.ws {
		t.Fatalf("read frame = %+v", data)
	}

	if r := e.call("POST", "/notify/read-all", e.alice, nil); r.Code != 204 {
		t.Fatalf("read all = %d %s", r.Code, r.Body)
	}
	if got := e.unread(e.alice); got != 0 {
		t.Fatalf("unread after read-all = %d, want 0", got)
	}
	frames = e.rt.of(notify.EventRead)
	if len(frames) != 2 {
		t.Fatalf("read frames after read-all = %d, want 2", len(frames))
	}
	if err := json.Unmarshal(frames[1].Data, &data); err != nil {
		t.Fatal(err)
	}
	if !data.All || data.UnreadCount != 0 || len(data.IDs) != 1 {
		t.Fatalf("read-all frame = %+v", data)
	}
}

func TestPruneJobDeletesOldReadRows(t *testing.T) {
	e := newEnv(t)
	e.start()

	if d, ok := e.jobs.interval(notify.PruneJobKind); !ok || d != notify.PruneJobInterval {
		t.Fatalf("notify.prune interval = %v (registered=%v), want %v", d, ok, notify.PruneJobInterval)
	}

	ctx := context.Background()
	oldRead, oldUnread, recentRead := newID(), newID(), newID()
	insert := func(id, key string, createdAt time.Time, readAt *time.Time) {
		t.Helper()
		_, err := e.pool.Exec(ctx, `
			INSERT INTO notify_notifications (id, workspace_id, user_id, kind, title, body, link, data, key, read_at, created_at)
			VALUES ($1, $2, $3, 'chat.mention', '', '', '', '{}', $4, $5, $6)`,
			uuid.MustParse(id), uuid.MustParse(e.ws), uuid.MustParse(e.alice.ID), key, readAt, createdAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-100 * 24 * time.Hour)
	now := time.Now()
	read := now.Add(-time.Minute)
	insert(oldRead, "chat.message:old-read", old, &read)
	insert(oldUnread, "chat.message:old-unread", old, nil)
	insert(recentRead, "chat.message:recent-read", now, &read)

	if err := e.jobs.run(notify.PruneJobKind); err != nil {
		t.Fatal(err)
	}

	var left []string
	rows, err := e.pool.Query(ctx, `SELECT key FROM notify_notifications WHERE workspace_id = $1 ORDER BY key`, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		left = append(left, k)
	}
	if len(left) != 2 || left[0] != "chat.message:old-unread" || left[1] != "chat.message:recent-read" {
		t.Fatalf("left after prune = %v", left)
	}
}

func TestWithoutRealtimeOrJobs(t *testing.T) {
	e := newEnvWith(t, nil, nil)
	e.mentionRule(e.alice, "chat.mention", "")
	e.start()

	if err := e.bus.Publish(context.Background(), thingHappened{WorkspaceID: e.ws, Key: "chat.message:m1"}); err != nil {
		t.Fatal(err)
	}
	if got := e.unread(e.alice); got != 1 {
		t.Fatalf("unread = %d, want 1", got)
	}
	if r := e.call("GET", "/notify/notifications", e.alice, nil); r.Code != 200 {
		t.Fatalf("list = %d %s", r.Code, r.Body)
	}
}

func TestNoRulesMeansNoSubscriptions(t *testing.T) {
	e := newEnv(t)
	e.start()

	if err := e.bus.Publish(context.Background(), thingHappened{WorkspaceID: e.ws, Key: "chat.message:m1"}); err != nil {
		t.Fatal(err)
	}
	if got := e.unread(e.alice); got != 0 {
		t.Fatalf("unread = %d, want 0", got)
	}
	if len(e.rt.of(notify.EventCreated)) != 0 {
		t.Fatal("an event with no rules sent a frame")
	}
}

func TestNoopRegistryCollectsNoRules(t *testing.T) {
	e := newEnv(t)
	e.deps.Notifications = app.NoopNotificationRegistry{}
	e.mentionRule(e.alice, "chat.mention", "")
	e.start()

	if err := e.bus.Publish(context.Background(), thingHappened{WorkspaceID: e.ws, Key: "chat.message:m1"}); err != nil {
		t.Fatal(err)
	}
	if got := e.unread(e.alice); got != 0 {
		t.Fatalf("unread = %d, want 0 with a noop registry", got)
	}
}

// channelRule notifies both users about the event's channel and kind, the
// way chat's rules do: the notice carries ChannelID.
func (e *env) channelRule() {
	e.rules.On("test.thing", func(_ context.Context, evt app.Event) ([]app.Notification, error) {
		ev := evt.(thingHappened)
		var out []app.Notification
		for _, u := range []user{e.alice, e.bob} {
			out = append(out, app.Notification{
				WorkspaceID: ev.WorkspaceID, UserID: u.ID, Kind: ev.Kind, Title: "Ada in #general",
				Link: "/chat/" + ev.ChannelID, ChannelID: ev.ChannelID, Key: ev.Key,
			})
		}
		return out, nil
	})
}

// publishTo publishes one event and returns nothing; the frames show up in
// e.rt.
func (e *env) publishTo(kind, channelID, key string) {
	e.t.Helper()
	evt := thingHappened{WorkspaceID: e.ws, Key: key, Kind: kind, ChannelID: channelID}
	if err := e.bus.Publish(context.Background(), evt); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) savePreferences(u user, body map[string]any) {
	e.t.Helper()
	if r := e.call("PUT", "/notify/preferences", u, body); r.Code != 200 {
		e.t.Fatalf("put preferences = %d %s", r.Code, r.Body)
	}
}

// createdFor returns the notification.created frames delivered to one user.
func (e *env) createdFor(userID string) []notify.CreatedEvent {
	e.t.Helper()
	var out []notify.CreatedEvent
	for _, f := range e.rt.of(notify.EventCreated) {
		if len(f.Users) != 1 || f.Users[0] != userID {
			continue
		}
		var data notify.CreatedEvent
		if err := json.Unmarshal(f.Data, &data); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, data)
	}
	return out
}

func TestMutedChannelAndDisabledKindDropOnlyForThatUser(t *testing.T) {
	e := newEnv(t)
	e.channelRule()
	e.start()
	const muted, open = "0193f0f0-0000-7000-8000-0000000000d1", "0193f0f0-0000-7000-8000-0000000000d2"
	e.savePreferences(e.alice, map[string]any{
		"disabled_kinds": []string{"chat.dm"}, "muted_channel_ids": []string{muted},
		"dnd_start": nil, "dnd_end": nil,
	})

	e.publishTo("chat.mention", muted, "chat.message:m1") // alice muted this channel
	e.publishTo("chat.dm", open, "chat.message:m2")       // alice turned DMs off
	e.publishTo("chat.mention", open, "chat.message:m3")  // alice keeps this one

	if got := e.unread(e.alice); got != 1 {
		t.Fatalf("alice unread = %d, want 1 (only the open channel's mention)", got)
	}
	if page := e.list(e.alice, ""); len(page.Items) != 1 || page.Items[0].Link != "/chat/"+open {
		t.Fatalf("alice list = %+v", page.Items)
	}
	if got := e.unread(e.bob); got != 3 {
		t.Fatalf("bob unread = %d, want 3: alice's settings must not touch bob", got)
	}
	if got := len(e.createdFor(e.alice.ID)); got != 1 {
		t.Fatalf("alice got %d frames, want 1", got)
	}
	if got := len(e.createdFor(e.bob.ID)); got != 3 {
		t.Fatalf("bob got %d frames, want 3", got)
	}
}

// aroundNow returns "HH:MM" times for the Bangkok clock an hour before and an
// hour after now, so a window built from them contains now. Shifting both by
// two hours gives a window that does not.
func aroundNow(t *testing.T) (inStart, inEnd, outStart, outEnd string) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(loc)
	clock := func(d time.Duration) string { return local.Add(d).Format("15:04") }
	return clock(-time.Hour), clock(time.Hour), clock(2 * time.Hour), clock(4 * time.Hour)
}

func TestDoNotDisturbStoresAndCountsButMarksQuiet(t *testing.T) {
	e := newEnv(t)
	e.zones[e.alice.ID] = "Asia/Bangkok"
	e.channelRule()
	e.start()
	inStart, inEnd, _, _ := aroundNow(t)
	e.savePreferences(e.alice, map[string]any{
		"disabled_kinds": []string{}, "muted_channel_ids": []string{},
		"dnd_start": inStart, "dnd_end": inEnd,
	})

	e.publishTo("chat.mention", "0193f0f0-0000-7000-8000-0000000000d3", "chat.message:m1")

	alice := e.createdFor(e.alice.ID)
	if len(alice) != 1 {
		t.Fatalf("alice got %d frames, want 1", len(alice))
	}
	if !alice[0].Quiet {
		t.Fatal("alice's frame is not quiet inside her window")
	}
	if alice[0].UnreadCount != 1 || e.unread(e.alice) != 1 {
		t.Fatal("a quiet notice must still be stored and counted")
	}
	bob := e.createdFor(e.bob.ID)
	if len(bob) != 1 || bob[0].Quiet {
		t.Fatalf("bob's frame = %+v, want one that is not quiet", bob)
	}
}

func TestOutsideTheWindowTheFrameIsNotQuiet(t *testing.T) {
	e := newEnv(t)
	e.zones[e.alice.ID] = "Asia/Bangkok"
	e.channelRule()
	e.start()
	_, _, outStart, outEnd := aroundNow(t)
	e.savePreferences(e.alice, map[string]any{
		"disabled_kinds": []string{}, "muted_channel_ids": []string{},
		"dnd_start": outStart, "dnd_end": outEnd,
	})

	e.publishTo("chat.mention", "0193f0f0-0000-7000-8000-0000000000d3", "chat.message:m1")

	if frames := e.createdFor(e.alice.ID); len(frames) != 1 || frames[0].Quiet {
		t.Fatalf("alice's frames = %+v, want one that is not quiet", frames)
	}
}

func TestPreferenceRowsAreReadPerWorkspace(t *testing.T) {
	e := newEnv(t)
	e.channelRule()
	e.start()
	e.savePreferences(e.alice, map[string]any{
		"disabled_kinds": []string{"chat.mention"}, "muted_channel_ids": []string{},
		"dnd_start": nil, "dnd_end": nil,
	})

	// A notice for another workspace is not alice's settings here.
	otherWS := newID()
	evt := thingHappened{WorkspaceID: otherWS, Key: "chat.message:m9", Kind: "chat.mention"}
	if err := e.bus.Publish(context.Background(), evt); err != nil {
		t.Fatal(err)
	}
	if got := len(e.rt.of(notify.EventCreated)); got != 2 {
		t.Fatalf("frames = %d, want 2: alice's workspace setting must not apply in %s", got, otherWS)
	}
}
