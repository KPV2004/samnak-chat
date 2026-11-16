package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

func TestListDefaultsAndClampsTheLimit(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	p := principal(ws, u1)
	ctx := context.Background()

	if _, err := svc.List(ctx, p, ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != DefaultLimit+1 {
		t.Fatalf("limit asked from the repo = %d, want %d", repo.lastLimit, DefaultLimit+1)
	}

	if _, err := svc.List(ctx, p, ListQuery{Limit: 500}); err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != MaxLimit+1 {
		t.Fatalf("limit asked from the repo = %d, want %d", repo.lastLimit, MaxLimit+1)
	}

	if _, err := svc.List(ctx, p, ListQuery{Limit: 3}); err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != 4 {
		t.Fatalf("limit asked from the repo = %d, want 4", repo.lastLimit)
	}
}

func TestListPagesNewestFirstAndReportsMore(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()
	p := principal(ws, u1)
	var ids []string
	for range 3 {
		n := appNotification(ws, u1, "")
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	page, err := svc.List(ctx, p, ListQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("page = %d items, has_more=%v, want 2/true", len(page.Items), page.HasMore)
	}
	if page.Items[0].ID != ids[2] || page.Items[1].ID != ids[1] {
		t.Fatalf("page order = %+v, want newest first", page.Items)
	}

	next, err := svc.List(ctx, p, ListQuery{Limit: 2, BeforeID: page.Items[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.HasMore || next.Items[0].ID != ids[0] {
		t.Fatalf("next page = %+v, want the oldest item and no more", next.Items)
	}
}

func TestListRejectsABadCursor(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.List(context.Background(), principal(ws, u1), ListQuery{BeforeID: "nope"})
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestListOnlySeesTheCallersWorkspace(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()
	for _, n := range []domain.Notification{
		appNotification(ws, u1, ""),
		appNotification(ws, u2, ""),
		appNotification("0193f0f0-0000-7000-8000-0000000000c9", u1, ""),
	} {
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	page, err := svc.List(ctx, principal(ws, u1), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].UserID != u1 || page.Items[0].WorkspaceID != ws {
		t.Fatalf("items = %+v, want only the caller's row", page.Items)
	}
}

func TestUnreadCount(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()
	p := principal(ws, u1)
	for range 2 {
		if _, _, err := repo.Insert(ctx, appNotification(ws, u1, "")); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := repo.Insert(ctx, appNotification(ws, u2, "")); err != nil {
		t.Fatal(err)
	}
	n, err := svc.UnreadCount(ctx, p)
	if err != nil || n != 2 {
		t.Fatalf("count = %d %v, want 2", n, err)
	}
}

func TestMarkReadTellsOtherDevicesOnce(t *testing.T) {
	svc, repo, _, ev, _ := newService(t)
	ctx := context.Background()
	p := principal(ws, u1)
	n := appNotification(ws, u1, "")
	if _, _, err := repo.Insert(ctx, n); err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkRead(ctx, p, n.ID); err != nil {
		t.Fatal(err)
	}
	reads := ev.readCalls()
	if len(reads) != 1 {
		t.Fatalf("read events = %d, want 1", len(reads))
	}
	got := reads[0]
	if got.WorkspaceID != ws || got.UserID != u1 || got.All || got.Unread != 0 || len(got.IDs) != 1 || got.IDs[0] != n.ID {
		t.Fatalf("read event = %+v", got)
	}

	// Idempotent: a second mark changes nothing and says nothing.
	if err := svc.MarkRead(ctx, p, n.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(ev.readCalls()); n != 1 {
		t.Fatalf("read events after a repeat = %d, want 1", n)
	}
}

func TestMarkReadAnotherUsersNotificationIsNotFound(t *testing.T) {
	svc, repo, _, ev, _ := newService(t)
	ctx := context.Background()
	n := appNotification(ws, u2, "")
	if _, _, err := repo.Insert(ctx, n); err != nil {
		t.Fatal(err)
	}

	err := svc.MarkRead(ctx, principal(ws, u1), n.ID)
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if len(ev.readCalls()) != 0 {
		t.Fatal("a failed mark read sent an event")
	}
}

func TestReadAll(t *testing.T) {
	svc, repo, _, ev, _ := newService(t)
	ctx := context.Background()
	p := principal(ws, u1)
	var ids []string
	for range 2 {
		n := appNotification(ws, u1, "")
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	if _, _, err := repo.Insert(ctx, appNotification(ws, u2, "")); err != nil {
		t.Fatal(err)
	}

	if err := svc.ReadAll(ctx, p); err != nil {
		t.Fatal(err)
	}
	reads := ev.readCalls()
	if len(reads) != 1 {
		t.Fatalf("read events = %d, want 1", len(reads))
	}
	if !reads[0].All || reads[0].Unread != 0 || len(reads[0].IDs) != 2 {
		t.Fatalf("read event = %+v", reads[0])
	}
	if count, err := svc.UnreadCount(ctx, p); err != nil || count != 0 {
		t.Fatalf("unread = %d %v, want 0", count, err)
	}

	// Nothing unread: no event.
	if err := svc.ReadAll(ctx, p); err != nil {
		t.Fatal(err)
	}
	if n := len(ev.readCalls()); n != 1 {
		t.Fatalf("read events after a repeat = %d, want 1", n)
	}
}

func TestPruneUsesTheNinetyDayCutoff(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()

	oldRead := appNotification(ws, u1, "chat.message:old")
	recentRead := appNotification(ws, u1, "chat.message:recent")
	for _, n := range []domain.Notification{oldRead, recentRead} {
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.MarkRead(ctx, ws, u1, n.ID, fixedNow()()); err != nil {
			t.Fatal(err)
		}
	}
	// Move the first row's creation date far enough back to be pruned.
	repo.mu.Lock()
	repo.rows[0].CreatedAt = fixedNow()().Add(-91 * 24 * time.Hour)
	repo.mu.Unlock()

	deleted, err := svc.Prune(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("prune = %d %v, want 1", deleted, err)
	}
	want := fixedNow()().Add(-PruneAfter)
	if !repo.pruned.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repo.pruned, want)
	}
	left := repo.all()
	if len(left) != 1 || left[0].ID != recentRead.ID {
		t.Fatalf("left = %+v, want the recent row", left)
	}
}
