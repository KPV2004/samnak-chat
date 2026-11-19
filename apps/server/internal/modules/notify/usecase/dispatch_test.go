package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// mention is a rule result for one user and one message key.
func mention(user, kind, key string) app.Notification {
	return app.Notification{
		WorkspaceID: ws, UserID: user, Kind: kind, Title: "Ada in #general", Body: "@bob hi",
		Link: "/chat/c1", Data: map[string]any{"channel_id": "c1"}, Key: key,
	}
}

func ruleOf(ns ...app.Notification) app.NotificationRule {
	return func(context.Context, app.Event) ([]app.Notification, error) { return ns, nil }
}

func TestDispatchStoresAndDeliversAfterCommit(t *testing.T) {
	svc, repo, tx, ev, rules := newService(t)
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1"), mention(u2, "chat.mention", "chat.message:m1")))
	// A second rule for the same user and message must not add a row.
	rules.On("test.thing", ruleOf(mention(u1, "chat.dm", "chat.message:m1")))

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}

	rows := repo.all()
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2: %+v", len(rows), rows)
	}
	calls := ev.createdCalls()
	if len(calls) != 2 {
		t.Fatalf("delivered %d frames, want 2", len(calls))
	}
	for _, c := range calls {
		if c.Unread != 1 {
			t.Fatalf("unread_count = %d, want 1", c.Unread)
		}
		if c.CommitsAtSend != 1 {
			t.Fatalf("delivery happened before the commit (commits=%d)", c.CommitsAtSend)
		}
		if c.N.Kind != "chat.mention" || c.N.Link != "/chat/c1" || c.N.Data["channel_id"] != "c1" {
			t.Fatalf("unexpected frame: %+v", c.N)
		}
	}
	if calls[0].N.UserID != u1 || calls[1].N.UserID != u2 {
		t.Fatalf("frames went to %q and %q, want %q and %q", calls[0].N.UserID, calls[1].N.UserID, u1, u2)
	}
	if tx.calls != 1 || tx.commits != 1 {
		t.Fatalf("tx calls/commits = %d/%d, want 1/1", tx.calls, tx.commits)
	}
}

func TestDispatchRepeatedEventStoresAndSendsNothing(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1")))
	evt := testEvent{Name: "test.thing"}

	for range 2 {
		if err := svc.Dispatch(context.Background(), evt); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1", n)
	}
	if n := len(ev.createdCalls()); n != 1 {
		t.Fatalf("delivered %d frames, want 1", n)
	}
}

func TestDispatchBadRulesDoNotStopTheGoodOnes(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	rules.On("test.thing", func(context.Context, app.Event) ([]app.Notification, error) {
		panic("boom")
	})
	rules.On("test.thing", func(context.Context, app.Event) ([]app.Notification, error) {
		return nil, errors.New("nope")
	})
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1")))

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatalf("Dispatch = %v, want nil", err)
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1", n)
	}
	if n := len(ev.createdCalls()); n != 1 {
		t.Fatalf("delivered %d frames, want 1", n)
	}
}

func TestDispatchDropsInvalidNotifications(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	rules.On("test.thing", ruleOf(
		app.Notification{UserID: u1, Kind: "chat.mention"},                       // no workspace
		app.Notification{WorkspaceID: ws, Kind: "chat.mention"},                  // no user
		app.Notification{WorkspaceID: ws, UserID: u1},                            // no kind
		app.Notification{WorkspaceID: ws, UserID: "not-a-uuid", Kind: "chat.dm"}, // unusable id
		mention(u3, "chat.thread_reply", "chat.message:m2"),
	))

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	rows := repo.all()
	if len(rows) != 1 || rows[0].UserID != u3 || rows[0].Kind != "chat.thread_reply" {
		t.Fatalf("rows = %+v, want the one valid notification", rows)
	}
	if n := len(ev.createdCalls()); n != 1 {
		t.Fatalf("delivered %d frames, want 1", n)
	}
}

func TestDispatchStoreFailureSendsNothing(t *testing.T) {
	svc, repo, tx, ev, rules := newService(t)
	repo.insertErr = errors.New("database down")
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1")))

	err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"})
	if err == nil {
		t.Fatal("Dispatch = nil, want the store error")
	}
	if tx.calls != 1 || tx.commits != 0 {
		t.Fatalf("tx calls/commits = %d/%d, want 1/0", tx.calls, tx.commits)
	}
	if n := len(ev.createdCalls()); n != 0 {
		t.Fatalf("delivered %d frames after a failed commit, want 0", n)
	}
	if n := len(repo.all()); n != 0 {
		t.Fatalf("stored %d rows, want 0", n)
	}
}

func TestDispatchWithoutRulesDoesNothing(t *testing.T) {
	svc, repo, tx, ev, _ := newService(t)

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.nothing"}); err != nil {
		t.Fatal(err)
	}
	if tx.calls != 0 || len(repo.all()) != 0 || len(ev.createdCalls()) != 0 {
		t.Fatalf("an event with no rules touched the queue: %d tx calls", tx.calls)
	}
}

func TestDispatchToleratesMissingParts(t *testing.T) {
	// No rule source at all (a bare test), and a nil event: both are no-ops.
	repo := &fakeRepo{}
	svc := New(&fakeTx{}, repo, nil, nil, nil, quietLog(), fixedNow())
	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}

	svc, repo2, _, ev, _ := newService(t)
	if err := svc.Dispatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(repo2.all()) != 0 || len(ev.createdCalls()) != 0 {
		t.Fatal("a nil event produced notifications")
	}
}

func TestDispatchWithoutRealtimeStillStores(t *testing.T) {
	repo := &fakeRepo{}
	tx := &fakeTx{}
	rules := app.NewNotificationRules()
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1")))
	svc := New(tx, repo, rules, nil, nil, quietLog(), fixedNow())

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1", n)
	}
}

func TestDispatchDeliversWithTheCountAfterTheBatch(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	// An older unread notification makes the count 2 for u1's new frame.
	if _, _, err := repo.Insert(context.Background(), appNotification(ws, u1, "chat.message:old")); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(mention(u1, "chat.mention", "chat.message:m1")))

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	calls := ev.createdCalls()
	if len(calls) != 1 || calls[0].Unread != 2 {
		t.Fatalf("frames = %+v, want one with unread_count 2", calls)
	}
}
