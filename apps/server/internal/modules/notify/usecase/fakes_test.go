package usecase

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

const (
	ws  = "0193f0f0-0000-7000-8000-0000000000a1"
	u1  = "0193f0f0-0000-7000-8000-0000000000b1"
	u2  = "0193f0f0-0000-7000-8000-0000000000b2"
	u3  = "0193f0f0-0000-7000-8000-0000000000b3"
	now = "2026-11-16T09:30:00Z"
)

func fixedNow() func() time.Time {
	at, err := time.Parse(time.RFC3339, now)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return at }
}

func quietLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

type testEvent struct {
	Name   string
	Detail string
}

func (e testEvent) EventName() string { return e.Name }

func principal(workspaceID, userID string) app.Principal {
	return app.Principal{UserID: userID, WorkspaceID: workspaceID, Role: app.RoleMember}
}

// appNotification is a stored notification, for seeding the fake repository.
func appNotification(workspaceID, userID, key string) domain.Notification {
	return domain.New(app.Notification{WorkspaceID: workspaceID, UserID: userID, Kind: "chat.mention", Key: key}, newID(), fixedNow()())
}

// fakeRepo keeps notifications in memory and copies the contract of the
// Postgres repository: keys deduplicate per user, a missing row is
// app.ErrNotFound, and pages are newest first.
type fakeRepo struct {
	mu    sync.Mutex
	rows  []domain.Notification
	order int

	insertErr error
	unreadErr error
	listErr   error
	markErr   error

	lastLimit  int
	lastBefore string
	pruned     time.Time
	pruneRows  int64
}

func (f *fakeRepo) Insert(_ context.Context, n domain.Notification) (domain.Notification, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return domain.Notification{}, false, f.insertErr
	}
	if n.Key != "" {
		for _, row := range f.rows {
			if row.UserID == n.UserID && row.Key == n.Key {
				return domain.Notification{}, false, nil
			}
		}
	}
	f.order++
	f.rows = append(f.rows, n)
	return n, true, nil
}

func (f *fakeRepo) List(_ context.Context, workspaceID, userID string, unreadOnly bool, beforeID string, limit int) ([]domain.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastLimit, f.lastBefore = limit, beforeID
	if f.listErr != nil {
		return nil, f.listErr
	}
	matches := make([]domain.Notification, 0, len(f.rows))
	for _, row := range f.rows {
		if row.WorkspaceID != workspaceID || row.UserID != userID {
			continue
		}
		if unreadOnly && row.ReadAt != nil {
			continue
		}
		matches = append(matches, row)
	}
	// The real query orders by (created_at, id) descending.
	sort.SliceStable(matches, func(i, j int) bool {
		if !matches[i].CreatedAt.Equal(matches[j].CreatedAt) {
			return matches[i].CreatedAt.After(matches[j].CreatedAt)
		}
		return matches[i].ID > matches[j].ID
	})
	if beforeID != "" {
		cut := -1
		for i, row := range matches {
			if row.ID == beforeID {
				cut = i
				break
			}
		}
		if cut < 0 {
			return nil, app.ErrNotFound
		}
		matches = matches[cut+1:]
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func (f *fakeRepo) UnreadCount(_ context.Context, workspaceID, userID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unreadErr != nil {
		return 0, f.unreadErr
	}
	var n int64
	for _, row := range f.rows {
		if row.WorkspaceID == workspaceID && row.UserID == userID && row.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) MarkRead(_ context.Context, workspaceID, userID, id string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markErr != nil {
		return false, f.markErr
	}
	for i, row := range f.rows {
		if row.ID == id && row.WorkspaceID == workspaceID && row.UserID == userID {
			if row.ReadAt != nil {
				return false, nil
			}
			t := at.UTC()
			f.rows[i].ReadAt = &t
			return true, nil
		}
	}
	return false, app.ErrNotFound
}

func (f *fakeRepo) MarkAllRead(_ context.Context, workspaceID, userID string, at time.Time) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := at.UTC()
	var ids []string
	for i, row := range f.rows {
		if row.WorkspaceID == workspaceID && row.UserID == userID && row.ReadAt == nil {
			f.rows[i].ReadAt = &t
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func (f *fakeRepo) Prune(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = before
	kept := f.rows[:0]
	var deleted int64
	for _, row := range f.rows {
		if row.ReadAt != nil && row.CreatedAt.Before(before) {
			deleted++
			continue
		}
		kept = append(kept, row)
	}
	f.rows = kept
	f.pruneRows = deleted
	return deleted, nil
}

func (f *fakeRepo) all() []domain.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Notification(nil), f.rows...)
}

// fakeTx records transactions and their commits, so a test can prove that
// delivery happened after the commit.
type fakeTx struct {
	mu      sync.Mutex
	calls   int
	depth   int
	commits int
	err     error
}

func (f *fakeTx) Do(ctx context.Context, fn func(context.Context) error) error {
	f.mu.Lock()
	f.calls++
	if f.err != nil {
		err := f.err
		f.mu.Unlock()
		return err
	}
	f.depth++
	f.mu.Unlock()

	err := fn(ctx)

	f.mu.Lock()
	f.depth--
	if err == nil {
		f.commits++
	}
	f.mu.Unlock()
	return err
}

func (f *fakeTx) commitsNow() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commits
}

type createdCall struct {
	N             domain.Notification
	Unread        int64
	CommitsAtSend int
}

type readCall struct {
	WorkspaceID string
	UserID      string
	IDs         []string
	All         bool
	Unread      int64
}

type fakeEvents struct {
	mu      sync.Mutex
	tx      *fakeTx
	created []createdCall
	read    []readCall
}

func (f *fakeEvents) Created(_ context.Context, n domain.Notification, unread int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	commits := 0
	if f.tx != nil {
		commits = f.tx.commitsNow()
	}
	f.created = append(f.created, createdCall{N: n, Unread: unread, CommitsAtSend: commits})
}

func (f *fakeEvents) Read(_ context.Context, workspaceID, userID string, ids []string, all bool, unread int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, readCall{WorkspaceID: workspaceID, UserID: userID, IDs: ids, All: all, Unread: unread})
}

func (f *fakeEvents) createdCalls() []createdCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]createdCall(nil), f.created...)
}

func (f *fakeEvents) readCalls() []readCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]readCall(nil), f.read...)
}

func newService(t *testing.T) (*Service, *fakeRepo, *fakeTx, *fakeEvents, *app.NotificationRules) {
	t.Helper()
	repo := &fakeRepo{}
	tx := &fakeTx{}
	ev := &fakeEvents{tx: tx}
	rules := app.NewNotificationRules()
	return New(tx, repo, rules, ev, quietLog(), fixedNow()), repo, tx, ev, rules
}
