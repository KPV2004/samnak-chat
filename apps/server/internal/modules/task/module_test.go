package task_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task"
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
// task and notify sets. Tests isolate themselves with fresh workspace and user
// IDs.
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
		if err := dbmigrations.Run(ctx, db, "task", task.Migrations); err != nil {
			dbErr = err.Error()
			return
		}
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

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	if id, ok := f[token]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

type fakeMembers map[string]app.Role

func (f fakeMembers) Role(_ context.Context, workspaceID, userID string) (app.Role, error) {
	if r, ok := f[workspaceID+"/"+userID]; ok {
		return r, nil
	}
	return "", fmt.Errorf("not a member: %w", app.ErrForbidden)
}

// fakeChannels is chat's ChannelAccess: readers lists "channel/user".
type fakeChannels map[string]bool

func (f fakeChannels) CanRead(_ context.Context, _, channelID, userID string) (bool, error) {
	return f[channelID+"/"+userID], nil
}

type fakeDirectory map[string]string

func (f fakeDirectory) Users(_ context.Context, ids []string) ([]auth.UserSummary, error) {
	var out []auth.UserSummary
	for _, id := range ids {
		if n, ok := f[id]; ok {
			out = append(out, auth.UserSummary{ID: id, DisplayName: n})
		}
	}
	return out, nil
}

type post struct {
	ID   string
	Card app.Card
}

type cardUpdate struct {
	MessageID string
	Payload   task.CardPayload
}

// fakeCards is chat's CardRegistry. It keeps what the module posted.
type fakeCards struct {
	mu      sync.Mutex
	kinds   []app.CardKind
	posts   []post
	updates []cardUpdate
}

func (c *fakeCards) Register(k app.CardKind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kinds = append(c.kinds, k)
}

func (c *fakeCards) Post(_ context.Context, card app.Card) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := newID()
	c.posts = append(c.posts, post{id, card})
	return id, nil
}

func (c *fakeCards) Update(_ context.Context, _, messageID string, payload any) error {
	p, ok := payload.(task.CardPayload)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, cardUpdate{messageID, p})
	return nil
}

func (c *fakeCards) DirectChannel(context.Context, string, []string) (string, error) {
	return "", errors.New("not used")
}

func (c *fakeCards) counts() (posts, updates int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.posts), len(c.updates)
}

// fakeExtensions is chat's ChatExtensions.
type fakeExtensions struct {
	commands []app.Command
	actions  []app.MessageAction
}

func (f *fakeExtensions) RegisterCommand(c app.Command)             { f.commands = append(f.commands, c) }
func (f *fakeExtensions) RegisterMessageAction(a app.MessageAction) { f.actions = append(f.actions, a) }

type fakeRealtime struct{}

func (fakeRealtime) SendToUsers(context.Context, []string, app.Envelope) error { return nil }
func (fakeRealtime) Online(context.Context, []string) ([]string, error)        { return nil, nil }
func (fakeRealtime) DisconnectDevice(context.Context, string) error            { return nil }
func (fakeRealtime) Handle(string, app.ClientHandler)                          {}

// fakeJobs records what the modules register and can run one handler.
type fakeJobs struct {
	handlers map[string]app.JobHandler
	every    map[string]time.Duration
}

func (f *fakeJobs) Handle(kind string, h app.JobHandler, _ app.JobOptions) {
	if f.handlers == nil {
		f.handlers = map[string]app.JobHandler{}
	}
	f.handlers[kind] = h
}

func (f *fakeJobs) Every(interval time.Duration, kind string, _ any) {
	if f.every == nil {
		f.every = map[string]time.Duration{}
	}
	f.every[kind] = interval
}

func (f *fakeJobs) Enqueue(context.Context, string, any, app.EnqueueOptions) error { return nil }

func (f *fakeJobs) run(kind string) error {
	h := f.handlers[kind]
	if h == nil {
		return fmt.Errorf("no handler for %q", kind)
	}
	return h(context.Background(), json.RawMessage(`null`))
}

// recordingBus is the in-memory bus that remembers what was declared.
type recordingBus struct {
	*eventbus.Bus
	declared []string
}

func (b *recordingBus) Declare(protos ...app.Event) {
	for _, p := range protos {
		b.declared = append(b.declared, p.EventName())
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

type user struct{ ID, Token, Name string }

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	h      http.Handler
	bus    *recordingBus
	jobs   *fakeJobs
	cards  *fakeCards
	ext    *fakeExtensions
	rules  *app.NotificationRules
	auth   fakeAuth
	roles  fakeMembers
	chans  fakeChannels
	dir    fakeDirectory
	svc    *app.Services
	deps   *app.Deps
	ws     string
	chID   string
	alice  user
	bob    user
	carol  user // a member who cannot read the channel
	admin  user
	notify bool
}

type option func(*env)

// without removes a service another module would provide.
func without(f func(e *env)) option { return f }

func newEnv(t *testing.T, opts ...option) *env {
	t.Helper()
	pool := sharedDB(t)
	e := &env{
		t: t, pool: pool, cards: &fakeCards{}, ext: &fakeExtensions{}, jobs: &fakeJobs{}, rules: app.NewNotificationRules(),
		auth: fakeAuth{}, roles: fakeMembers{}, chans: fakeChannels{}, dir: fakeDirectory{}, ws: newID(), chID: newID(), notify: true,
	}
	e.alice, e.bob, e.carol = e.newUser("Alice", app.RoleMember), e.newUser("Bob", app.RoleMember), e.newUser("Carol", app.RoleMember)
	e.admin = e.newUser("Admin", app.RoleAdmin)
	for _, u := range []user{e.alice, e.bob, e.admin} {
		e.chans[e.chID+"/"+u.ID] = true
	}

	e.svc = app.NewServices()
	app.Provide[app.Authenticator](e.svc, e.auth)
	app.Provide[app.MembershipResolver](e.svc, e.roles)
	app.Provide[app.ChannelAccess](e.svc, e.chans)
	app.Provide[app.CardRegistry](e.svc, e.cards)
	app.Provide[app.ChatExtensions](e.svc, e.ext)
	app.Provide[auth.Directory](e.svc, e.dir)

	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	e.bus = &recordingBus{Bus: eventbus.New(logger)}
	e.deps = &app.Deps{
		Logger: logger, Services: e.svc, Bus: e.bus, Notifications: e.rules, Tx: database.NewTxManager(pool),
		Platform: map[string]any{"db": pool}, Realtime: fakeRealtime{}, Jobs: e.jobs,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// start runs the registry like the server does.
func (e *env) start() {
	e.t.Helper()
	modules := []app.Module{task.New()}
	names := []string{"task"}
	if e.notify {
		modules, names = append(modules, notify.New()), append(names, "notify")
	}
	reg := app.NewRegistry(modules...)
	mods, err := reg.Enable(names)
	if err != nil {
		e.t.Fatal(err)
	}
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var startErr error
	router.Route("/api/v1", func(v1 chi.Router) {
		startErr = reg.Start(mods, e.deps, httpx.NewAPI(v1, "test"))
	})
	if startErr != nil {
		e.t.Fatal(startErr)
	}
	e.h = router
}

func (e *env) newUser(name string, role app.Role) user {
	id := newID()
	u := user{ID: id, Token: "tok-" + id, Name: name}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.roles[e.ws+"/"+id] = role
	e.dir[id] = name
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

func (e *env) call(method, path string, u user, body any) resp {
	e.t.Helper()
	rd := bytes.NewReader(nil)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	req.Header.Set("X-Workspace-ID", e.ws)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{Code: rec.Code, Body: rec.Body.Bytes()}
}

type taskBody struct {
	ID         string  `json:"id"`
	MessageID  *string `json:"message_id"`
	Title      string  `json:"title"`
	Status     string  `json:"status"`
	AssigneeID *string `json:"assignee_id"`
	UpdatedAt  string  `json:"updated_at"`
}

func (e *env) createTask(u user, body map[string]any) taskBody {
	e.t.Helper()
	if _, ok := body["channel_id"]; !ok {
		body["channel_id"] = e.chID
	}
	r := e.call("POST", "/task/items", u, body)
	if r.Code != 201 {
		e.t.Fatalf("create = %d %s", r.Code, r.Body)
	}
	var tb taskBody
	r.json(e.t, &tb)
	return tb
}

func (e *env) getTask(u user, id string) taskBody {
	e.t.Helper()
	r := e.call("GET", "/task/items/"+id, u, nil)
	if r.Code != 200 {
		e.t.Fatalf("get = %d %s", r.Code, r.Body)
	}
	var tb taskBody
	r.json(e.t, &tb)
	return tb
}

type notificationBody struct {
	Kind  string         `json:"kind"`
	Title string         `json:"title"`
	Body  string         `json:"body"`
	Link  string         `json:"link"`
	Data  map[string]any `json:"data"`
}

func (e *env) inbox(u user) []notificationBody {
	e.t.Helper()
	r := e.call("GET", "/notify/notifications", u, nil)
	if r.Code != 200 {
		e.t.Fatalf("inbox = %d %s", r.Code, r.Body)
	}
	var page struct {
		Items []notificationBody `json:"items"`
	}
	r.json(e.t, &page)
	return page.Items
}

func kinds(ns []notificationBody) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Kind
	}
	slices.Sort(out)
	return out
}

func TestInitRegistersWhatTheModuleAddsToOthers(t *testing.T) {
	e := newEnv(t)
	e.start()

	if len(e.cards.kinds) != 1 || e.cards.kinds[0] != (app.CardKind{Kind: "task.item", Module: "task"}) {
		t.Fatalf("card kinds = %+v", e.cards.kinds)
	}
	if len(e.ext.commands) != 1 {
		t.Fatalf("commands = %+v", e.ext.commands)
	}
	if c := e.ext.commands[0]; c.Name != "task" || c.Module != "task" || c.Usage != "/task <title> [@assignee]" || c.Description == "" || c.Handler == nil {
		t.Fatalf("command = %+v", c)
	}
	if len(e.ext.actions) != 1 || e.ext.actions[0] != (app.MessageAction{ID: "task.create_from_message", Module: "task"}) {
		t.Fatalf("actions = %+v", e.ext.actions)
	}
	if d, ok := e.jobs.every[task.DueScanJobKind]; !ok || d != 5*time.Minute || e.jobs.handlers[task.DueScanJobKind] == nil {
		t.Fatalf("due scan job: interval=%v registered=%v", d, ok)
	}
	want := []string{"task.assigned", "task.created", "task.due_soon", "task.overdue", "task.status_changed", "task.updated"}
	got := slices.Clone(e.bus.declared)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("declared = %v, want %v", got, want)
	}
	rules := e.rules.Rules()
	for _, name := range []string{"task.assigned", "task.due_soon", "task.overdue"} {
		if len(rules[name]) != 1 {
			t.Errorf("rules for %s = %d, want 1", name, len(rules[name]))
		}
	}
}

func TestEventsRoundTripThroughJSON(t *testing.T) {
	at := time.Date(2026, 3, 4, 10, 0, 0, 123456000, time.UTC)
	for _, evt := range []app.Event{
		task.TaskCreated{WorkspaceID: "w", TaskID: "t", ChannelID: "c", CreatorID: "u", AssigneeID: "a"},
		task.TaskAssigned{WorkspaceID: "w", TaskID: "t", AssigneeID: "a", ActorID: "u", UpdatedAt: at},
		task.TaskStatusChanged{WorkspaceID: "w", TaskID: "t", From: "todo", To: "done", ActorID: "u"},
		task.TaskUpdated{WorkspaceID: "w", TaskID: "t"},
		task.TaskDueSoon{WorkspaceID: "w", TaskID: "t", AssigneeID: "a", DueAt: at},
		task.TaskOverdue{WorkspaceID: "w", TaskID: "t", AssigneeID: "a", DueAt: at},
	} {
		b, err := json.Marshal(evt)
		if err != nil {
			t.Fatal(err)
		}
		back := newOf(evt)
		if err := json.Unmarshal(b, back); err != nil {
			t.Fatal(err)
		}
		if got := derefEvent(back); got != evt {
			t.Errorf("%s: %+v came back as %+v", evt.EventName(), evt, got)
		}
	}
}

func newOf(evt app.Event) any {
	switch evt.(type) {
	case task.TaskCreated:
		return &task.TaskCreated{}
	case task.TaskAssigned:
		return &task.TaskAssigned{}
	case task.TaskStatusChanged:
		return &task.TaskStatusChanged{}
	case task.TaskUpdated:
		return &task.TaskUpdated{}
	case task.TaskDueSoon:
		return &task.TaskDueSoon{}
	default:
		return &task.TaskOverdue{}
	}
}

func derefEvent(p any) app.Event {
	switch v := p.(type) {
	case *task.TaskCreated:
		return *v
	case *task.TaskAssigned:
		return *v
	case *task.TaskStatusChanged:
		return *v
	case *task.TaskUpdated:
		return *v
	case *task.TaskDueSoon:
		return *v
	default:
		return *p.(*task.TaskOverdue)
	}
}

func TestCreatePostsTheCardAndNotifiesTheAssignee(t *testing.T) {
	e := newEnv(t)
	e.start()
	due := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)

	created := e.createTask(e.alice, map[string]any{"title": "Fix login", "assignee_id": e.bob.ID, "due_at": due.Format(time.RFC3339), "priority": "high"})

	posts, _ := e.cards.counts()
	if posts != 1 {
		t.Fatalf("cards posted = %d, want 1", posts)
	}
	card := e.cards.posts[0].Card
	if card.WorkspaceID != e.ws || card.ChannelID != e.chID || card.SenderID != e.alice.ID || card.Kind != "task.item" || card.FallbackText != "Task: Fix login" {
		t.Fatalf("card = %+v", card)
	}
	p := card.Payload.(task.CardPayload)
	if p.TaskID != created.ID || p.Status != "todo" || p.Priority != "high" || p.AssigneeName != "Bob" || p.AssigneeID == nil || *p.AssigneeID != e.bob.ID ||
		p.DueAt == nil || !p.DueAt.Equal(due) || p.Version == 0 {
		t.Fatalf("payload = %+v", p)
	}
	if got := e.getTask(e.bob, created.ID); got.MessageID == nil || *got.MessageID != e.cards.posts[0].ID {
		t.Fatalf("message_id = %v, want the posted card", got.MessageID)
	}

	// Bob is told; Alice, who did it, is not.
	inbox := e.inbox(e.bob)
	if len(inbox) != 1 || inbox[0].Kind != task.NotifyAssigned || inbox[0].Title != "Alice assigned you a task" || inbox[0].Body != "Fix login" ||
		inbox[0].Link != "/chat/"+e.chID || inbox[0].Data["task_id"] != created.ID {
		t.Fatalf("bob's inbox = %+v", inbox)
	}
	if got := e.inbox(e.alice); len(got) != 0 {
		t.Fatalf("alice's inbox = %+v", got)
	}
}

func TestSelfAssignmentNotifiesNobody(t *testing.T) {
	e := newEnv(t)
	e.start()
	e.createTask(e.alice, map[string]any{"title": "Mine", "assignee_id": e.alice.ID})
	if got := e.inbox(e.alice); len(got) != 0 {
		t.Fatalf("inbox = %+v", got)
	}
}

func TestReassigningNotifiesTheNewAssigneeOnce(t *testing.T) {
	e := newEnv(t)
	e.start()
	created := e.createTask(e.alice, map[string]any{"title": "Fix"})

	r := e.call("PATCH", "/task/items/"+created.ID, e.alice, map[string]any{"assignee_id": e.bob.ID})
	if r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	if got := kinds(e.inbox(e.bob)); !slices.Equal(got, []string{"task.assigned"}) {
		t.Fatalf("bob's inbox = %v", got)
	}
	// A plain edit afterwards is not an assignment.
	if r := e.call("PATCH", "/task/items/"+created.ID, e.alice, map[string]any{"title": "Fix it"}); r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	if got := e.inbox(e.bob); len(got) != 1 {
		t.Fatalf("bob's inbox = %d notifications, want 1", len(got))
	}
}

func TestStatusChangeUpdatesTheCard(t *testing.T) {
	e := newEnv(t)
	e.start()
	created := e.createTask(e.alice, map[string]any{"title": "Fix", "assignee_id": e.bob.ID})
	_, before := e.cards.counts()
	firstVersion := e.cards.posts[0].Card.Payload.(task.CardPayload).Version

	time.Sleep(5 * time.Millisecond) // the card version is updated_at in milliseconds
	r := e.call("PATCH", "/task/items/"+created.ID, e.bob, map[string]any{"status": "doing"})
	if r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	posts, updates := e.cards.counts()
	if posts != 1 || updates != before+1 {
		t.Fatalf("posts=%d updates=%d, want 1 post and one more update", posts, updates)
	}
	u := e.cards.updates[len(e.cards.updates)-1]
	if u.MessageID != e.cards.posts[0].ID || u.Payload.Status != "doing" || u.Payload.Version <= firstVersion {
		t.Fatalf("update = %+v (first version %d)", u, firstVersion)
	}

	// Nobody but the creator, assignee and admins may change it.
	e.chans[e.chID+"/"+e.carol.ID] = true
	if r := e.call("PATCH", "/task/items/"+created.ID, e.carol, map[string]any{"status": "done"}); r.Code != 403 {
		t.Fatalf("carol = %d, want 403", r.Code)
	}
	if r := e.call("PATCH", "/task/items/"+created.ID, e.admin, map[string]any{"status": "done"}); r.Code != 200 {
		t.Fatalf("admin = %d %s", r.Code, r.Body)
	}
}

func TestARedeliveredEventDoesNotPostASecondCard(t *testing.T) {
	e := newEnv(t)
	e.start()
	created := e.createTask(e.alice, map[string]any{"title": "Fix"})
	evt := task.TaskCreated{WorkspaceID: e.ws, TaskID: created.ID, ChannelID: e.chID, CreatorID: e.alice.ID}

	for range 3 {
		if err := e.bus.Publish(context.Background(), evt); err != nil {
			t.Fatal(err)
		}
	}
	posts, updates := e.cards.counts()
	if posts != 1 || updates != 3 {
		t.Fatalf("posts=%d updates=%d, want 1 and 3", posts, updates)
	}
}

func TestOnlyReadersOfTheChannelSeeTheTask(t *testing.T) {
	e := newEnv(t)
	e.start()
	created := e.createTask(e.alice, map[string]any{"title": "Secret plans"})

	if r := e.call("GET", "/task/items/"+created.ID, e.carol, nil); r.Code != 404 {
		t.Fatalf("get = %d, want 404", r.Code)
	}
	r := e.call("GET", "/task/items", e.carol, nil)
	var page struct {
		Items []taskBody `json:"items"`
	}
	r.json(t, &page)
	if r.Code != 200 || len(page.Items) != 0 {
		t.Fatalf("carol's list = %d %s", r.Code, r.Body)
	}
	if r := e.call("POST", "/task/items", e.carol, map[string]any{"channel_id": e.chID, "title": "x"}); r.Code != 404 {
		t.Fatalf("create in an unreadable channel = %d, want 404", r.Code)
	}
	if r := e.call("POST", "/task/items", e.alice, map[string]any{"channel_id": e.chID, "title": "x", "assignee_id": e.carol.ID}); r.Code != 400 {
		t.Fatalf("assign to someone who cannot read = %d, want 400", r.Code)
	}
	r = e.call("GET", "/task/items", e.bob, nil)
	r.json(t, &page)
	if len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("bob's list = %s", r.Body)
	}
}

func TestListFiltersOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.start()
	a := e.createTask(e.alice, map[string]any{"title": "a", "assignee_id": e.bob.ID})
	e.createTask(e.alice, map[string]any{"title": "b"})
	if r := e.call("PATCH", "/task/items/"+a.ID, e.bob, map[string]any{"status": "doing"}); r.Code != 200 {
		t.Fatalf("patch = %d", r.Code)
	}
	list := func(u user, q string) []string {
		r := e.call("GET", "/task/items"+q, u, nil)
		if r.Code != 200 {
			t.Fatalf("%s = %d %s", q, r.Code, r.Body)
		}
		var page struct {
			Items []taskBody `json:"items"`
		}
		r.json(t, &page)
		var out []string
		for _, k := range page.Items {
			out = append(out, k.Title)
		}
		return out
	}
	if got := list(e.bob, "?assignee=me"); !slices.Equal(got, []string{"a"}) {
		t.Errorf("assignee=me = %v", got)
	}
	if got := list(e.alice, "?creator=me&status=todo"); !slices.Equal(got, []string{"b"}) {
		t.Errorf("creator=me&status=todo = %v", got)
	}
	if got := list(e.alice, "?status=todo,doing&channel_id="+e.chID); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("status=todo,doing = %v", got)
	}
}

// dueScanEnv starts an env with an empty task table: the scan looks at every
// workspace, and the tests of this package share one database.
func dueScanEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	if _, err := e.pool.Exec(context.Background(), `DELETE FROM task_items`); err != nil {
		t.Fatal(err)
	}
	e.start()
	return e
}

func TestDueScanNotifiesTheAssigneeOncePerDueDate(t *testing.T) {
	e := dueScanEnv(t)
	soon := e.createTask(e.alice, map[string]any{"title": "Soon", "assignee_id": e.bob.ID, "due_at": time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)})
	e.createTask(e.alice, map[string]any{"title": "Late", "assignee_id": e.bob.ID, "due_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})
	e.createTask(e.alice, map[string]any{"title": "Far", "assignee_id": e.bob.ID, "due_at": time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)})
	e.createTask(e.alice, map[string]any{"title": "Nobody", "due_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})

	if err := e.jobs.run(task.DueScanJobKind); err != nil {
		t.Fatal(err)
	}
	got := kinds(e.inbox(e.bob))
	want := []string{"task.assigned", "task.assigned", "task.assigned", "task.due_soon", "task.overdue"}
	if !slices.Equal(got, want) {
		t.Fatalf("bob's inbox = %v, want %v", got, want)
	}
	for _, n := range e.inbox(e.bob) {
		if n.Kind == "task.due_soon" && (n.Body != "Soon" || n.Data["task_id"] != soon.ID) {
			t.Fatalf("due soon = %+v", n)
		}
		if n.Kind == "task.overdue" && n.Body != "Late" {
			t.Fatalf("overdue = %+v", n)
		}
	}
	if got := e.inbox(e.alice); len(got) != 0 {
		t.Fatalf("alice's inbox = %+v", got)
	}

	// The marks are stored: another scan, or another server's scan, adds nothing.
	if err := e.jobs.run(task.DueScanJobKind); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox(e.bob); len(got) != 5 {
		t.Fatalf("after a second scan: %d notifications, want 5", len(got))
	}

	// A new due date is announced again.
	r := e.call("PATCH", "/task/items/"+soon.ID, e.alice, map[string]any{"due_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)})
	if r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	if err := e.jobs.run(task.DueScanJobKind); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox(e.bob); len(got) != 6 {
		t.Fatalf("after rescheduling: %d notifications, want 6", len(got))
	}

	// The cards were refreshed by the due events too.
	if _, updates := e.cards.counts(); updates == 0 {
		t.Fatal("due events did not touch the cards")
	}
}

func TestDueScanSkipsDoneTasks(t *testing.T) {
	e := dueScanEnv(t)
	done := e.createTask(e.alice, map[string]any{"title": "Done", "assignee_id": e.bob.ID, "due_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})
	if r := e.call("PATCH", "/task/items/"+done.ID, e.alice, map[string]any{"status": "done"}); r.Code != 200 {
		t.Fatalf("patch = %d", r.Code)
	}
	if err := e.jobs.run(task.DueScanJobKind); err != nil {
		t.Fatal(err)
	}
	if got := kinds(e.inbox(e.bob)); !slices.Equal(got, []string{"task.assigned"}) {
		t.Fatalf("bob's inbox = %v", got)
	}
}

func principalCtx(e *env, u user) context.Context {
	return app.WithPrincipal(context.Background(), app.Principal{UserID: u.ID, WorkspaceID: e.ws, Role: app.RoleMember})
}

func TestTaskCommandCreatesATaskAndItsCard(t *testing.T) {
	e := newEnv(t)
	e.start()
	cmd := e.ext.commands[0]

	res, err := cmd.Handler(principalCtx(e, e.alice), app.CommandInput{ChannelID: e.chID, Args: "Fix <@" + e.bob.ID + "> the login page"})
	if err != nil || res.Text != "" {
		t.Fatalf("result = %+v, %v", res, err)
	}
	r := e.call("GET", "/task/items?assignee=me", e.bob, nil)
	var page struct {
		Items []taskBody `json:"items"`
	}
	r.json(t, &page)
	if len(page.Items) != 1 || page.Items[0].Title != "Fix the login page" {
		t.Fatalf("bob's tasks = %s", r.Body)
	}
	if posts, _ := e.cards.counts(); posts != 1 {
		t.Fatalf("cards = %d, want 1", posts)
	}
	if got := kinds(e.inbox(e.bob)); !slices.Equal(got, []string{"task.assigned"}) {
		t.Fatalf("bob's inbox = %v", got)
	}

	for name, tc := range map[string]struct {
		ctx  context.Context
		in   app.CommandInput
		want error
	}{
		"no title":     {principalCtx(e, e.alice), app.CommandInput{ChannelID: e.chID, Args: "  "}, app.ErrInvalid},
		"cannot read":  {principalCtx(e, e.carol), app.CommandInput{ChannelID: e.chID, Args: "x"}, app.ErrNotFound},
		"no principal": {context.Background(), app.CommandInput{ChannelID: e.chID, Args: "x"}, app.ErrUnauthorized},
		"bad assignee": {principalCtx(e, e.alice), app.CommandInput{ChannelID: e.chID, Args: "x <@" + e.carol.ID + ">"}, app.ErrInvalid},
	} {
		if _, err := cmd.Handler(tc.ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestWorksWithoutChatExtensionsCardsJobsOrDirectory(t *testing.T) {
	e := newEnv(t, without(func(e *env) {
		e.svc = app.NewServices()
		app.Provide[app.Authenticator](e.svc, e.auth)
		app.Provide[app.MembershipResolver](e.svc, e.roles)
		app.Provide[app.ChannelAccess](e.svc, e.chans)
		e.deps.Services, e.deps.Jobs = e.svc, nil
	}))
	e.start()

	created := e.createTask(e.alice, map[string]any{"title": "Plain", "assignee_id": e.bob.ID})
	if got := e.getTask(e.bob, created.ID); got.MessageID != nil {
		t.Fatalf("message_id = %v, want none without a card registry", got.MessageID)
	}
	if posts, _ := e.cards.counts(); posts != 0 {
		t.Fatalf("posted %d cards", posts)
	}
	inbox := e.inbox(e.bob)
	if len(inbox) != 1 || inbox[0].Title != "You were assigned a task" {
		t.Fatalf("bob's inbox = %+v", inbox)
	}
}

func TestWorksWithNotifyOff(t *testing.T) {
	e := newEnv(t)
	e.notify = false
	e.start()
	created := e.createTask(e.alice, map[string]any{"title": "Quiet", "assignee_id": e.bob.ID})
	if posts, _ := e.cards.counts(); posts != 1 {
		t.Fatalf("cards = %d, want 1", posts)
	}
	if r := e.call("PATCH", "/task/items/"+created.ID, e.bob, map[string]any{"status": "done"}); r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	if r := e.call("GET", "/notify/notifications", e.bob, nil); r.Code != 404 {
		t.Fatalf("notify routes = %d, want 404 while the module is off", r.Code)
	}
}

func TestWithoutADatabaseTheModuleStillStarts(t *testing.T) {
	// The openapi command runs Init and Routes with no database and no jobs.
	svc := app.NewServices()
	deps := &app.Deps{Services: svc, Notifications: app.NoopNotificationRegistry{}, Bus: eventbus.New(nil), Platform: map[string]any{}}
	reg := app.NewRegistry(task.New())
	mods, err := reg.Enable([]string{"task"})
	if err != nil {
		t.Fatal(err)
	}
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var startErr error
	router.Route("/api/v1", func(v1 chi.Router) { startErr = reg.Start(mods, deps, httpx.NewAPI(v1, "test")) })
	if startErr != nil {
		t.Fatal(startErr)
	}
}
