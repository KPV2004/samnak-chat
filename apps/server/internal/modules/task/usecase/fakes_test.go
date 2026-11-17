package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
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
	dave  = "0193f0f0-0000-7000-8000-0000000000b5"
)

var clock = time.Date(2026, 3, 4, 10, 30, 0, 0, time.UTC)

func principal(user string, role app.Role) app.Principal {
	return app.Principal{UserID: user, WorkspaceID: ws, Role: role}
}

var (
	asAlice = principal(alice, app.RoleMember)
	asBob   = principal(bob, app.RoleMember)
	asCarol = principal(carol, app.RoleMember)
	asAdmin = principal(admin, app.RoleAdmin)
)

// fakeRepo keeps tasks in memory with the visible rules of the Postgres
// repository: workspace scoping, newest-first pages by ID and ErrNotFound.
type fakeRepo struct {
	mu    sync.Mutex
	tasks map[string]domain.Task
	// insertErr and updateErr make the next write fail.
	insertErr, updateErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{tasks: map[string]domain.Task{}} }

func (r *fakeRepo) put(t domain.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
}

func (r *fakeRepo) Insert(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertErr != nil {
		return domain.Task{}, r.insertErr
	}
	r.tasks[t.ID] = t
	return t, nil
}

func (r *fakeRepo) Get(_ context.Context, workspaceID, id string) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || t.WorkspaceID != workspaceID {
		return domain.Task{}, fmt.Errorf("task %q: %w", id, app.ErrNotFound)
	}
	return t, nil
}

func (r *fakeRepo) GetForUpdate(ctx context.Context, workspaceID, id string) (domain.Task, error) {
	return r.Get(ctx, workspaceID, id)
}

func (r *fakeRepo) Update(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateErr != nil {
		return domain.Task{}, r.updateErr
	}
	old, ok := r.tasks[t.ID]
	if !ok || old.WorkspaceID != t.WorkspaceID {
		return domain.Task{}, app.ErrNotFound
	}
	// Only the editable columns are written.
	old.Title, old.Description, old.AssigneeID, old.DueAt = t.Title, t.Description, t.AssigneeID, t.DueAt
	old.Priority, old.Status, old.DueSoonSentAt, old.OverdueSentAt, old.UpdatedAt = t.Priority, t.Status, t.DueSoonSentAt, t.OverdueSentAt, t.UpdatedAt
	r.tasks[t.ID] = old
	return old, nil
}

func (r *fakeRepo) SetMessage(_ context.Context, workspaceID, id, messageID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || t.WorkspaceID != workspaceID {
		return false, app.ErrNotFound
	}
	if t.MessageID != "" {
		return false, nil
	}
	t.MessageID = messageID
	r.tasks[id] = t
	return true, nil
}

func (r *fakeRepo) List(_ context.Context, workspaceID string, f usecase.Filter, afterID string, limit int) ([]domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Task
	for _, t := range r.tasks {
		switch {
		case t.WorkspaceID != workspaceID,
			f.AssigneeID != "" && t.AssigneeID != f.AssigneeID,
			f.CreatorID != "" && t.CreatorID != f.CreatorID,
			f.ChannelID != "" && t.ChannelID != f.ChannelID,
			len(f.Statuses) > 0 && !slices.Contains(f.Statuses, t.Status),
			f.OpenOnly && t.Status == domain.StatusDone,
			f.DueFrom != nil && (t.DueAt == nil || t.DueAt.Before(*f.DueFrom)),
			f.DueBefore != nil && (t.DueAt == nil || !t.DueAt.Before(*f.DueBefore)),
			afterID != "" && t.ID >= afterID:
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

func (r *fakeRepo) ClaimDueSoon(_ context.Context, now, horizon time.Time, batch int) ([]usecase.Due, error) {
	return r.claim(batch, func(t *domain.Task) bool {
		if t.DueSoonSentAt != nil || !t.DueAt.After(now) || t.DueAt.After(horizon) {
			return false
		}
		n := now
		t.DueSoonSentAt = &n
		return true
	})
}

func (r *fakeRepo) ClaimOverdue(_ context.Context, now time.Time, batch int) ([]usecase.Due, error) {
	return r.claim(batch, func(t *domain.Task) bool {
		if t.OverdueSentAt != nil || t.DueAt.After(now) {
			return false
		}
		n := now
		t.OverdueSentAt = &n
		return true
	})
}

func (r *fakeRepo) claim(batch int, mark func(*domain.Task) bool) ([]usecase.Due, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out []usecase.Due
	for _, id := range ids {
		t := r.tasks[id]
		if t.Status == domain.StatusDone || t.AssigneeID == "" || t.DueAt == nil || len(out) >= batch {
			continue
		}
		if mark(&t) {
			r.tasks[id] = t
			out = append(out, usecase.Due{TaskID: t.ID, WorkspaceID: t.WorkspaceID, AssigneeID: t.AssigneeID, DueAt: *t.DueAt})
		}
	}
	return out, nil
}

// fakeTx runs fn and tells the fakes whether they are inside it.
type fakeTx struct {
	in *bool
	// failAfter makes Do return this error after fn succeeded, like a failed commit.
	failAfter error
}

func (f fakeTx) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	*f.in = true
	defer func() { *f.in = false }()
	if err := fn(ctx); err != nil {
		return err
	}
	return f.failAfter
}

// fakeChannels: readers lists "channel/user" pairs that can read.
type fakeChannels struct {
	mu      sync.Mutex
	readers map[string]bool
	calls   int
	err     error
}

func (c *fakeChannels) allow(channel string, users ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, u := range users {
		c.readers[channel+"/"+u] = true
	}
}

func (c *fakeChannels) CanRead(_ context.Context, workspaceID, channelID, userID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.err != nil {
		return false, c.err
	}
	return workspaceID == ws && c.readers[channelID+"/"+userID], nil
}

type fakeMembers map[string]bool

func (m fakeMembers) IsActiveMember(_ context.Context, workspaceID, userID string) (bool, error) {
	return workspaceID == ws && m[userID], nil
}

type fakeDirectory map[string]string

func (d fakeDirectory) Names(_ context.Context, ids []string) map[string]string {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := d[id]; ok {
			out[id] = n
		}
	}
	return out
}

type posted struct {
	Card app.Card
	ID   string
}

type updated struct {
	WorkspaceID, MessageID string
	Payload                any
}

type fakeCards struct {
	mu      sync.Mutex
	posts   []posted
	updates []updated
	// postErr and updateErr make the call fail.
	postErr, updateErr error
	// beforePost runs inside Post, to simulate another delivery.
	beforePost func()
}

func (c *fakeCards) Post(_ context.Context, card app.Card) (string, error) {
	if c.beforePost != nil {
		c.beforePost()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.postErr != nil {
		return "", c.postErr
	}
	id := fmt.Sprintf("0193f0f0-0000-7000-8000-%012d", len(c.posts)+1)
	c.posts = append(c.posts, posted{Card: card, ID: id})
	return id, nil
}

func (c *fakeCards) Update(_ context.Context, workspaceID, messageID string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updateErr != nil {
		return c.updateErr
	}
	c.updates = append(c.updates, updated{workspaceID, messageID, payload})
	return nil
}

// fakeEvents records events as "name" strings and whether each was published
// inside the transaction.
type fakeEvents struct {
	inTx   *bool
	mu     sync.Mutex
	names  []string
	outTx  []string // events published outside a transaction
	last   map[string]any
	failOn string
}

func (e *fakeEvents) record(name string, detail any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failOn == name {
		return errors.New("publish failed")
	}
	if !*e.inTx {
		e.outTx = append(e.outTx, name)
	}
	e.names = append(e.names, name)
	if e.last == nil {
		e.last = map[string]any{}
	}
	e.last[name] = detail
	return nil
}

type assignedEvent struct {
	Task  domain.Task
	Actor string
}

type statusEvent struct {
	Task  domain.Task
	From  domain.Status
	Actor string
}

func (e *fakeEvents) Created(_ context.Context, t domain.Task) error { return e.record("created", t) }
func (e *fakeEvents) Assigned(_ context.Context, t domain.Task, actor string) error {
	return e.record("assigned", assignedEvent{t, actor})
}
func (e *fakeEvents) StatusChanged(_ context.Context, t domain.Task, from domain.Status, actor string) error {
	return e.record("status_changed", statusEvent{t, from, actor})
}
func (e *fakeEvents) Updated(_ context.Context, t domain.Task) error { return e.record("updated", t) }
func (e *fakeEvents) DueSoon(_ context.Context, d usecase.Due) error { return e.record("due_soon", d) }
func (e *fakeEvents) Overdue(_ context.Context, d usecase.Due) error { return e.record("overdue", d) }

func (e *fakeEvents) got() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.names)
}

type env struct {
	t        *testing.T
	uc       *usecase.Service
	repo     *fakeRepo
	channels *fakeChannels
	members  usecase.Members
	dir      usecase.Directory
	cards    *fakeCards
	events   *fakeEvents
	tx       *fakeTx
	now      time.Time
}

// newEnv has alice, bob, carol and admin in the workspace. Alice and bob and
// admin can read ch; carol cannot, and nobody can read ch2 but carol.
func newEnv(t *testing.T) *env {
	t.Helper()
	inTx := new(bool)
	e := &env{
		t: t, repo: newFakeRepo(), channels: &fakeChannels{readers: map[string]bool{}},
		members: fakeMembers{alice: true, bob: true, carol: true, admin: true},
		dir:     fakeDirectory{alice: "Alice", bob: "Bob", carol: "Carol", admin: "Admin"},
		cards:   &fakeCards{}, events: &fakeEvents{inTx: inTx}, tx: &fakeTx{in: inTx}, now: clock,
	}
	e.channels.allow(ch, alice, bob, admin)
	e.channels.allow(ch2, carol)
	e.build()
	return e
}

func (e *env) build() {
	e.uc = usecase.New(usecase.Deps{
		Tx: e.tx, Repo: e.repo, Channels: e.channels, Members: e.members, Directory: e.dir,
		Cards: e.cards, Events: e.events, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return e.now },
	})
}

// buildWithoutChannels rebuilds the service as if chat did not provide ChannelAccess.
func (e *env) buildWithoutChannels() {
	e.uc = usecase.New(usecase.Deps{
		Tx: e.tx, Repo: e.repo, Members: e.members, Directory: e.dir, Cards: e.cards, Events: e.events,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return e.now },
	})
}

func (e *env) create(p app.Principal, in usecase.CreateInput) domain.Task {
	e.t.Helper()
	got, err := e.uc.Create(context.Background(), p, in)
	if err != nil {
		e.t.Fatal(err)
	}
	return got
}

func ptr[T any](v T) *T { return &v }
