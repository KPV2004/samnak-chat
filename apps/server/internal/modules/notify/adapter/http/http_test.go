package httpadapter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/usecase"
)

const (
	ws     = "0193f0f0-0000-7000-8000-0000000000a1"
	otherW = "0193f0f0-0000-7000-8000-0000000000a2"
	alice  = "0193f0f0-0000-7000-8000-0000000000b1"
	bob    = "0193f0f0-0000-7000-8000-0000000000b2"
	carol  = "0193f0f0-0000-7000-8000-0000000000b3"
)

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

// testRepo is an in-memory usecase.Repository with the same visible rules as
// the Postgres one: per-user key deduplication, newest-first pages and
// ErrNotFound for anything the caller does not own.
type testRepo struct {
	mu   sync.Mutex
	rows []domain.Notification
	seq  int
}

func (r *testRepo) Insert(_ context.Context, n domain.Notification) (domain.Notification, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n.Key != "" {
		for _, row := range r.rows {
			if row.UserID == n.UserID && row.Key == n.Key {
				return domain.Notification{}, false, nil
			}
		}
	}
	r.seq++
	r.rows = append(r.rows, n)
	return n, true, nil
}

func (r *testRepo) List(_ context.Context, workspaceID, userID string, unreadOnly bool, beforeID string, limit int) ([]domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Notification
	for _, row := range r.rows {
		if row.WorkspaceID == workspaceID && row.UserID == userID && (!unreadOnly || row.ReadAt == nil) {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if beforeID != "" {
		cut := -1
		for i, row := range out {
			if row.ID == beforeID {
				cut = i
			}
		}
		if cut < 0 {
			return nil, app.ErrNotFound
		}
		out = out[cut+1:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *testRepo) UnreadCount(_ context.Context, workspaceID, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, row := range r.rows {
		if row.WorkspaceID == workspaceID && row.UserID == userID && row.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (r *testRepo) MarkRead(_ context.Context, workspaceID, userID, id string, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, row := range r.rows {
		if row.ID == id && row.WorkspaceID == workspaceID && row.UserID == userID {
			if row.ReadAt != nil {
				return false, nil
			}
			t := at
			r.rows[i].ReadAt = &t
			return true, nil
		}
	}
	return false, app.ErrNotFound
}

func (r *testRepo) MarkAllRead(_ context.Context, workspaceID, userID string, at time.Time) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for i, row := range r.rows {
		if row.WorkspaceID == workspaceID && row.UserID == userID && row.ReadAt == nil {
			t := at
			r.rows[i].ReadAt = &t
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func (r *testRepo) Prune(context.Context, time.Time) (int64, error) { return 0, nil }

type readCall struct {
	WorkspaceID string
	UserID      string
	IDs         []string
	All         bool
	Unread      int64
}

type fakeEvents struct {
	mu      sync.Mutex
	created []domain.Notification
	read    []readCall
}

func (f *fakeEvents) Created(_ context.Context, n domain.Notification, _ int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, n)
}

func (f *fakeEvents) Read(_ context.Context, workspaceID, userID string, ids []string, all bool, unread int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, readCall{WorkspaceID: workspaceID, UserID: userID, IDs: ids, All: all, Unread: unread})
}

func (f *fakeEvents) readCalls() []readCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]readCall(nil), f.read...)
}

type env struct {
	t    *testing.T
	api  humatest.TestAPI
	repo *testRepo
	ev   *fakeEvents
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo := &testRepo{}
	ev := &fakeEvents{}
	uc := usecase.New(noopTx{}, repo, nil, ev, nil, nil)

	services := app.NewServices()
	app.Provide[app.Authenticator](services, fakeAuth{
		"alice": {UserID: alice, DeviceID: "dev-a"},
		"bob":   {UserID: bob, DeviceID: "dev-b"},
		"carol": {UserID: carol, DeviceID: "dev-c"},
	})
	app.Provide[app.MembershipResolver](services, fakeMembers{
		ws + "/" + alice:     app.RoleMember,
		ws + "/" + bob:       app.RoleMember,
		otherW + "/" + alice: app.RoleGuest,
	})

	_, api := humatest.New(t)
	httpadapter.New(uc, nil).Register(api, services)
	return &env{t: t, api: api, repo: repo, ev: ev}
}

type noopTx struct{}

func (noopTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// seed stores a notification for user and returns its ID.
func (e *env) seed(workspaceID, user, kind, title, key string, at time.Time) string {
	e.t.Helper()
	n := domain.New(app.Notification{
		WorkspaceID: workspaceID, UserID: user, Kind: kind, Title: title, Body: "hi", Link: "/chat/c1",
		Data: map[string]any{"channel_id": "c1"}, Key: key,
	}, uuid.Must(uuid.NewV7()).String(), at)
	if _, _, err := e.repo.Insert(context.Background(), n); err != nil {
		e.t.Fatal(err)
	}
	return n.ID
}

func headers(user string) []any {
	return []any{"Authorization: Bearer " + user, "X-Workspace-ID: " + ws}
}

type notificationBody struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Link      string         `json:"link"`
	Data      map[string]any `json:"data"`
	ReadAt    *time.Time     `json:"read_at"`
	CreatedAt time.Time      `json:"created_at"`
}

type listBody struct {
	Items   []notificationBody `json:"items"`
	HasMore bool               `json:"has_more"`
}

func TestListRequiresAuthAndWorkspace(t *testing.T) {
	e := newEnv(t)
	if got := e.api.Get("/notifications").Code; got != 401 {
		t.Fatalf("no token = %d, want 401", got)
	}
	if got := e.api.Get("/notifications", "Authorization: Bearer alice").Code; got != 400 {
		t.Fatalf("no workspace = %d, want 400", got)
	}
	if got := e.api.Get("/notifications", "Authorization: Bearer carol", "X-Workspace-ID: "+ws).Code; got != 403 {
		t.Fatalf("non-member = %d, want 403", got)
	}
}

func TestListReturnsTheCallersPage(t *testing.T) {
	e := newEnv(t)
	base := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	first := e.seed(ws, alice, "chat.mention", "Ada in #general", "chat.message:m1", base)
	second := e.seed(ws, alice, "chat.dm", "Ada", "chat.message:m2", base.Add(time.Minute))
	e.seed(ws, bob, "chat.mention", "Bob's", "chat.message:m3", base.Add(2*time.Minute))

	rec := e.api.Get("/notifications", headers("alice")...)
	if rec.Code != 200 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	var body listBody
	mustDecode(t, rec.Body.Bytes(), &body)
	if len(body.Items) != 2 || body.HasMore {
		t.Fatalf("page = %d items has_more=%v, want 2/false", len(body.Items), body.HasMore)
	}
	if body.Items[0].ID != second || body.Items[1].ID != first {
		t.Fatalf("order = %s, %s, want newest first", body.Items[0].ID, body.Items[1].ID)
	}
	got := body.Items[0]
	if got.Kind != "chat.dm" || got.Title != "Ada" || got.Body != "hi" || got.Link != "/chat/c1" {
		t.Fatalf("item = %+v", got)
	}
	if got.Data["channel_id"] != "c1" || got.ReadAt != nil || got.CreatedAt.IsZero() {
		t.Fatalf("item fields = %+v", got)
	}
}

func TestListCursorAndUnreadFilter(t *testing.T) {
	e := newEnv(t)
	base := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	var ids []string
	for i := range 3 {
		ids = append(ids, e.seed(ws, alice, "chat.mention", "n", fmt.Sprintf("chat.message:m%d", i), base.Add(time.Duration(i)*time.Minute)))
	}

	rec := e.api.Get("/notifications?limit=2", headers("alice")...)
	var page listBody
	mustDecode(t, rec.Body.Bytes(), &page)
	if rec.Code != 200 || len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("first page = %d %+v", rec.Code, page)
	}

	rec = e.api.Get("/notifications?limit=2&before="+page.Items[1].ID, headers("alice")...)
	var next listBody
	mustDecode(t, rec.Body.Bytes(), &next)
	if rec.Code != 200 || len(next.Items) != 1 || next.HasMore || next.Items[0].ID != ids[0] {
		t.Fatalf("next page = %d %+v", rec.Code, next)
	}

	if _, err := e.repo.MarkRead(context.Background(), ws, alice, ids[2], time.Now()); err != nil {
		t.Fatal(err)
	}
	rec = e.api.Get("/notifications?unread=true", headers("alice")...)
	var unread listBody
	mustDecode(t, rec.Body.Bytes(), &unread)
	if rec.Code != 200 || len(unread.Items) != 2 {
		t.Fatalf("unread page = %d %d items, want 2", rec.Code, len(unread.Items))
	}
}

func TestListValidation(t *testing.T) {
	e := newEnv(t)
	if got := e.api.Get("/notifications?limit=51", headers("alice")...).Code; got != 400 {
		t.Fatalf("limit=51 = %d, want 400", got)
	}
	if got := e.api.Get("/notifications?before=not-an-id", headers("alice")...).Code; got != 400 {
		t.Fatalf("bad cursor = %d, want 400", got)
	}
}

func TestUnreadCount(t *testing.T) {
	e := newEnv(t)
	e.seed(ws, alice, "chat.mention", "n", "chat.message:m1", time.Now())
	e.seed(ws, alice, "chat.mention", "n", "chat.message:m2", time.Now())
	e.seed(ws, bob, "chat.mention", "n", "chat.message:m3", time.Now())

	rec := e.api.Get("/unread-count", headers("alice")...)
	if rec.Code != 200 {
		t.Fatalf("count = %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Count int64 `json:"count"`
	}
	mustDecode(t, rec.Body.Bytes(), &body)
	if body.Count != 2 {
		t.Fatalf("count = %d, want 2", body.Count)
	}
}

func TestMarkRead(t *testing.T) {
	e := newEnv(t)
	id := e.seed(ws, alice, "chat.mention", "n", "chat.message:m1", time.Now())
	foreign := e.seed(ws, bob, "chat.mention", "n", "chat.message:m2", time.Now())

	if rec := e.api.Post("/notifications/"+id+"/read", headers("alice")...); rec.Code != 204 {
		t.Fatalf("mark read = %d %s", rec.Code, rec.Body)
	}
	if rec := e.api.Post("/notifications/"+id+"/read", headers("alice")...); rec.Code != 204 {
		t.Fatalf("repeat mark read = %d %s", rec.Code, rec.Body)
	}
	if rec := e.api.Post("/notifications/"+foreign+"/read", headers("alice")...); rec.Code != 404 {
		t.Fatalf("another user's id = %d, want 404", rec.Code)
	}
	if rec := e.api.Post("/notifications/nope/read", headers("alice")...); rec.Code != 404 {
		t.Fatalf("unknown id = %d, want 404", rec.Code)
	}

	reads := e.ev.readCalls()
	if len(reads) != 1 {
		t.Fatalf("read events = %d, want 1", len(reads))
	}
	if reads[0].UserID != alice || reads[0].All || len(reads[0].IDs) != 1 || reads[0].IDs[0] != id || reads[0].Unread != 0 {
		t.Fatalf("read event = %+v", reads[0])
	}
}

func TestReadAll(t *testing.T) {
	e := newEnv(t)
	e.seed(ws, alice, "chat.mention", "n", "chat.message:m1", time.Now())
	e.seed(ws, alice, "chat.dm", "n", "chat.message:m2", time.Now())
	e.seed(ws, bob, "chat.mention", "n", "chat.message:m3", time.Now())

	if rec := e.api.Post("/read-all", headers("alice")...); rec.Code != 204 {
		t.Fatalf("read all = %d %s", rec.Code, rec.Body)
	}
	reads := e.ev.readCalls()
	if len(reads) != 1 || !reads[0].All || len(reads[0].IDs) != 2 || reads[0].Unread != 0 {
		t.Fatalf("read events = %+v", reads)
	}

	rec := e.api.Get("/unread-count", headers("alice")...)
	var body struct {
		Count int64 `json:"count"`
	}
	mustDecode(t, rec.Body.Bytes(), &body)
	if body.Count != 0 {
		t.Fatalf("count after read-all = %d, want 0", body.Count)
	}
}

func mustDecode(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
}
