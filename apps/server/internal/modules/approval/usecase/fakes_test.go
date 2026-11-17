package usecase_test

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

const (
	ws        = "0193f0f0-0000-7000-8000-0000000000a1"
	otherWS   = "0193f0f0-0000-7000-8000-0000000000a2"
	alice     = "0193f0f0-0000-7000-8000-0000000000b1" // the requester
	bob       = "0193f0f0-0000-7000-8000-0000000000b2" // the approver
	carol     = "0193f0f0-0000-7000-8000-0000000000b3" // a third member
	manager   = "0193f0f0-0000-7000-8000-0000000000b4" // alice's group manager
	manager2  = "0193f0f0-0000-7000-8000-0000000000b5"
	outsider  = "0193f0f0-0000-7000-8000-0000000000b6" // not a member
	leaveJSON = `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-03","reason":"trip"}`
)

var t0 = time.Date(2026, 11, 17, 9, 0, 0, 123456789, time.UTC)

func p(user string) app.Principal {
	return app.Principal{UserID: user, WorkspaceID: ws, Role: app.RoleMember}
}

// fakeRepo keeps everything in memory and copies the contract of the Postgres
// repository. snapshot and restore let memTx roll back.
type fakeRepo struct {
	mu          sync.Mutex
	requests    map[string]domain.Request
	steps       map[string]domain.Step
	history     []domain.HistoryEntry
	attachments map[string]domain.Attachment
	cardCalls   int
	insertErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{requests: map[string]domain.Request{}, steps: map[string]domain.Step{}, attachments: map[string]domain.Attachment{}}
}

type snapshot struct {
	requests    map[string]domain.Request
	steps       map[string]domain.Step
	history     []domain.HistoryEntry
	attachments map[string]domain.Attachment
}

func (f *fakeRepo) snapshot() snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return snapshot{maps.Clone(f.requests), maps.Clone(f.steps), slices.Clone(f.history), maps.Clone(f.attachments)}
}

func (f *fakeRepo) restore(s snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests, f.steps, f.history, f.attachments = s.requests, s.steps, s.history, s.attachments
}

func notFound(what string) error { return fmt.Errorf("%s: %w", what, app.ErrNotFound) }

func (f *fakeRepo) InsertRequest(_ context.Context, r domain.Request, step domain.Step, h domain.HistoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.requests[r.ID] = r
	f.steps[r.ID] = step
	f.history = append(f.history, h)
	return nil
}

func (f *fakeRepo) Request(_ context.Context, workspaceID, id string) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[id]
	if !ok || r.WorkspaceID != workspaceID {
		return domain.Request{}, notFound("request")
	}
	return r, nil
}

func (f *fakeRepo) List(_ context.Context, q usecase.ListFilter) ([]domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []domain.Request
	for _, r := range f.requests {
		if r.WorkspaceID != q.WorkspaceID {
			continue
		}
		mine := (q.Role != "approver" && r.RequesterID == q.UserID) || (q.Role != "requester" && r.ApproverID == q.UserID)
		if !mine || (len(q.Statuses) > 0 && !slices.Contains(q.Statuses, r.Status)) {
			continue
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		return all[i].ID > all[j].ID
	})
	if q.BeforeID != "" {
		i := slices.IndexFunc(all, func(r domain.Request) bool { return r.ID == q.BeforeID })
		if i < 0 {
			return nil, notFound("cursor")
		}
		all = all[i+1:]
	}
	if len(all) > q.Limit {
		all = all[:q.Limit]
	}
	return all, nil
}

func (f *fakeRepo) Decide(_ context.Context, workspaceID, id string, to domain.Status, reason string, at time.Time) (domain.Request, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[id]
	if !ok || r.WorkspaceID != workspaceID {
		return domain.Request{}, false, notFound("request")
	}
	if r.Status != domain.StatusPending {
		return r, false, nil
	}
	r.Status, r.Reason, r.DecidedAt, r.UpdatedAt = to, reason, &at, at
	f.requests[id] = r
	s := f.steps[id]
	s.Status, s.Reason, s.DecidedAt = to, reason, &at
	f.steps[id] = s
	return r, true, nil
}

func (f *fakeRepo) InsertHistory(_ context.Context, h domain.HistoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = append(f.history, h)
	return nil
}

func (f *fakeRepo) History(_ context.Context, requestID string) ([]domain.HistoryEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.HistoryEntry
	for _, h := range f.history {
		if h.RequestID == requestID {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func (f *fakeRepo) SetCard(_ context.Context, workspaceID, id, channelID, messageID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cardCalls++
	r, ok := f.requests[id]
	if !ok || r.WorkspaceID != workspaceID {
		return false, notFound("request")
	}
	if r.MessageID != "" {
		return false, nil
	}
	r.ChannelID, r.MessageID = channelID, messageID
	f.requests[id] = r
	return true, nil
}

func (f *fakeRepo) InsertAttachment(_ context.Context, a domain.Attachment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attachments[a.ID] = a
	return nil
}

func (f *fakeRepo) Attachment(_ context.Context, workspaceID, id string) (domain.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attachments[id]
	if !ok || a.WorkspaceID != workspaceID {
		return domain.Attachment{}, notFound("attachment")
	}
	return a, nil
}

func (f *fakeRepo) AttachmentsByIDs(_ context.Context, workspaceID string, ids []string) ([]domain.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Attachment
	for _, id := range ids {
		if a, ok := f.attachments[id]; ok && a.WorkspaceID == workspaceID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepo) LinkAttachments(_ context.Context, workspaceID, requestID, uploaderID string, ids []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, id := range ids {
		a, ok := f.attachments[id]
		if !ok || a.WorkspaceID != workspaceID || a.UploaderID != uploaderID || a.RequestID != "" {
			continue
		}
		a.RequestID = requestID
		f.attachments[id] = a
		n++
	}
	return n, nil
}

func (f *fakeRepo) AttachmentsOf(_ context.Context, requestID string) ([]domain.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Attachment
	for _, a := range f.attachments {
		if a.RequestID == requestID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// request returns the stored request, failing the test when it is missing.
func (f *fakeRepo) request(t *testing.T, id string) domain.Request {
	t.Helper()
	r, err := f.Request(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// memTx is a TxManager over fakeRepo: it rolls the repository back when fn
// fails, and hands events to the bus only after fn succeeded, like the outbox.
type memTx struct {
	repo *fakeRepo
	bus  *fakeBus
}

type inTxKey struct{}

func (m *memTx) Do(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(inTxKey{}) != nil {
		return fn(ctx)
	}
	snap := m.repo.snapshot()
	m.bus.begin()
	if err := fn(context.WithValue(ctx, inTxKey{}, true)); err != nil {
		m.repo.restore(snap)
		m.bus.rollback()
		return err
	}
	m.bus.commit()
	return nil
}

// fakeBus records events. Inside a transaction they wait for the commit.
type fakeBus struct {
	mu        sync.Mutex
	pending   []app.Event
	events    []app.Event
	publishFn func(app.Event) error
}

func (b *fakeBus) begin() { b.mu.Lock(); b.pending = nil; b.mu.Unlock() }
func (b *fakeBus) rollback() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = nil
}
func (b *fakeBus) commit() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, b.pending...)
	b.pending = nil
}

func (b *fakeBus) Publish(_ context.Context, evt app.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.publishFn != nil {
		if err := b.publishFn(evt); err != nil {
			return err
		}
	}
	b.pending = append(b.pending, evt)
	return nil
}

func (b *fakeBus) names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.events))
	for i, e := range b.events {
		out[i] = e.EventName()
	}
	return out
}

func (b *fakeBus) last() app.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil
	}
	return b.events[len(b.events)-1]
}

type fakeMembers map[string]bool

func (m fakeMembers) Active(_ context.Context, workspaceID, userID string) bool {
	return workspaceID == ws && m[userID]
}

type fakeManagers struct {
	byUser map[string][]string
	err    error
	calls  int
}

func (m *fakeManagers) ManagersOf(_ context.Context, _, userID string) ([]string, error) {
	m.calls++
	return m.byUser[userID], m.err
}

type fakeNames map[string]string

func (n fakeNames) Names(_ context.Context, ids []string) map[string]string {
	out := map[string]string{}
	for _, id := range ids {
		if v, ok := n[id]; ok {
			out[id] = v
		}
	}
	return out
}

// posted is one Cards.Post call.
type posted struct {
	WorkspaceID, RequesterID, ApproverID, Fallback string
	Payload                                        usecase.CardPayload
}

type fakeCards struct {
	mu        sync.Mutex
	posts     []posted
	updates   []usecase.CardPayload
	updatedID []string
	postErr   error
	updateErr error
}

func (c *fakeCards) Post(_ context.Context, workspaceID, requesterID, approverID string, payload any, fallback string) (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.postErr != nil {
		return "", "", c.postErr
	}
	c.posts = append(c.posts, posted{workspaceID, requesterID, approverID, fallback, payload.(usecase.CardPayload)})
	return "0193f0f0-0000-7000-8000-0000000000c1", fmt.Sprintf("0193f0f0-0000-7000-8000-00000000d%03d", len(c.posts)), nil
}

func (c *fakeCards) Update(_ context.Context, _, messageID string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updateErr != nil {
		return c.updateErr
	}
	c.updates = append(c.updates, payload.(usecase.CardPayload))
	c.updatedID = append(c.updatedID, messageID)
	return nil
}

type putCall struct {
	Key, ContentType string
	Size             int64
	TTL              time.Duration
}

type fakeStorage struct {
	puts    []putCall
	gets    []string
	getName []string
	stored  map[string]int64 // key -> size of an uploaded object
	putErr  error
}

func (s *fakeStorage) PresignPut(_ context.Context, key, contentType string, size int64, ttl time.Duration) (app.PresignedRequest, error) {
	if s.putErr != nil {
		return app.PresignedRequest{}, s.putErr
	}
	s.puts = append(s.puts, putCall{key, contentType, size, ttl})
	return app.PresignedRequest{URL: "https://store.test/put/" + key, Method: "PUT", Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: t0.Add(ttl)}, nil
}

func (s *fakeStorage) PresignGet(_ context.Context, key, fileName string, ttl time.Duration) (app.PresignedRequest, error) {
	s.gets = append(s.gets, key)
	s.getName = append(s.getName, fileName)
	return app.PresignedRequest{URL: "https://store.test/get/" + key, Method: "GET", ExpiresAt: t0.Add(ttl)}, nil
}

func (s *fakeStorage) Stat(_ context.Context, key string) (int64, error) {
	if size, ok := s.stored[key]; ok {
		return size, nil
	}
	return 0, fmt.Errorf("object: %w", app.ErrNotFound)
}

// rig wires a Service over the fakes.
type rig struct {
	svc      *usecase.Service
	repo     *fakeRepo
	bus      *fakeBus
	managers *fakeManagers
	cards    *fakeCards
	store    *fakeStorage
	now      time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		repo: newFakeRepo(), bus: &fakeBus{}, cards: &fakeCards{}, store: &fakeStorage{stored: map[string]int64{}},
		managers: &fakeManagers{byUser: map[string][]string{alice: {manager, manager2}}}, now: t0,
	}
	r.svc = usecase.New(usecase.Deps{
		Tx: &memTx{repo: r.repo, bus: r.bus}, Repo: r.repo, Bus: r.bus,
		Members:  fakeMembers{alice: true, bob: true, carol: true, manager: true, manager2: true},
		Managers: r.managers,
		Names:    fakeNames{alice: "Alice", bob: "Bob", carol: "Carol", manager: "Manager"},
		Cards:    r.cards, Storage: r.store,
		Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return r.now },
	})
	return r
}

// advance moves the fake clock.
func (r *rig) advance(d time.Duration) { r.now = r.now.Add(d) }

// submit sends alice's leave request to bob and returns it.
func (r *rig) submit(t *testing.T) usecase.Detail {
	t.Helper()
	d, err := r.svc.Submit(context.Background(), p(alice), usecase.SubmitInput{Type: "leave", Payload: []byte(leaveJSON), ApproverID: bob})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func newServiceWithoutManagers(r *rig) *usecase.Service {
	return usecase.New(usecase.Deps{
		Tx: &memTx{repo: r.repo, bus: r.bus}, Repo: r.repo, Bus: r.bus,
		Members: fakeMembers{alice: true, bob: true, carol: true, manager: true}, Cards: r.cards, Storage: r.store,
		Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return r.now },
	})
}

func newServiceWithoutStorage(r *rig) *usecase.Service {
	return usecase.New(usecase.Deps{
		Tx: &memTx{repo: r.repo, bus: r.bus}, Repo: r.repo, Bus: r.bus,
		Members: fakeMembers{alice: true, bob: true, carol: true}, Managers: r.managers, Cards: r.cards,
		Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return r.now },
	})
}

func newServiceWithRepo(r *rig, repo usecase.Repository) *usecase.Service {
	return usecase.New(usecase.Deps{
		Tx: &memTx{repo: r.repo, bus: r.bus}, Repo: repo, Bus: r.bus,
		Members:  fakeMembers{alice: true, bob: true, carol: true, manager: true, manager2: true},
		Managers: r.managers, Cards: r.cards, Storage: r.store,
		Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return r.now },
	})
}

// racingRepo loses every Decide to "someone else" while Request still shows
// pending: the repository's conditional update found the row not pending.
type racingRepo struct{ *fakeRepo }

func (racingRepo) Decide(context.Context, string, string, domain.Status, string, time.Time) (domain.Request, bool, error) {
	return domain.Request{Status: domain.StatusApproved}, false, nil
}
