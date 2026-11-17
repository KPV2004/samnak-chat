package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

const (
	ws    = "0193f0f0-0000-7000-8000-0000000000a1"
	ch    = "0193f0f0-0000-7000-8000-0000000000c1"
	ch2   = "0193f0f0-0000-7000-8000-0000000000c2"
	alice = "0193f0f0-0000-7000-8000-0000000000b1"
	bob   = "0193f0f0-0000-7000-8000-0000000000b2"
	carol = "0193f0f0-0000-7000-8000-0000000000b3"
	admin = "0193f0f0-0000-7000-8000-0000000000b4"
)

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	if id, ok := f[token]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

type fakeRoles map[string]app.Role

func (f fakeRoles) Role(_ context.Context, workspaceID, userID string) (app.Role, error) {
	if r, ok := f[workspaceID+"/"+userID]; ok {
		return r, nil
	}
	return "", fmt.Errorf("not a member: %w", app.ErrForbidden)
}

// memRepo is a small in-memory usecase.Repository: the handlers are what is
// under test here, the SQL has its own tests.
type memRepo struct {
	mu    sync.Mutex
	tasks map[string]domain.Task
}

func (r *memRepo) Insert(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return t, nil
}

func (r *memRepo) Get(_ context.Context, workspaceID, id string) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[id]; ok && t.WorkspaceID == workspaceID {
		return t, nil
	}
	return domain.Task{}, fmt.Errorf("task: %w", app.ErrNotFound)
}

func (r *memRepo) GetForUpdate(ctx context.Context, workspaceID, id string) (domain.Task, error) {
	return r.Get(ctx, workspaceID, id)
}

func (r *memRepo) Update(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return t, nil
}

func (r *memRepo) SetMessage(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (r *memRepo) List(_ context.Context, workspaceID string, f usecase.Filter, afterID string, limit int) ([]domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Task
	for _, t := range r.tasks {
		if t.WorkspaceID != workspaceID || (f.AssigneeID != "" && t.AssigneeID != f.AssigneeID) || (f.CreatorID != "" && t.CreatorID != f.CreatorID) ||
			(len(f.Statuses) > 0 && !slices.Contains(f.Statuses, t.Status)) || (afterID != "" && t.ID >= afterID) {
			continue
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b domain.Task) int { return strings.Compare(b.ID, a.ID) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memRepo) ClaimDueSoon(context.Context, time.Time, time.Time, int) ([]usecase.Due, error) {
	return nil, nil
}
func (r *memRepo) ClaimOverdue(context.Context, time.Time, int) ([]usecase.Due, error) {
	return nil, nil
}

type readers map[string]bool

func (r readers) CanRead(_ context.Context, workspaceID, channelID, userID string) (bool, error) {
	return workspaceID == ws && r[channelID+"/"+userID], nil
}

type activeMembers map[string]bool

func (m activeMembers) IsActiveMember(_ context.Context, _, userID string) (bool, error) {
	return m[userID], nil
}

type noopTx struct{}

func (noopTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type env struct {
	t    *testing.T
	api  humatest.TestAPI
	repo *memRepo
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// newEnvWith is newEnv with the events the use cases publish and the logger
// the handlers write to, so a test can change the first and watch the second.
func newEnvWith(t *testing.T, events usecase.Events, log *slog.Logger) *env {
	t.Helper()
	repo := &memRepo{tasks: map[string]domain.Task{}}
	uc := usecase.New(usecase.Deps{
		Tx: noopTx{}, Repo: repo, Events: events,
		Channels: readers{ch + "/" + alice: true, ch + "/" + bob: true, ch + "/" + admin: true, ch + "/" + carol: true, ch2 + "/" + admin: true},
		Members:  activeMembers{alice: true, bob: true, carol: true, admin: true},
		Log:      log,
	})
	services := app.NewServices()
	app.Provide[app.Authenticator](services, fakeAuth{
		"alice": {UserID: alice, DeviceID: "dev-a"}, "bob": {UserID: bob, DeviceID: "dev-b"},
		"carol": {UserID: carol, DeviceID: "dev-c"}, "admin": {UserID: admin, DeviceID: "dev-d"},
		"outsider": {UserID: "0193f0f0-0000-7000-8000-0000000000ff", DeviceID: "dev-x"},
	})
	app.Provide[app.MembershipResolver](services, fakeRoles{
		ws + "/" + alice: app.RoleMember, ws + "/" + bob: app.RoleMember, ws + "/" + carol: app.RoleMember, ws + "/" + admin: app.RoleAdmin,
	})
	_, api := humatest.New(t)
	httpadapter.New(uc, log).Register(api, services)
	return &env{t: t, api: api, repo: repo}
}

// abortingEvents fails the status write the way the outbox does once the
// request's context has ended: the INSERT is cancelled with the request.
type abortingEvents struct{}

func (abortingEvents) Created(context.Context, domain.Task) error          { return nil }
func (abortingEvents) Assigned(context.Context, domain.Task, string) error { return nil }
func (abortingEvents) Updated(context.Context, domain.Task) error          { return nil }
func (abortingEvents) DueSoon(context.Context, usecase.Due) error          { return nil }
func (abortingEvents) Overdue(context.Context, usecase.Due) error          { return nil }
func (abortingEvents) StatusChanged(ctx context.Context, _ domain.Task, _ domain.Status, _ string) error {
	return fmt.Errorf("eventbus: write %q to the outbox: %w", "task.status_changed", ctx.Err())
}

func headers(token string) []any {
	return []any{"Authorization: Bearer " + token, "X-Workspace-ID: " + ws}
}

type taskBody struct {
	ID              string  `json:"id"`
	ChannelID       string  `json:"channel_id"`
	MessageID       *string `json:"message_id"`
	SourceMessageID *string `json:"source_message_id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	CreatorID       string  `json:"creator_id"`
	AssigneeID      *string `json:"assignee_id"`
	DueAt           *string `json:"due_at"`
	Priority        string  `json:"priority"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type pageBody struct {
	Items      []taskBody `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type result struct {
	Code int
	Body []byte
}

func (r result) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
}

func (r result) errorCode(t *testing.T) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	r.decode(t, &b)
	return b.Error.Code
}

func (e *env) do(method, path, token string, body any) result {
	e.t.Helper()
	args := headers(token)
	if body != nil {
		args = append(args, body)
	}
	var resp = e.api.Do(method, path, args...)
	return result{Code: resp.Code, Body: resp.Body.Bytes()}
}

func (e *env) create(token string, body map[string]any) taskBody {
	e.t.Helper()
	r := e.do(http.MethodPost, "/items", token, body)
	if r.Code != 201 {
		e.t.Fatalf("create = %d %s", r.Code, r.Body)
	}
	var t taskBody
	r.decode(e.t, &t)
	return t
}

func TestCreateReturnsTheTask(t *testing.T) {
	e := newEnv(t)
	got := e.create("alice", map[string]any{
		"channel_id": ch, "title": "Fix login", "description": "see ticket", "assignee_id": bob,
		"due_at": "2026-03-10T09:00:00Z", "priority": "high",
	})
	if got.ID == "" || got.ChannelID != ch || got.Title != "Fix login" || got.Description != "see ticket" || got.CreatorID != alice ||
		got.AssigneeID == nil || *got.AssigneeID != bob || got.Priority != "high" || got.Status != "todo" ||
		got.DueAt == nil || *got.DueAt != "2026-03-10T09:00:00Z" || got.MessageID != nil || got.SourceMessageID != nil {
		t.Fatalf("task = %+v", got)
	}
	bare := e.create("alice", map[string]any{"channel_id": ch, "title": "Plain"})
	if bare.Priority != "normal" || bare.AssigneeID != nil || bare.DueAt != nil {
		t.Fatalf("bare = %+v", bare)
	}
}

func TestCreateErrors(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		name  string
		token string
		body  map[string]any
		code  int
		err   string
	}{
		{"missing title", "alice", map[string]any{"channel_id": ch}, 400, "invalid"},
		{"empty title", "alice", map[string]any{"channel_id": ch, "title": ""}, 400, "invalid"},
		{"blank title", "alice", map[string]any{"channel_id": ch, "title": "   "}, 400, "invalid"},
		{"long title", "alice", map[string]any{"channel_id": ch, "title": strings.Repeat("a", 201)}, 400, "invalid"},
		{"long description", "alice", map[string]any{"channel_id": ch, "title": "x", "description": strings.Repeat("a", 4001)}, 400, "invalid"},
		{"bad priority", "alice", map[string]any{"channel_id": ch, "title": "x", "priority": "urgent"}, 400, "invalid"},
		{"bad channel id", "alice", map[string]any{"channel_id": "nope", "title": "x"}, 400, "invalid"},
		{"channel carol cannot read", "carol", map[string]any{"channel_id": ch2, "title": "x"}, 404, "not_found"},
		{"assignee who cannot read", "alice", map[string]any{"channel_id": ch, "title": "x", "assignee_id": "0193f0f0-0000-7000-8000-0000000000ff"}, 400, "invalid"},
		{"no token", "", map[string]any{"channel_id": ch, "title": "x"}, 401, "unauthorized"},
		{"not a member", "outsider", map[string]any{"channel_id": ch, "title": "x"}, 403, "forbidden"},
	}
	for _, c := range cases {
		r := e.do(http.MethodPost, "/items", c.token, c.body)
		if r.Code != c.code || r.errorCode(t) != c.err {
			t.Errorf("%s: %d %s, want %d %s", c.name, r.Code, r.Body, c.code, c.err)
		}
	}
}

func TestCreateRefusesAWorkspaceInTheBody(t *testing.T) {
	e := newEnv(t)
	// The workspace comes from the header and the Principal only; the schema
	// does not know the property, so Huma rejects it.
	r := e.do(http.MethodPost, "/items", "alice", map[string]any{"channel_id": ch, "title": "x", "workspace_id": "0193f0f0-0000-7000-8000-0000000000a9"})
	if r.Code != 400 || len(e.repo.tasks) != 0 {
		t.Fatalf("create = %d %s, tasks = %d", r.Code, r.Body, len(e.repo.tasks))
	}
}

func TestGet(t *testing.T) {
	e := newEnv(t)
	task := e.create("alice", map[string]any{"channel_id": ch, "title": "x"})

	r := e.do(http.MethodGet, "/items/"+task.ID, "bob", nil)
	var got taskBody
	r.decode(t, &got)
	if r.Code != 200 || got.ID != task.ID {
		t.Fatalf("bob: %d %s", r.Code, r.Body)
	}
	e.repo.tasks[task.ID] = withChannel(e.repo.tasks[task.ID], ch2)
	if r := e.do(http.MethodGet, "/items/"+task.ID, "alice", nil); r.Code != 404 {
		t.Fatalf("a task in a channel alice cannot read = %d, want 404", r.Code)
	}
	if r := e.do(http.MethodGet, "/items/"+task.ID, "admin", nil); r.Code != 200 {
		t.Fatalf("admin reads ch2: %d %s", r.Code, r.Body)
	}
	if r := e.do(http.MethodGet, "/items/nope", "alice", nil); r.Code != 404 {
		t.Fatalf("bad id = %d, want 404", r.Code)
	}
}

func withChannel(t domain.Task, channel string) domain.Task {
	t.ChannelID = channel
	return t
}

func TestListFiltersAndPages(t *testing.T) {
	e := newEnv(t)
	var made []taskBody
	for i := range 5 {
		body := map[string]any{"channel_id": ch, "title": fmt.Sprintf("t%d", i)}
		if i%2 == 0 {
			body["assignee_id"] = bob
		}
		made = append(made, e.create("alice", body))
		time.Sleep(2 * time.Millisecond) // UUIDv7 IDs order by time
	}
	list := func(query, token string) (int, pageBody) {
		r := e.do(http.MethodGet, "/items"+query, token, nil)
		var p pageBody
		if r.Code == 200 {
			r.decode(t, &p)
		}
		return r.Code, p
	}
	titles := func(p pageBody) string {
		out := make([]string, len(p.Items))
		for i, k := range p.Items {
			out[i] = k.Title
		}
		return strings.Join(out, ",")
	}

	code, p := list("", "alice")
	if code != 200 || titles(p) != "t4,t3,t2,t1,t0" || p.NextCursor != nil {
		t.Fatalf("all: %d %s next=%v", code, titles(p), p.NextCursor)
	}
	if _, p = list("?assignee=me", "bob"); titles(p) != "t4,t2,t0" {
		t.Fatalf("bob's = %s", titles(p))
	}
	if _, p = list("?assignee=me", "alice"); titles(p) != "" {
		t.Fatalf("alice's = %s", titles(p))
	}
	if _, p = list("?creator=me", "bob"); titles(p) != "" {
		t.Fatalf("bob created %s", titles(p))
	}

	_, first := list("?limit=2", "alice")
	if titles(first) != "t4,t3" || first.NextCursor == nil || *first.NextCursor != made[3].ID {
		t.Fatalf("page 1 = %s next=%v", titles(first), first.NextCursor)
	}
	_, second := list("?limit=2&cursor="+*first.NextCursor, "alice")
	if titles(second) != "t2,t1" || second.NextCursor == nil {
		t.Fatalf("page 2 = %s", titles(second))
	}
	_, third := list("?limit=2&cursor="+*second.NextCursor, "alice")
	if titles(third) != "t0" || third.NextCursor != nil {
		t.Fatalf("page 3 = %s next=%v", titles(third), third.NextCursor)
	}

	// Status is a comma-separated list.
	r := e.do(http.MethodPatch, "/items/"+made[0].ID, "alice", map[string]any{"status": "doing"})
	if r.Code != 200 {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}
	if _, p = list("?status=doing", "alice"); titles(p) != "t0" {
		t.Fatalf("doing = %s", titles(p))
	}
	if _, p = list("?status=doing,done", "alice"); titles(p) != "t0" {
		t.Fatalf("doing,done = %s", titles(p))
	}
	if _, p = list("?status=todo,doing", "alice"); len(p.Items) != 5 {
		t.Fatalf("todo,doing = %s", titles(p))
	}
}

func TestListRejectsBadQueries(t *testing.T) {
	e := newEnv(t)
	for _, q := range []string{"?assignee=bob", "?creator=any", "?status=blocked", "?due=tomorrow", "?limit=0", "?limit=101", "?channel_id=nope", "?cursor=nope"} {
		if r := e.do(http.MethodGet, "/items"+q, "alice", nil); r.Code != 400 {
			t.Errorf("%s = %d %s, want 400", q, r.Code, r.Body)
		}
	}
}

func TestListWithNothingIsAnEmptyArray(t *testing.T) {
	e := newEnv(t)
	r := e.do(http.MethodGet, "/items", "alice", nil)
	if r.Code != 200 || !strings.Contains(string(r.Body), `"items":[]`) || !strings.Contains(string(r.Body), `"next_cursor":null`) {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}

func TestUpdateTask(t *testing.T) {
	e := newEnv(t)
	task := e.create("alice", map[string]any{"channel_id": ch, "title": "Fix", "assignee_id": bob, "due_at": "2026-03-10T09:00:00Z"})

	r := e.do(http.MethodPatch, "/items/"+task.ID, "bob", map[string]any{"status": "doing", "title": "Fix it", "priority": "low"})
	var got taskBody
	r.decode(t, &got)
	if r.Code != 200 || got.Status != "doing" || got.Title != "Fix it" || got.Priority != "low" || got.AssigneeID == nil || got.DueAt == nil {
		t.Fatalf("patch = %d %s", r.Code, r.Body)
	}

	// null clears; a missing field stays.
	r = e.do(http.MethodPatch, "/items/"+task.ID, "alice", map[string]any{"assignee_id": nil, "due_at": nil})
	got = taskBody{}
	r.decode(t, &got)
	if r.Code != 200 || got.AssigneeID != nil || got.DueAt != nil || got.Status != "doing" || got.Title != "Fix it" {
		t.Fatalf("clear = %d %s", r.Code, r.Body)
	}

	r = e.do(http.MethodPatch, "/items/"+task.ID, "alice", map[string]any{"assignee_id": carol, "due_at": "2026-04-01T00:00:00Z"})
	got = taskBody{}
	r.decode(t, &got)
	if r.Code != 200 || got.AssigneeID == nil || *got.AssigneeID != carol || got.DueAt == nil || *got.DueAt != "2026-04-01T00:00:00Z" {
		t.Fatalf("set = %d %s", r.Code, r.Body)
	}

	// An empty body changes nothing.
	r = e.do(http.MethodPatch, "/items/"+task.ID, "alice", map[string]any{})
	if r.Code != 200 {
		t.Fatalf("empty patch = %d %s", r.Code, r.Body)
	}
}

func TestUpdateErrors(t *testing.T) {
	e := newEnv(t)
	task := e.create("alice", map[string]any{"channel_id": ch, "title": "Fix", "assignee_id": bob})
	cases := []struct {
		name, token string
		id          string
		body        map[string]any
		code        int
	}{
		{"neither creator nor assignee", "carol", task.ID, map[string]any{"title": "x"}, 403},
		{"status by a bystander", "carol", task.ID, map[string]any{"status": "done"}, 403},
		{"admin may", "admin", task.ID, map[string]any{"title": "by admin"}, 200},
		{"unknown id", "alice", "0193f0f0-0000-7000-8000-00000000eeee", map[string]any{"title": "x"}, 404},
		{"bad status", "alice", task.ID, map[string]any{"status": "blocked"}, 400},
		{"bad priority", "alice", task.ID, map[string]any{"priority": "urgent"}, 400},
		{"empty title", "alice", task.ID, map[string]any{"title": ""}, 400},
		{"blank title", "alice", task.ID, map[string]any{"title": "  "}, 400},
		{"bad assignee id", "alice", task.ID, map[string]any{"assignee_id": "nope"}, 400},
		{"assignee outside the workspace", "alice", task.ID, map[string]any{"assignee_id": "0193f0f0-0000-7000-8000-0000000000ff"}, 400},
		{"bad due_at", "alice", task.ID, map[string]any{"due_at": "tomorrow"}, 400},
		{"no token", "", task.ID, map[string]any{"title": "x"}, 401},
	}
	for _, c := range cases {
		if r := e.do(http.MethodPatch, "/items/"+c.id, c.token, c.body); r.Code != c.code {
			t.Errorf("%s: %d %s, want %d", c.name, r.Code, r.Body, c.code)
		}
	}
}

// TestClientAbortIsNotAServerError is the bug found in browser testing: a
// client that hangs up during PATCH cancels the outbox write, which used to be
// answered 500 and logged at ERROR. An abort is the client's doing, not ours.
func TestClientAbortIsNotAServerError(t *testing.T) {
	var logs bytes.Buffer
	e := newEnvWith(t, abortingEvents{}, slog.New(slog.NewTextHandler(&logs, nil)))
	task := e.create("alice", map[string]any{"channel_id": ch, "title": "Draft the agenda"})

	gone, hangUp := context.WithCancel(context.Background())
	hangUp()
	resp := e.api.PatchCtx(gone, "/items/"+task.ID, append(headers("alice"), map[string]any{"status": "doing"})...)
	// 499 is nginx's "client closed request"; the client is gone, so nothing reads it.
	if resp.Code != 499 {
		t.Fatalf("status = %d, want 499 for an aborted request: %s", resp.Code, resp.Body)
	}
	if got := (result{Code: resp.Code, Body: resp.Body.Bytes()}).errorCode(t); got != "client_closed_request" {
		t.Fatalf("error code = %q, want client_closed_request", got)
	}
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("an aborted request was logged as an error: %s", logs.String())
	}
}

func TestOpenAPIDescribesTheNullableFields(t *testing.T) {
	e := newEnv(t)
	spec, err := json.Marshal(e.api.OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Type any `json:"type"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, methods := range doc.Paths {
		for _, op := range methods {
			ids[op.OperationID] = true
		}
	}
	for _, want := range []string{"task-create-item", "task-list-items", "task-get-item", "task-update-item"} {
		if !ids[want] {
			t.Errorf("operation %s is missing", want)
		}
	}
	update := doc.Components.Schemas["UpdateTaskRequest"]
	for _, f := range []string{"assignee_id", "due_at"} {
		types, _ := update.Properties[f].Type.([]any)
		if !slices.Contains(types, any("null")) {
			t.Errorf("%s type = %v, want it to allow null", f, update.Properties[f].Type)
		}
	}
}
