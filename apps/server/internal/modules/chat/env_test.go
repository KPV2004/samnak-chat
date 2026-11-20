package chat_test

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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
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
// Tests isolate themselves with their own workspaces and users.
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
		if err := migrations.Run(ctx, db, "chat", chat.Migrations); err != nil {
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

// fakeMembers is the workspace module's MembershipResolver, backed by a map.
type fakeMembers struct {
	mu    sync.Mutex
	roles map[[2]string]app.Role
}

func (f *fakeMembers) Role(_ context.Context, workspaceID, userID string) (app.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.roles[[2]string{workspaceID, userID}]; ok {
		return r, nil
	}
	return "", fmt.Errorf("not a member: %w", app.ErrForbidden)
}

func (f *fakeMembers) remove(workspaceID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.roles, [2]string{workspaceID, userID})
}

func (f *fakeMembers) set(workspaceID, userID string, r app.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.roles[[2]string{workspaceID, userID}] = r
}

// fakePolicy is the workspace module's Authorizer and ActionRegistry.
type fakePolicy struct {
	mu      sync.Mutex
	allowed map[workspace.Action]map[app.Role]bool
}

func (p *fakePolicy) Register(a workspace.Action, roles ...app.Role) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := map[app.Role]bool{}
	for _, r := range roles {
		set[r] = true
	}
	p.allowed[a] = set
	return nil
}

func (p *fakePolicy) Can(pr app.Principal, a workspace.Action) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.allowed[a][pr.Role]
}

type fakeDirectory struct {
	mu    sync.Mutex
	users map[string]auth.UserSummary
}

func (d *fakeDirectory) Users(_ context.Context, ids []string) ([]auth.UserSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []auth.UserSummary
	for _, id := range ids {
		if u, ok := d.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// delivery is one SendToUsers call.
type delivery struct {
	Users []string
	Type  string
	Data  json.RawMessage
}

// fakeRealtime records deliveries.
type fakeRealtime struct {
	mu       sync.Mutex
	list     []delivery
	handlers map[string]app.ClientHandler
}

func (f *fakeRealtime) SendToUsers(_ context.Context, ids []string, env app.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = append(f.list, delivery{Users: append([]string(nil), ids...), Type: env.Type, Data: env.Data})
	return nil
}
func (f *fakeRealtime) Online(context.Context, []string) ([]string, error) { return nil, nil }
func (f *fakeRealtime) DisconnectDevice(context.Context, string) error     { return nil }

func (f *fakeRealtime) Handle(typ string, h app.ClientHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handlers == nil {
		f.handlers = map[string]app.ClientHandler{}
	}
	f.handlers[typ] = h
}

// client plays a WebSocket client: it hands a frame to the handler chat
// registered, as the hub does after auth, and returns the handler's error.
func (f *fakeRealtime) client(typ string, from app.Identity, data string) error {
	f.mu.Lock()
	h := f.handlers[typ]
	f.mu.Unlock()
	if h == nil {
		return fmt.Errorf("no handler for %q", typ)
	}
	return h(context.Background(), from, json.RawMessage(data))
}

// of returns the deliveries of one type.
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

// fakeStorage is an app.ObjectStorage that keeps objects in memory: the test
// "uploads" with put, and Stat sees what was put.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]int64
	puts    []putCall
	gets    []getCall
}

type putCall struct {
	Key, ContentType string
	Size             int64
	TTL              time.Duration
}

type getCall struct {
	Key, FileName string
	TTL           time.Duration
}

func (f *fakeStorage) PresignPut(_ context.Context, key, contentType string, size int64, ttl time.Duration) (app.PresignedRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts = append(f.puts, putCall{key, contentType, size, ttl})
	return app.PresignedRequest{
		URL: "https://storage.test/" + key + "?sig=put", Method: "PUT", ExpiresAt: time.Now().Add(ttl),
		Headers: map[string]string{"Content-Type": contentType, "Content-Length": fmt.Sprint(size)},
	}, nil
}

func (f *fakeStorage) PresignGet(_ context.Context, key, fileName string, ttl time.Duration) (app.PresignedRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, getCall{key, fileName, ttl})
	return app.PresignedRequest{URL: "https://storage.test/" + key + "?sig=get", Method: "GET", ExpiresAt: time.Now().Add(ttl)}, nil
}

func (f *fakeStorage) Stat(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if size, ok := f.objects[key]; ok {
		return size, nil
	}
	return 0, fmt.Errorf("object %q: %w", key, app.ErrNotFound)
}

// put stores an object as if the client had uploaded it.
func (f *fakeStorage) put(key string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = size
}

type eventLog struct {
	mu   sync.Mutex
	list []chat.MessageCreated
}

func (l *eventLog) add(_ context.Context, e app.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, e.(chat.MessageCreated))
	return nil
}

func (l *eventLog) all() []chat.MessageCreated {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]chat.MessageCreated(nil), l.list...)
}

// testWriter sends log output to the test log, so a 500 explains itself.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

type user struct {
	ID, Token, Name string
}

type env struct {
	t       *testing.T
	h       http.Handler
	pool    *pgxpool.Pool
	dir     *fakeDirectory
	auth    fakeAuth
	members *fakeMembers
	rt      *fakeRealtime
	cards   app.CardRegistry
	svc     *app.Services
	storage *fakeStorage
	events  *eventLog
	bus     *eventbus.Bus
}

// newEnv wires the chat module to a real database through the Registry, with
// fakes for what workspace, auth and realtime provide.
func newEnv(t *testing.T) *env { return newEnvWith(t, &fakeRealtime{}) }

// newEnvWith is newEnv with a chosen Realtime; nil leaves Deps.Realtime unset.
func newEnvWith(t *testing.T, rt *fakeRealtime) *env {
	return newEnvWithRules(t, rt, app.NoopNotificationRegistry{})
}

// newEnvWithRules is newEnvWith with a chosen notification registry, so a test
// can capture and run chat's rules.
func newEnvWithRules(t *testing.T, rt *fakeRealtime, rules app.NotificationRegistry) *env {
	t.Helper()
	url := sharedDB(t)
	pool, err := database.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	e := &env{
		t: t, pool: pool, dir: &fakeDirectory{users: map[string]auth.UserSummary{}}, auth: fakeAuth{},
		members: &fakeMembers{roles: map[[2]string]app.Role{}}, rt: rt, events: &eventLog{},
		storage: &fakeStorage{objects: map[string]int64{}},
	}
	policy := &fakePolicy{allowed: map[workspace.Action]map[app.Role]bool{}}
	svc := app.NewServices()
	app.Provide[app.Authenticator](svc, e.auth)
	app.Provide[app.MembershipResolver](svc, e.members)
	app.Provide[workspace.Authorizer](svc, policy)
	app.Provide[workspace.ActionRegistry](svc, policy)
	app.Provide[auth.Directory](svc, e.dir)

	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	e.bus = eventbus.New(logger)
	e.bus.Subscribe("chat.message_created", e.events.add)

	mod := chat.New()
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"chat"})
	if err != nil {
		t.Fatal(err)
	}
	deps := &app.Deps{
		Logger: logger, Services: svc, Bus: e.bus, Tx: database.NewTxManager(pool),
		Notifications: rules, Platform: map[string]any{"db": pool}, Storage: e.storage,
	}
	if rt != nil {
		deps.Realtime = rt
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
	e.svc = svc
	e.cards, _ = app.Lookup[app.CardRegistry](svc)
	return e
}

// newWorkspace returns a fresh workspace ID. Users join it with join.
func (e *env) newWorkspace() string { return uuid.Must(uuid.NewV7()).String() }

// newUser makes a user who is not in any workspace yet.
func (e *env) newUser(name string) user {
	id := uuid.Must(uuid.NewV7()).String()
	u := user{ID: id, Token: "tok-" + id, Name: name}
	e.auth[u.Token] = app.Identity{UserID: id, DeviceID: "dev-" + id}
	e.dir.mu.Lock()
	e.dir.users[id] = auth.UserSummary{ID: id, DisplayName: name}
	e.dir.mu.Unlock()
	return u
}

// join makes u an active member of ws with a role.
func (e *env) join(ws string, u user, role app.Role) {
	e.members.set(ws, u.ID, role)
}

// member is newUser plus join as a plain member.
func (e *env) member(ws, name string) user {
	u := e.newUser(name)
	e.join(ws, u, app.RoleMember)
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

// call sends a request to /api/v1<path> as u in workspace ws.
func (e *env) call(method, path string, u user, ws string, body any) resp {
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

// Response shapes the tests decode.
type channelBody struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Name         string           `json:"name"`
	Topic        string           `json:"topic"`
	LastSeq      int64            `json:"last_seq"`
	LastReadSeq  int64            `json:"last_read_seq"`
	UnreadCount  int64            `json:"unread_count"`
	MentionCount int64            `json:"mention_count"`
	MemberCount  int64            `json:"member_count"`
	DMUserIDs    []string         `json:"dm_user_ids"`
	ArchivedAt   *string          `json:"archived_at"`
	PinnedAt     *string          `json:"pinned_at"`
	LastMessage  *lastMessageBody `json:"last_message"`
}

type lastMessageBody struct {
	ID        string `json:"id"`
	SenderID  string `json:"sender_id"`
	Kind      string `json:"kind"`
	Preview   string `json:"preview"`
	CreatedAt string `json:"created_at"`
}

type messageBody struct {
	ID              string   `json:"id"`
	ChannelID       string   `json:"channel_id"`
	Seq             int64    `json:"seq"`
	SenderID        string   `json:"sender_id"`
	ClientMsgID     string   `json:"client_msg_id"`
	Kind            string   `json:"kind"`
	Body            string   `json:"body"`
	ThreadRootID    *string  `json:"thread_root_id"`
	ReplyCount      int64    `json:"reply_count"`
	Mentions        []string `json:"mentions"`
	MentionsChannel bool     `json:"mentions_channel"`
	EditedAt        *string  `json:"edited_at"`
	DeletedAt       *string  `json:"deleted_at"`

	Payload      json.RawMessage  `json:"payload"`
	FallbackText string           `json:"fallback_text"`
	Attachments  []attachmentBody `json:"attachments"`
	Reactions    []reactionBody   `json:"reactions"`
}

type attachmentBody struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	IsImage  bool   `json:"is_image"`
}

type reactionBody struct {
	Emoji string `json:"emoji"`
	Count int64  `json:"count"`
	Me    bool   `json:"me"`
}

type pageBody struct {
	Items   []messageBody `json:"items"`
	HasMore bool          `json:"has_more"`
}

func (e *env) createChannel(u user, ws, typ, name string, memberIDs ...string) channelBody {
	e.t.Helper()
	body := map[string]any{"type": typ, "name": name}
	if len(memberIDs) > 0 {
		body["member_ids"] = memberIDs
	}
	r := e.call("POST", "/chat/channels", u, ws, body)
	if r.Code != 201 {
		e.t.Fatalf("create channel %q = %d %s", name, r.Code, r.Body)
	}
	var c channelBody
	r.json(e.t, &c)
	return c
}

func (e *env) send(u user, ws, channelID, text string) messageBody {
	e.t.Helper()
	return e.sendWith(u, ws, channelID, uuid.NewString(), text, "")
}

func (e *env) sendWith(u user, ws, channelID, clientID, text, root string) messageBody {
	e.t.Helper()
	body := map[string]any{"client_msg_id": clientID, "body": text}
	if root != "" {
		body["thread_root_id"] = root
	}
	r := e.call("POST", "/chat/channels/"+channelID+"/messages", u, ws, body)
	if r.Code != 201 && r.Code != 200 {
		e.t.Fatalf("send = %d %s", r.Code, r.Body)
	}
	var m messageBody
	r.json(e.t, &m)
	return m
}

func (e *env) getChannel(u user, ws, id string) channelBody {
	e.t.Helper()
	r := e.call("GET", "/chat/channels/"+id, u, ws, nil)
	if r.Code != 200 {
		e.t.Fatalf("get channel = %d %s", r.Code, r.Body)
	}
	var c channelBody
	r.json(e.t, &c)
	return c
}

func (e *env) messages(u user, ws, channelID, query string) pageBody {
	e.t.Helper()
	r := e.call("GET", "/chat/channels/"+channelID+"/messages"+query, u, ws, nil)
	if r.Code != 200 {
		e.t.Fatalf("list messages = %d %s", r.Code, r.Body)
	}
	var p pageBody
	r.json(e.t, &p)
	return p
}
