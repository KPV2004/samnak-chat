package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

const (
	ws      = "0193f0f0-0000-7000-8000-0000000000a1"
	ws2     = "0193f0f0-0000-7000-8000-0000000000a2"
	alice   = "0193f0f0-0000-7000-8000-0000000000b1"
	bob     = "0193f0f0-0000-7000-8000-0000000000b2"
	carol   = "0193f0f0-0000-7000-8000-0000000000b3"
	admin   = "0193f0f0-0000-7000-8000-0000000000b4"
	outside = "0193f0f0-0000-7000-8000-0000000000b9"
)

var t0 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

// fakeTx runs the function at once: there is no database to roll back.
type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// fakePerms allows the helpdesk.admin action for the listed users.
type fakePerms map[string]bool

func (f fakePerms) Can(p app.Principal, action string) bool {
	return action == usecase.ActionAdmin && f[p.UserID]
}

// fakeMembers knows the active members of every workspace.
type fakeMembers map[string]bool

func (f fakeMembers) Active(_ context.Context, workspaceID, userID string) bool {
	return f[workspaceID+"/"+userID]
}

// memRepo is an in-memory usecase.Repository. Its SQL has its own tests.
type memRepo struct {
	mu         sync.Mutex
	queues     map[string]domain.Queue
	categories map[string]domain.Category
	tickets    map[string]domain.Ticket
	history    []domain.HistoryEntry
	// next is the number the next ticket of each queue takes.
	next map[string]int64
}

func newMemRepo() *memRepo {
	return &memRepo{
		queues: map[string]domain.Queue{}, categories: map[string]domain.Category{},
		tickets: map[string]domain.Ticket{}, next: map[string]int64{},
	}
}

func (r *memRepo) NextTicketNumber(_ context.Context, workspaceID, queueID string) (string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[queueID]
	if !ok || q.WorkspaceID != workspaceID || q.ArchivedAt != nil {
		return "", 0, notFound("queue")
	}
	r.next[queueID]++
	return q.Prefix, r.next[queueID], nil
}

func (r *memRepo) InsertTicket(_ context.Context, t domain.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, have := range r.tickets {
		if have.QueueID == t.QueueID && have.Number == t.Number {
			return fmt.Errorf("ticket number is taken: %w", app.ErrConflict)
		}
	}
	r.tickets[t.ID] = t
	return nil
}

func (r *memRepo) InsertHistory(_ context.Context, h domain.HistoryEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, h)
	return nil
}

// keyOf is the ticket's key from its queue's prefix.
func (r *memRepo) keyOf(t domain.Ticket) string {
	return domain.TicketKey(r.queues[t.QueueID].Prefix, t.Number)
}

func (r *memRepo) Ticket(_ context.Context, workspaceID, id string) (domain.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[id]
	if !ok || t.WorkspaceID != workspaceID {
		return domain.Ticket{}, notFound("ticket")
	}
	t.Key = r.keyOf(t)
	return t, nil
}

func (r *memRepo) ListTickets(_ context.Context, f usecase.TicketFilter) ([]domain.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Ticket
	for _, t := range r.tickets {
		if t.WorkspaceID != f.WorkspaceID || !visibleTo(r, t, f.UserID, f.Admin) {
			continue
		}
		if f.QueueID != "" && t.QueueID != f.QueueID {
			continue
		}
		if len(f.Statuses) > 0 && !slicesContains(f.Statuses, t.Status) {
			continue
		}
		if f.AssigneeID != "" && t.AssigneeID != f.AssigneeID {
			continue
		}
		if f.NoAssignee && t.AssigneeID != "" {
			continue
		}
		if f.RequesterID != "" && t.RequesterID != f.RequesterID {
			continue
		}
		if f.BeforeID != "" && t.ID >= f.BeforeID {
			continue
		}
		t.Key = r.keyOf(t)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// visibleTo is the requester, the queue's agents and the admins.
func visibleTo(r *memRepo, t domain.Ticket, userID string, admin bool) bool {
	if admin || t.RequesterID == userID {
		return true
	}
	q := r.queues[t.QueueID]
	return q.IsAgent(userID)
}

func slicesContains(list []domain.Status, s domain.Status) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (r *memRepo) IsAgent(_ context.Context, workspaceID, queueID, userID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[queueID]
	return ok && q.WorkspaceID == workspaceID && q.IsAgent(userID), nil
}

func (r *memRepo) History(_ context.Context, ticketID string) ([]domain.HistoryEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.HistoryEntry
	for _, h := range r.history {
		if h.TicketID == ticketID {
			out = append(out, h)
		}
	}
	return out, nil
}

// ticketsOf returns the tickets of a queue, by number.
func (r *memRepo) ticketsOf(queueID string) []domain.Ticket {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Ticket
	for _, t := range r.tickets {
		if t.QueueID == queueID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func notFound(what string) error { return fmt.Errorf("%s: %w", what, app.ErrNotFound) }

func (r *memRepo) InsertQueue(_ context.Context, q domain.Queue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, have := range r.queues {
		if have.WorkspaceID == q.WorkspaceID && have.Prefix == q.Prefix {
			return fmt.Errorf("prefix is already used: %w", app.ErrConflict)
		}
	}
	r.queues[q.ID] = q
	return nil
}

func (r *memRepo) UpdateQueue(_ context.Context, q domain.Queue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	have, ok := r.queues[q.ID]
	if !ok || have.WorkspaceID != q.WorkspaceID {
		return notFound("queue")
	}
	q.AgentIDs = have.AgentIDs
	r.queues[q.ID] = q
	return nil
}

func (r *memRepo) Queue(_ context.Context, workspaceID, id string) (domain.Queue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q, ok := r.queues[id]; ok && q.WorkspaceID == workspaceID {
		return q, nil
	}
	return domain.Queue{}, notFound("queue")
}

func (r *memRepo) Queues(_ context.Context, workspaceID string, includeArchived bool) ([]domain.Queue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Queue
	for _, q := range r.queues {
		if q.WorkspaceID == workspaceID && (includeArchived || q.ArchivedAt == nil) {
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *memRepo) SetAgents(_ context.Context, workspaceID, queueID string, userIDs []string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[queueID]
	if !ok || q.WorkspaceID != workspaceID {
		return notFound("queue")
	}
	q.AgentIDs = append([]string(nil), userIDs...)
	r.queues[queueID] = q
	return nil
}

func (r *memRepo) InsertCategory(_ context.Context, c domain.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.categories[c.ID] = c
	return nil
}

func (r *memRepo) UpdateCategory(_ context.Context, c domain.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if have, ok := r.categories[c.ID]; !ok || have.WorkspaceID != c.WorkspaceID {
		return notFound("category")
	}
	r.categories[c.ID] = c
	return nil
}

func (r *memRepo) Category(_ context.Context, workspaceID, id string) (domain.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.categories[id]; ok && c.WorkspaceID == workspaceID {
		return c, nil
	}
	return domain.Category{}, notFound("category")
}

func (r *memRepo) Categories(_ context.Context, workspaceID string, includeArchived bool) ([]domain.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Category
	for _, c := range r.categories {
		if c.WorkspaceID == workspaceID && (includeArchived || c.ArchivedAt == nil) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *memRepo) ClearDefault(_ context.Context, workspaceID, exceptID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.categories {
		if c.WorkspaceID == workspaceID && id != exceptID && c.IsDefault {
			c.IsDefault = false
			r.categories[id] = c
		}
	}
	return nil
}

// defaults returns the IDs of the default categories of a workspace.
func (r *memRepo) defaults(workspaceID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id, c := range r.categories {
		if c.WorkspaceID == workspaceID && c.IsDefault && c.ArchivedAt == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// recordingBus keeps every published event, in order.
type recordingBus struct {
	mu     sync.Mutex
	events []app.Event
	err    error
}

func (b *recordingBus) Publish(_ context.Context, evt app.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	b.events = append(b.events, evt)
	return nil
}

func (b *recordingBus) all() []app.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]app.Event(nil), b.events...)
}

// fakeMessages answers ReadMessage from a table; a missing message or one the
// reader may not see is not found.
type fakeMessages map[string]app.MessageSnapshot

func (f fakeMessages) ReadMessage(_ context.Context, _, messageID, _ string) (app.MessageSnapshot, error) {
	if m, ok := f[messageID]; ok {
		return m, nil
	}
	return app.MessageSnapshot{}, notFound("message")
}

// env wires a Service over the fakes. Admins are alice and bob; carol is an
// active member who is not an admin; outside is not a member at all.
type env struct {
	svc  *usecase.Service
	repo *memRepo
	bus  *recordingBus
	msgs fakeMessages
	p    map[string]app.Principal
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo := newMemRepo()
	members := fakeMembers{ws + "/" + alice: true, ws + "/" + bob: true, ws + "/" + carol: true, ws + "/" + admin: true}
	e := &env{repo: repo, bus: &recordingBus{}, msgs: fakeMessages{}, p: map[string]app.Principal{}}
	for name, id := range map[string]string{"alice": alice, "bob": bob, "carol": carol, "admin": admin, "outside": outside} {
		role := app.RoleMember
		if name == "alice" || name == "admin" {
			role = app.RoleAdmin
		}
		e.p[name] = app.Principal{UserID: id, WorkspaceID: ws, Role: role}
	}
	e.svc = usecase.New(usecase.Deps{
		Tx: fakeTx{}, Repo: repo, Members: members, Bus: e.bus, Messages: e.msgs,
		Permissions: fakePerms{alice: true, admin: true},
		Now:         func() time.Time { return t0 },
	})
	return e
}
