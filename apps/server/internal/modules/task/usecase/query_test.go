package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

func TestGetShowsATaskOnlyToChannelReaders(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"})

	got, err := e.uc.Get(context.Background(), asBob, task.ID)
	if err != nil || got.ID != task.ID {
		t.Fatalf("bob: %+v, %v", got, err)
	}
	// carol cannot read ch: 404, not 403, so the task's existence stays hidden.
	if _, err := e.uc.Get(context.Background(), asCarol, task.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("carol: err = %v, want ErrNotFound", err)
	}
	// Another workspace's task is just as missing.
	other := principal(bob, app.RoleMember)
	other.WorkspaceID = "0193f0f0-0000-7000-8000-0000000000a9"
	if _, err := e.uc.Get(context.Background(), other, task.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("other workspace: err = %v, want ErrNotFound", err)
	}
	if _, err := e.uc.Get(context.Background(), asBob, "nope"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("bad id: err = %v, want ErrNotFound", err)
	}
}

func TestGetFailsClosedWithoutChannelAccess(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"})
	e.buildWithoutChannels()
	if _, err := e.uc.Get(context.Background(), asAlice, task.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound when chat is not providing ChannelAccess", err)
	}
}

// seed stores n tasks in ch, newest last, with increasing IDs.
func (e *env) seed(n int, mod func(i int, t *domain.Task)) []domain.Task {
	e.t.Helper()
	var out []domain.Task
	for i := range n {
		task := domain.Task{
			ID: fmt.Sprintf("0193f0f0-1000-7000-8000-%012d", i+1), WorkspaceID: ws, ChannelID: ch, Title: fmt.Sprintf("task %d", i+1),
			CreatorID: alice, Priority: domain.PriorityNormal, Status: domain.StatusTodo, CreatedAt: clock, UpdatedAt: clock,
		}
		if mod != nil {
			mod(i, &task)
		}
		e.repo.put(task)
		out = append(out, task)
	}
	return out
}

func titles(items []domain.Task) []string {
	out := make([]string, len(items))
	for i, k := range items {
		out[i] = k.Title
	}
	return out
}

func TestListHidesTasksOfUnreadableChannels(t *testing.T) {
	e := newEnv(t)
	e.seed(4, func(i int, k *domain.Task) {
		if i%2 == 1 {
			k.ChannelID = ch2 // only carol reads ch2
		}
	})
	page, err := e.uc.List(context.Background(), asAlice, usecase.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(titles(page.Items)) != "[task 3 task 1]" || page.NextCursor != "" {
		t.Fatalf("alice = %v next=%q", titles(page.Items), page.NextCursor)
	}
	page, _ = e.uc.List(context.Background(), asCarol, usecase.ListQuery{})
	if fmt.Sprint(titles(page.Items)) != "[task 4 task 2]" {
		t.Fatalf("carol = %v", titles(page.Items))
	}
}

func TestListChecksEachChannelOnce(t *testing.T) {
	e := newEnv(t)
	e.seed(30, nil)
	if _, err := e.uc.List(context.Background(), asAlice, usecase.ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if e.channels.calls != 1 {
		t.Fatalf("CanRead calls = %d, want 1 for one channel", e.channels.calls)
	}
}

func TestListPagesWithACursor(t *testing.T) {
	e := newEnv(t)
	e.seed(5, nil)
	ctx := context.Background()

	p1, err := e.uc.List(ctx, asAlice, usecase.ListQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(titles(p1.Items)) != "[task 5 task 4]" || p1.NextCursor == "" {
		t.Fatalf("page 1 = %v next=%q", titles(p1.Items), p1.NextCursor)
	}
	p2, _ := e.uc.List(ctx, asAlice, usecase.ListQuery{Limit: 2, Cursor: p1.NextCursor})
	if fmt.Sprint(titles(p2.Items)) != "[task 3 task 2]" || p2.NextCursor == "" {
		t.Fatalf("page 2 = %v next=%q", titles(p2.Items), p2.NextCursor)
	}
	p3, _ := e.uc.List(ctx, asAlice, usecase.ListQuery{Limit: 2, Cursor: p2.NextCursor})
	if fmt.Sprint(titles(p3.Items)) != "[task 1]" || p3.NextCursor != "" {
		t.Fatalf("page 3 = %v next=%q", titles(p3.Items), p3.NextCursor)
	}
}

func TestListPagesPastHiddenTasks(t *testing.T) {
	e := newEnv(t)
	// The newest 120 tasks are in a channel alice cannot read.
	e.seed(130, func(i int, k *domain.Task) {
		if i >= 10 {
			k.ChannelID = ch2
		}
	})
	page, err := e.uc.List(context.Background(), asAlice, usecase.ListQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 5 || page.Items[0].Title != "task 10" || page.NextCursor == "" {
		t.Fatalf("items = %v next=%q", titles(page.Items), page.NextCursor)
	}
}

func TestListFilters(t *testing.T) {
	e := newEnv(t)
	day := clock.Truncate(24 * time.Hour) // 2026-03-04 00:00 UTC
	e.seed(6, func(i int, k *domain.Task) {
		switch i {
		case 0: // overdue, mine
			k.AssigneeID, k.DueAt = alice, ptr(clock.Add(-time.Hour))
		case 1: // due later today, mine
			k.AssigneeID, k.DueAt, k.Status = alice, ptr(day.Add(20*time.Hour)), domain.StatusDoing
		case 2: // due in three days, bob's, created by bob
			k.AssigneeID, k.CreatorID, k.DueAt = bob, bob, ptr(day.Add(3*24*time.Hour))
		case 3: // overdue but done
			k.AssigneeID, k.Status, k.DueAt = alice, domain.StatusDone, ptr(clock.Add(-2*time.Hour))
		case 4: // due in ten days
			k.DueAt = ptr(day.Add(10 * 24 * time.Hour))
		}
	})
	list := func(q usecase.ListQuery) string {
		t.Helper()
		page, err := e.uc.List(context.Background(), asAlice, q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		return fmt.Sprint(titles(page.Items))
	}
	cases := []struct {
		name string
		q    usecase.ListQuery
		want string
	}{
		{"assignee me", usecase.ListQuery{Assignee: "me"}, "[task 4 task 2 task 1]"},
		{"assignee any", usecase.ListQuery{Assignee: "any"}, "[task 6 task 5 task 4 task 3 task 2 task 1]"},
		{"creator me", usecase.ListQuery{Creator: "me"}, "[task 6 task 5 task 4 task 2 task 1]"},
		{"status", usecase.ListQuery{Statuses: []string{"todo", "doing"}}, "[task 6 task 5 task 3 task 2 task 1]"},
		{"channel", usecase.ListQuery{ChannelID: ch}, "[task 6 task 5 task 4 task 3 task 2 task 1]"},
		{"overdue leaves out done", usecase.ListQuery{Due: "overdue"}, "[task 1]"},
		{"today", usecase.ListQuery{Due: "today"}, "[task 4 task 2 task 1]"},
		{"week", usecase.ListQuery{Due: "week"}, "[task 4 task 3 task 2 task 1]"},
	}
	for _, c := range cases {
		if got := list(c.q); got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestListRejectsBadFilters(t *testing.T) {
	e := newEnv(t)
	for name, q := range map[string]usecase.ListQuery{
		"assignee": {Assignee: "bob"},
		"creator":  {Creator: "any"},
		"status":   {Statuses: []string{"todo", "blocked"}},
		"due":      {Due: "tomorrow"},
		"channel":  {ChannelID: "nope"},
		"cursor":   {Cursor: "nope"},
	} {
		if _, err := e.uc.List(context.Background(), asAlice, q); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestListLimits(t *testing.T) {
	e := newEnv(t)
	e.seed(120, nil)
	page, _ := e.uc.List(context.Background(), asAlice, usecase.ListQuery{})
	if len(page.Items) != usecase.DefaultLimit {
		t.Fatalf("default page = %d, want %d", len(page.Items), usecase.DefaultLimit)
	}
	page, _ = e.uc.List(context.Background(), asAlice, usecase.ListQuery{Limit: 1000})
	if len(page.Items) != usecase.MaxLimit {
		t.Fatalf("capped page = %d, want %d", len(page.Items), usecase.MaxLimit)
	}
}

func TestListOnAnUnreadableChannelIsEmpty(t *testing.T) {
	e := newEnv(t)
	e.seed(3, nil)
	page, err := e.uc.List(context.Background(), asCarol, usecase.ListQuery{ChannelID: ch})
	if err != nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("page = %+v err = %v", page, err)
	}
}
