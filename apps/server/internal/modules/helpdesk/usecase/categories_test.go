package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

func TestCategoryNeedsAnActiveQueue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: newID(), Name: "Laptop"})
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("unknown queue: err = %v, want ErrInvalid", err)
	}
	q := e.createQueue(t, "alice", "IT", "IT")
	archived := true
	if _, err := e.svc.UpdateQueue(ctx, e.p["alice"], q.ID, domain.QueuePatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("archived queue: err = %v, want ErrInvalid", err)
	}
}

func TestOneDefaultCategoryPerWorkspace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	first, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Email", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.repo.defaults(ws); len(got) != 1 || got[0] != second.ID {
		t.Fatalf("defaults = %v, want only %s (the latest)", got, second.ID)
	}
	if first.ID == second.ID {
		t.Fatal("two categories share an ID")
	}
}

func TestSettingADefaultLaterMovesIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	plain, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := e.svc.UpdateCategory(ctx, e.p["alice"], plain.ID, domain.CategoryPatch{IsDefault: &yes}); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.defaults(ws); len(got) != 1 || got[0] != plain.ID {
		t.Fatalf("defaults = %v, want %s", got, plain.ID)
	}
}

func TestArchivingTheDefaultDropsIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	c, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	archived := true
	if _, err := e.svc.UpdateCategory(ctx, e.p["alice"], c.ID, domain.CategoryPatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.defaults(ws); len(got) != 0 {
		t.Fatalf("defaults = %v after archiving, want none", got)
	}
	list, err := e.svc.ListQueues(ctx, e.p["carol"])
	if err != nil {
		t.Fatal(err)
	}
	if len(list[0].Categories) != 0 {
		t.Errorf("archived category still listed: %+v", list[0].Categories)
	}
}

func TestMovingACategoryNeedsAnActiveQueue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.createQueue(t, "alice", "IT", "IT")
	hr := e.createQueue(t, "alice", "HR", "HR")
	c, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: it.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	gone := newID()
	if _, err := e.svc.UpdateCategory(ctx, e.p["alice"], c.ID, domain.CategoryPatch{QueueID: &gone}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("unknown queue: err = %v, want ErrInvalid", err)
	}
	moved, err := e.svc.UpdateCategory(ctx, e.p["alice"], c.ID, domain.CategoryPatch{QueueID: &hr.ID})
	if err != nil {
		t.Fatal(err)
	}
	if moved.QueueID != hr.ID {
		t.Errorf("queue = %s, want HR", moved.QueueID)
	}
}

func TestCategoryOfAnotherWorkspaceIsNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	c, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	other := e.p["alice"]
	other.WorkspaceID = ws2
	name := "X"
	if _, err := e.svc.UpdateCategory(ctx, other, c.ID, domain.CategoryPatch{Name: &name}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
