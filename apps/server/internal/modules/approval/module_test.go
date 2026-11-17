package approval_test

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
	"sort"
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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
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
// approval set. Tests isolate themselves with fresh workspace and user IDs.
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
		if err := dbmigrations.Run(ctx, db, "approval", approval.Migrations); err != nil {
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

// fakeGroups answers ManagersOf from a table.
type fakeGroups map[string][]string

func (f fakeGroups) ManagersOf(_ context.Context, _, userID string) ([]string, error) {
	return f[userID], nil
}

// fakeCards is the chat card registry: it keeps the cards in memory.
type fakeCards struct {
	mu       sync.Mutex
	kinds    []app.CardKind
	cards    map[string]app.Card // by message ID
	posts    int
	updates  int
	channels map[string]string
}

func newFakeCards() *fakeCards {
	return &fakeCards{cards: map[string]app.Card{}, channels: map[string]string{}}
}

func (f *fakeCards) Register(k app.CardKind) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kinds = append(f.kinds, k)
}

func (f *fakeCards) Post(_ context.Context, c app.Card) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts++
	id := newID()
	b, err := json.Marshal(c.Payload)
	if err != nil {
		return "", err
	}
	c.Payload = json.RawMessage(b)
	f.cards[id] = c
	return id, nil
}

func (f *fakeCards) Update(_ context.Context, _, messageID string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[messageID]
	if !ok {
		return fmt.Errorf("card %s: %w", messageID, app.ErrNotFound)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.Payload = json.RawMessage(b)
	f.cards[messageID] = c
	f.updates++
	return nil
}

func (f *fakeCards) DirectChannel(_ context.Context, _ string, userIDs []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := append([]string(nil), userIDs...)
	sort.Strings(ids)
	key := strings.Join(ids, "+")
	if id, ok := f.channels[key]; ok {
		return id, nil
	}
	f.channels[key] = newID()
	return f.channels[key], nil
}

// card returns the single card of a message.
func (f *fakeCards) card(t *testing.T, messageID string) (app.Card, cardBody) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[messageID]
	if !ok {
		t.Fatalf("no card %q", messageID)
	}
	var body cardBody
	if err := json.Unmarshal(c.Payload.(json.RawMessage), &body); err != nil {
		t.Fatal(err)
	}
	return c, body
}

type cardBody struct {
	RequestID     string         `json:"request_id"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	Fields        map[string]any `json:"fields"`
	RequesterID   string         `json:"requester_id"`
	RequesterName string         `json:"requester_name"`
	ApproverID    string         `json:"approver_id"`
	ApproverName  string         `json:"approver_name"`
	Reason        string         `json:"reason"`
	DecidedAt     *time.Time     `json:"decided_at"`
	Version       int64          `json:"version"`
}

type storedObject struct{ size int64 }

// fakeStorage presigns without a network and knows which objects "exist".
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]storedObject
}

func (s *fakeStorage) PresignPut(_ context.Context, key, ct string, _ int64, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/put/" + key, Method: "PUT", Headers: map[string]string{"Content-Type": ct}, ExpiresAt: time.Now().Add(ttl)}, nil
}

func (s *fakeStorage) PresignGet(_ context.Context, key, _ string, ttl time.Duration) (app.PresignedRequest, error) {
	return app.PresignedRequest{URL: "https://store.test/get/" + key, Method: "GET", ExpiresAt: time.Now().Add(ttl)}, nil
}

func (s *fakeStorage) Stat(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o, ok := s.objects[key]; ok {
		return o.size, nil
	}
	return 0, fmt.Errorf("object: %w", app.ErrNotFound)
}

func (s *fakeStorage) put(key string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = storedObject{size}
}

// outboxSim stands in for the real outbox: Publish inside a transaction is
// held until the commit, must be of a declared type and must survive a JSON
// round trip, as the outbox requires (PLT-10). Outside a transaction it runs
// the handlers at once.
type outboxSim struct {
	*eventbus.Bus
	inner    app.TxManager
	mu       sync.Mutex
	declared map[string]reflect.Type
}

type collectorKey struct{}

type collector struct{ events []app.Event }

func newOutboxSim(log *slog.Logger, tx app.TxManager) *outboxSim {
	return &outboxSim{Bus: eventbus.New(log), inner: tx, declared: map[string]reflect.Type{}}
}

func (o *outboxSim) Declare(protos ...app.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range protos {
		o.declared[p.EventName()] = reflect.TypeOf(p)
	}
}

func (o *outboxSim) Publish(ctx context.Context, evt app.Event) error {
	c, ok := ctx.Value(collectorKey{}).(*collector)
	if !ok {
		return o.Bus.Publish(ctx, evt)
	}
	o.mu.Lock()
	typ, declared := o.declared[evt.EventName()]
	o.mu.Unlock()
	if !declared || typ != reflect.TypeOf(evt) {
		return fmt.Errorf("event %q published in a transaction without being declared", evt.EventName())
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	back := reflect.New(typ)
	if err := json.Unmarshal(b, back.Interface()); err != nil {
		return err
	}
	if !reflect.DeepEqual(back.Elem().Interface(), evt) {
		return fmt.Errorf("event %q does not round-trip through JSON: %+v vs %+v", evt.EventName(), back.Elem().Interface(), evt)
	}
	c.events = append(c.events, back.Elem().Interface().(app.Event))
	return nil
}

// Do implements app.TxManager: the events go out only after the commit.
func (o *outboxSim) Do(ctx context.Context, fn func(context.Context) error) error {
	if _, nested := ctx.Value(collectorKey{}).(*collector); nested {
		return fn(ctx)
	}
	c := &collector{}
	err := o.inner.Do(context.WithValue(ctx, collectorKey{}, c), fn)
	if err != nil {
		return err
	}
	for _, evt := range c.events {
		_ = o.Bus.Publish(context.WithoutCancel(ctx), evt)
	}
	return nil
}

type user struct {
	ID, Token string
}

type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	h        http.Handler
	bus      *outboxSim
	cards    *fakeCards
	store    *fakeStorage
	rules    *app.NotificationRules
	auth     fakeAuth
	members  fakeMembers
	names    fakeDirectory
	groups   fakeGroups
	services *app.Services
	deps     *app.Deps
	ws       string
	alice    user // the requester
	bob      user // the approver
	carol    user // a third member
	boss     user // alice's group manager
}

// newEnv wires the module with chat's, workspace's and auth's services faked.
// withCards, withGroups and withNotifications let a test switch those off.
func newEnv(t *testing.T) *env { return newEnvWith(t, true, true) }

func newEnvWith(t *testing.T, withCards, withGroups bool) *env {
	t.Helper()
	pool := sharedDB(t)
	e := &env{
		t: t, pool: pool, auth: fakeAuth{}, members: fakeMembers{}, names: fakeDirectory{}, groups: fakeGroups{},
		cards: newFakeCards(), store: &fakeStorage{objects: map[string]storedObject{}}, rules: app.NewNotificationRules(), ws: newID(),
	}
	e.alice, e.bob, e.carol, e.boss = e.newUser("Alice"), e.newUser("Bob"), e.newUser("Carol"), e.newUser("Boss")
	e.groups[e.alice.ID] = []string{e.boss.ID}

	e.services = app.NewServices()
	app.Provide[app.Authenticator](e.services, e.auth)
	app.Provide[app.MembershipResolver](e.services, e.members)
	app.Provide[auth.Directory](e.services, e.names)
	if withCards {
		app.Provide[app.CardRegistry](e.services, e.cards)
	}
	if withGroups {
		app.Provide[workspace.GroupDirectory](e.services, e.groups)
	}

	logger := slogNew(testWriter{t})
	e.bus = newOutboxSim(logger, database.NewTxManager(pool))
	e.deps = &app.Deps{
		Logger: logger, Services: e.services, Bus: e.bus, Notifications: e.rules, Tx: e.bus, Storage: e.store,
		Platform: map[string]any{"db": pool},
	}
	return e
}

func (e *env) newUser(name string) user {
	id := newID()
	u := user{ID: id, Token: "tok-" + id}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.members[e.ws+"/"+id] = app.RoleMember
	e.names[id] = name
	return u
}

func (e *env) start() {
	e.t.Helper()
	reg := app.NewRegistry(approval.New())
	mods, err := reg.Enable([]string{"approval"})
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

func (e *env) callIn(ws, method, path string, u user, body any) resp {
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
	if ws != "" {
		req.Header.Set("X-Workspace-ID", ws)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{Code: rec.Code, Body: rec.Body.Bytes()}
}

func (e *env) call(method, path string, u user, body any) resp {
	return e.callIn(e.ws, method, path, u, body)
}

// requestBody is the Request of the plan.
type requestBody struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Status      string         `json:"status"`
	RequesterID string         `json:"requester_id"`
	ApproverID  string         `json:"approver_id"`
	Payload     map[string]any `json:"payload"`
	Reason      string         `json:"reason"`
	DecidedAt   *time.Time     `json:"decided_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	MessageID   *string        `json:"message_id"`
	History     []struct {
		ActorID string `json:"actor_id"`
		Action  string `json:"action"`
		Reason  string `json:"reason"`
	} `json:"history"`
	Attachments []struct {
		ID       string `json:"id"`
		FileName string `json:"file_name"`
		Size     int64  `json:"size"`
	} `json:"attachments"`
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func leave() map[string]any {
	return map[string]any{"type": "leave", "payload": map[string]any{
		"leave_type": "annual", "start_date": "2026-12-01", "end_date": "2026-12-03", "reason": "trip",
	}}
}

func (e *env) submit(u user, body map[string]any) requestBody {
	e.t.Helper()
	r := e.call("POST", "/approval/requests", u, body)
	if r.Code != http.StatusCreated {
		e.t.Fatalf("submit = %d %s", r.Code, r.Body)
	}
	var out requestBody
	r.json(e.t, &out)
	return out
}

func withApprover(body map[string]any, u user) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	out["approver_id"] = u.ID
	return out
}

func (e *env) fetch(u user, id string) requestBody {
	e.t.Helper()
	r := e.call("GET", "/approval/requests/"+id, u, nil)
	if r.Code != http.StatusOK {
		e.t.Fatalf("get = %d %s", r.Code, r.Body)
	}
	var out requestBody
	r.json(e.t, &out)
	return out
}

func (e *env) code(method, path string, u user, body any) (int, errorBody) {
	e.t.Helper()
	r := e.call(method, path, u, body)
	var eb errorBody
	_ = json.Unmarshal(r.Body, &eb)
	return r.Code, eb
}
