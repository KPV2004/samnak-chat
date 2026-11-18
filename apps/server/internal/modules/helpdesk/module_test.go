package helpdesk_test

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
	"reflect"
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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
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
// helpdesk set. Tests isolate themselves with fresh workspace and user IDs.
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

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func slogNew(w io.Writer) *slog.Logger { return slog.New(slog.NewTextHandler(w, nil)) }

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

// fakeCards is chat's card registry: it keeps what was posted and updated.
type fakeCards struct {
	mu      sync.Mutex
	kinds   []app.CardKind
	posts   []app.Card
	updates int
	byID    map[string]app.Card
}

func newFakeCards() *fakeCards { return &fakeCards{byID: map[string]app.Card{}} }

func (f *fakeCards) Register(k app.CardKind) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kinds = append(f.kinds, k)
}

func (f *fakeCards) Post(_ context.Context, c app.Card) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := newID()
	f.posts = append(f.posts, c)
	f.byID[id] = c
	return id, nil
}

func (f *fakeCards) Update(_ context.Context, _, messageID string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[messageID]
	if !ok {
		return fmt.Errorf("card %s: %w", messageID, app.ErrNotFound)
	}
	c.Payload = payload
	f.byID[messageID] = c
	f.updates++
	return nil
}

func (f *fakeCards) DirectChannel(_ context.Context, _ string, _ []string) (string, error) {
	return newID(), nil
}

// fakeExtensions is chat's ChatExtensions.
type fakeExtensions struct {
	commands []app.Command
	actions  []app.MessageAction
}

func (f *fakeExtensions) RegisterCommand(c app.Command)             { f.commands = append(f.commands, c) }
func (f *fakeExtensions) RegisterMessageAction(a app.MessageAction) { f.actions = append(f.actions, a) }

// fakeDirectory is auth's Directory.
type fakeDirectory map[string]string

func (f fakeDirectory) Users(_ context.Context, ids []string) ([]auth.UserSummary, error) {
	out := make([]auth.UserSummary, 0, len(ids))
	for _, id := range ids {
		if name, ok := f[id]; ok {
			out = append(out, auth.UserSummary{ID: id, DisplayName: name})
		}
	}
	return out, nil
}

// fakeStorage presigns without a network and knows which objects exist.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]int64
}

func (s *fakeStorage) PresignPut(_ context.Context, key, _ string, _ int64, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/put/" + key, Method: "PUT", Headers: map[string]string{}, ExpiresAt: time.Now().Add(ttl)}, nil
}

func (s *fakeStorage) PresignGet(_ context.Context, key, _ string, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/get/" + key, Method: "GET", ExpiresAt: time.Now().Add(ttl)}, nil
}

func (s *fakeStorage) Stat(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if size, ok := s.objects[key]; ok {
		return size, nil
	}
	return 0, fmt.Errorf("object: %w", app.ErrNotFound)
}

// busSim stands in for the outbox: an event published inside a transaction is
// held until the commit, must be declared, and must survive a JSON round trip
// (PLT-10). Outside a transaction it is delivered at once.
type busSim struct {
	*eventbus.Bus
	tx       app.TxManager
	mu       sync.Mutex
	declared map[string]reflect.Type
	// delivered is what listeners saw, in order.
	delivered []app.Event
}

type collectorKey struct{}

type collector struct{ events []app.Event }

func newBusSim(log *slog.Logger, tx app.TxManager) *busSim {
	return &busSim{Bus: eventbus.New(log), tx: tx, declared: map[string]reflect.Type{}}
}

func (b *busSim) Declare(protos ...app.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range protos {
		b.declared[p.EventName()] = reflect.TypeOf(p)
	}
}

func (b *busSim) Publish(ctx context.Context, evt app.Event) error {
	c, ok := ctx.Value(collectorKey{}).(*collector)
	if !ok {
		b.record(evt)
		return b.Bus.Publish(ctx, evt)
	}
	b.mu.Lock()
	typ, declared := b.declared[evt.EventName()]
	b.mu.Unlock()
	if !declared || typ != reflect.TypeOf(evt) {
		return fmt.Errorf("event %q published in a transaction without being declared", evt.EventName())
	}
	raw, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	back := reflect.New(typ)
	if err := json.Unmarshal(raw, back.Interface()); err != nil {
		return err
	}
	if !reflect.DeepEqual(back.Elem().Interface(), evt) {
		return fmt.Errorf("event %q does not round-trip through JSON", evt.EventName())
	}
	c.events = append(c.events, back.Elem().Interface().(app.Event))
	return nil
}

func (b *busSim) record(evt app.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.delivered = append(b.delivered, evt)
}

// Do implements app.TxManager: the held events are delivered after the commit.
func (b *busSim) Do(ctx context.Context, fn func(context.Context) error) error {
	if _, nested := ctx.Value(collectorKey{}).(*collector); nested {
		return fn(ctx)
	}
	c := &collector{}
	if err := b.tx.Do(context.WithValue(ctx, collectorKey{}, c), fn); err != nil {
		return err
	}
	for _, evt := range c.events {
		b.record(evt)
		_ = b.Bus.Publish(context.WithoutCancel(ctx), evt)
	}
	return nil
}

func (b *busSim) names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, e := range b.delivered {
		out = append(out, e.EventName())
	}
	return out
}

type user struct {
	ID, Token string
}

type env struct {
	t        *testing.T
	h        http.Handler
	bus      *busSim
	deps     *app.Deps
	store    *fakeStorage
	cards    *fakeCards
	ext      *fakeExtensions
	dir      fakeDirectory
	rules    *app.NotificationRules
	ws       string
	admin    user
	agent    user
	member   user
	outsider user
	auth     fakeAuth
	members  fakeMembers
}

// newEnv wires the helpdesk module on the real registry with the real workspace
// authorizer, a real transaction manager, fake object storage, chat's card
// registry and extensions, and auth's directory.
func newEnv(t *testing.T) *env {
	t.Helper()
	pool := sharedDB(t)
	e := &env{
		t: t, auth: fakeAuth{}, members: fakeMembers{}, store: &fakeStorage{objects: map[string]int64{}},
		cards: newFakeCards(), ext: &fakeExtensions{}, dir: fakeDirectory{}, rules: app.NewNotificationRules(), ws: newID(),
	}
	e.admin = e.newUser("admin", app.RoleAdmin)
	e.agent = e.newUser("agent", app.RoleMember)
	e.member = e.newUser("member", app.RoleMember)
	e.outsider = user{ID: newID(), Token: "tok-outsider"}
	e.auth[e.outsider.Token] = app.Identity{UserID: e.outsider.ID, DeviceID: "dev-x"}

	services := app.NewServices()
	policy := workspace.NewAuthorizer()
	app.Provide[app.Authenticator](services, e.auth)
	app.Provide[app.MembershipResolver](services, e.members)
	app.Provide[workspace.Authorizer](services, policy)
	app.Provide[workspace.ActionRegistry](services, policy)
	app.Provide[app.CardRegistry](services, e.cards)
	app.Provide[app.ChatExtensions](services, e.ext)
	app.Provide[auth.Directory](services, e.dir)

	logger := slogNew(testWriter{t})
	e.bus = newBusSim(logger, database.NewTxManager(pool))
	e.deps = &app.Deps{
		Logger: logger, Services: services, Bus: e.bus, Notifications: e.rules, Tx: e.bus,
		Storage: e.store, Platform: map[string]any{"db": pool},
	}
	return e
}

func (e *env) newUser(name string, role app.Role) user {
	id := newID()
	u := user{ID: id, Token: "tok-" + name + "-" + id}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.members[e.ws+"/"+id] = role
	e.dir[id] = name
	return u
}

// start builds the server with the given modules enabled.
func (e *env) start(modules ...app.Module) {
	e.t.Helper()
	reg := app.NewRegistry(modules...)
	names := make([]string, 0, len(modules))
	for _, m := range modules {
		names = append(names, m.Name())
	}
	mods, err := reg.Enable(names)
	if err != nil {
		e.t.Fatal(err)
	}
	router := httpx.NewRouter(slogNew(testWriter{e.t}))
	var startErr error
	router.Route("/api/v1", func(v1 chi.Router) {
		startErr = reg.Start(mods, e.deps, httpx.NewAPI(v1, "test"))
	})
	if startErr != nil {
		e.t.Fatal(startErr)
	}
	e.h = router
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
	var rd io.Reader = bytes.NewReader(nil)
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

// TestHelpdeskRunsTheWholeFlowOnTheRealRegistry takes a ticket from the form to
// closed through the HTTP API of the registered module.
func TestHelpdeskRunsTheWholeFlowOnTheRealRegistry(t *testing.T) {
	e := newEnv(t)
	e.start(helpdesk.New())

	var q struct {
		ID string `json:"id"`
	}
	r := e.call(http.MethodPost, "/helpdesk/queues", e.admin, map[string]any{"name": "IT", "prefix": "IT"})
	if r.Code != http.StatusCreated {
		t.Fatalf("create queue = %d: %s", r.Code, r.Body)
	}
	r.json(t, &q)
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+q.ID+"/agents", e.admin, map[string]any{"user_ids": []string{e.agent.ID}}); r.Code != http.StatusNoContent {
		t.Fatalf("set agents = %d: %s", r.Code, r.Body)
	}
	var cat struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/categories", e.admin, map[string]any{"queue_id": q.ID, "name": "Laptop", "is_default": true}).json(t, &cat)

	var tk struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	r = e.call(http.MethodPost, "/helpdesk/tickets", e.member, map[string]any{"category_id": cat.ID, "title": "Laptop will not boot"})
	if r.Code != http.StatusCreated {
		t.Fatalf("create ticket = %d: %s", r.Code, r.Body)
	}
	r.json(t, &tk)
	if tk.Key != "IT-0001" {
		t.Fatalf("key = %s, want IT-0001", tk.Key)
	}
	base := "/helpdesk/tickets/" + tk.ID

	for _, step := range []struct {
		name   string
		method string
		path   string
		who    user
		body   map[string]any
		want   int
	}{
		{"agent claims", http.MethodPost, base + "/assign", e.agent, map[string]any{"assignee_id": e.agent.ID}, http.StatusOK},
		{"agent starts work", http.MethodPost, base + "/status", e.agent, map[string]any{"status": "in_progress"}, http.StatusOK},
		{"requester cannot resolve", http.MethodPost, base + "/status", e.member, map[string]any{"status": "resolved"}, http.StatusForbidden},
		{"internal note", http.MethodPost, base + "/comments", e.agent, map[string]any{"body": "Battery swap", "internal": true}, http.StatusCreated},
		{"requester cannot write internal", http.MethodPost, base + "/comments", e.member, map[string]any{"body": "x", "internal": true}, http.StatusForbidden},
		{"agent resolves", http.MethodPost, base + "/status", e.agent, map[string]any{"status": "resolved"}, http.StatusOK},
		{"requester closes", http.MethodPost, base + "/status", e.member, map[string]any{"status": "closed"}, http.StatusOK},
	} {
		if r := e.call(step.method, step.path, step.who, step.body); r.Code != step.want {
			t.Fatalf("%s: %d, want %d: %s", step.name, r.Code, step.want, r.Body)
		}
	}

	var detail struct {
		Status   string `json:"status"`
		Comments []struct {
			Internal bool `json:"internal"`
		} `json:"comments"`
	}
	e.call(http.MethodGet, base, e.member, nil).json(t, &detail)
	if detail.Status != "closed" || len(detail.Comments) != 0 {
		t.Errorf("requester's view = %+v; closed, and the internal note hidden", detail)
	}

	names := strings.Join(e.bus.names(), " ")
	for _, want := range []string{"helpdesk.ticket_created", "helpdesk.ticket_assigned", "helpdesk.ticket_resolved", "helpdesk.comment_added"} {
		if !strings.Contains(names, want) {
			t.Errorf("events %q do not include %s", names, want)
		}
	}
}

// TestTicketWithAFileOnTheRealRegistry checks the file path on the real
// registry: a file of the requester is attached and downloadable by them only.
func TestTicketWithAFileOnTheRealRegistry(t *testing.T) {
	e := newEnv(t)
	e.start(helpdesk.New())
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", e.admin, map[string]any{"name": "IT", "prefix": "IT"}).json(t, &q)
	var cat struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/categories", e.admin, map[string]any{"queue_id": q.ID, "name": "Laptop"}).json(t, &cat)

	var up struct {
		AttachmentID string `json:"attachment_id"`
	}
	e.call(http.MethodPost, "/helpdesk/uploads", e.member, map[string]any{"file_name": "screen.png", "size": 5, "mime_type": "image/png"}).json(t, &up)
	e.store.objects["helpdesk/"+e.ws+"/"+up.AttachmentID] = 5
	r := e.call(http.MethodPost, "/helpdesk/tickets", e.member, map[string]any{"category_id": cat.ID, "title": "Screen", "attachment_ids": []string{up.AttachmentID}})
	if r.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", r.Code, r.Body)
	}
	var tk struct {
		ID string `json:"id"`
	}
	r.json(t, &tk)
	if r := e.call(http.MethodGet, "/helpdesk/attachments/"+up.AttachmentID, e.member, nil); r.Code != http.StatusOK {
		t.Errorf("download = %d: %s", r.Code, r.Body)
	}
	if r := e.call(http.MethodGet, "/helpdesk/attachments/"+up.AttachmentID, e.outsider, nil); r.Code != http.StatusForbidden {
		t.Errorf("outsider download = %d, want 403", r.Code)
	}
}

// TestWithoutTheModuleThereAreNoRoutes: the module is optional (PLT-11).
func TestWithoutTheModuleThereAreNoRoutes(t *testing.T) {
	e := newEnv(t)
	e.start()
	if r := e.call(http.MethodGet, "/helpdesk/queues", e.member, nil); r.Code != http.StatusNotFound {
		t.Errorf("GET /helpdesk/queues with the module off = %d, want 404", r.Code)
	}
}

// TestTheModuleRegistersItsCardCommandActionAndRules: the parts chat and
// notify read from the real registry (HD-03, HD-08, HD-20).
func TestTheModuleRegistersItsCardCommandActionAndRules(t *testing.T) {
	e := newEnv(t)
	e.start(helpdesk.New())

	if len(e.cards.kinds) != 1 || e.cards.kinds[0].Kind != "helpdesk.ticket" || e.cards.kinds[0].Module != "helpdesk" {
		t.Errorf("card kinds = %+v, want helpdesk.ticket of helpdesk", e.cards.kinds)
	}
	if len(e.ext.commands) != 1 || e.ext.commands[0].Name != "ticket" || e.ext.commands[0].Usage != "/ticket <title>" {
		t.Errorf("commands = %+v, want /ticket", e.ext.commands)
	}
	if len(e.ext.actions) != 1 || e.ext.actions[0].ID != "helpdesk.create_from_message" {
		t.Errorf("actions = %+v, want helpdesk.create_from_message", e.ext.actions)
	}
	rules := e.rules.Rules()
	for _, name := range []string{"helpdesk.ticket_created", "helpdesk.ticket_assigned", "helpdesk.status_changed", "helpdesk.comment_added"} {
		if len(rules[name]) != 1 {
			t.Errorf("rules[%s] = %d, want 1", name, len(rules[name]))
		}
	}
	if len(rules["helpdesk.ticket_resolved"]) != 0 {
		t.Errorf("a resolved ticket has its own rule; want status_changed only")
	}
}

// TestTheQueueCardFollowsTheTicketOnTheRealRegistry: the modules own event
// handlers post the card after the commit and update it on every change.
func TestTheQueueCardFollowsTheTicketOnTheRealRegistry(t *testing.T) {
	e := newEnv(t)
	e.start(helpdesk.New())
	channelID := newID()
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", e.admin, map[string]any{"name": "IT", "prefix": "IT", "channel_id": channelID}).json(t, &q)
	if r := e.call(http.MethodPut, "/helpdesk/queues/"+q.ID+"/agents", e.admin, map[string]any{"user_ids": []string{e.agent.ID}}); r.Code != http.StatusNoContent {
		t.Fatalf("set agents = %d: %s", r.Code, r.Body)
	}
	var cat struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/categories", e.admin, map[string]any{"queue_id": q.ID, "name": "Laptop"}).json(t, &cat)

	var tk struct {
		ID string `json:"id"`
	}
	r := e.call(http.MethodPost, "/helpdesk/tickets", e.member, map[string]any{"category_id": cat.ID, "title": "Laptop will not boot"})
	if r.Code != http.StatusCreated {
		t.Fatalf("create ticket = %d: %s", r.Code, r.Body)
	}
	r.json(t, &tk)

	if len(e.cards.posts) != 1 || e.cards.posts[0].ChannelID != channelID || e.cards.posts[0].Kind != "helpdesk.ticket" {
		t.Fatalf("posts = %+v, want one card in the queue channel", e.cards.posts)
	}
	if got := e.cards.posts[0].SenderID; got != e.member.ID {
		t.Errorf("card sender = %q, want the requester %q", got, e.member.ID)
	}
	if r := e.call(http.MethodPost, "/helpdesk/tickets/"+tk.ID+"/assign", e.agent, map[string]any{"assignee_id": e.agent.ID}); r.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", r.Code, r.Body)
	}
	if e.cards.updates == 0 {
		t.Error("claiming did not update the card")
	}
	for _, c := range e.cards.byID {
		p, ok := c.Payload.(usecase.CardPayload)
		if !ok {
			t.Fatalf("payload is %T", c.Payload)
		}
		if p.TicketID != tk.ID || p.Status != "open" || p.AssigneeID == nil || *p.AssigneeID != e.agent.ID {
			t.Errorf("card payload = %+v, want the claimed ticket", p)
		}
	}
}

// TestTheTicketCommandOnTheRealRegistry opens a ticket in the caller's
// channel through the registered command and puts its card there.
func TestTheTicketCommandOnTheRealRegistry(t *testing.T) {
	e := newEnv(t)
	e.start(helpdesk.New())
	channelID := newID()
	var q struct {
		ID string `json:"id"`
	}
	e.call(http.MethodPost, "/helpdesk/queues", e.admin, map[string]any{"name": "IT", "prefix": "IT"}).json(t, &q)
	e.call(http.MethodPost, "/helpdesk/categories", e.admin, map[string]any{"queue_id": q.ID, "name": "Laptop", "is_default": true}).json(t, &struct{}{})

	ctx := app.WithPrincipal(context.Background(), app.Principal{UserID: e.member.ID, WorkspaceID: e.ws, Role: app.RoleMember})
	if _, err := e.ext.commands[0].Handler(ctx, app.CommandInput{ChannelID: channelID, Args: "Laptop is dead"}); err != nil {
		t.Fatal(err)
	}
	if len(e.cards.posts) != 1 || e.cards.posts[0].ChannelID != channelID {
		t.Fatalf("posts = %+v, want the requester card in the command's channel", e.cards.posts)
	}
	var list struct {
		Items []struct {
			Key         string `json:"key"`
			Title       string `json:"title"`
			RequesterID string `json:"requester_id"`
		} `json:"items"`
	}
	e.call(http.MethodGet, "/helpdesk/tickets", e.member, nil).json(t, &list)
	if len(list.Items) != 1 || list.Items[0].Key != "IT-0001" || list.Items[0].Title != "Laptop is dead" || list.Items[0].RequesterID != e.member.ID {
		t.Errorf("tickets = %+v, want the command's ticket", list.Items)
	}
}
