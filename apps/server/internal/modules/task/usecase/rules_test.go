package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

func (e *env) assignedTask() domain.Task {
	e.t.Helper()
	due := clock.Add(10 * time.Hour)
	return e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Fix login", AssigneeID: bob, DueAt: &due})
}

func TestNotifyAssignedTellsTheAssignee(t *testing.T) {
	e := newEnv(t)
	task := e.assignedTask()
	in := usecase.AssignedInput{WorkspaceID: ws, TaskID: task.ID, AssigneeID: bob, ActorID: alice, UpdatedAt: task.UpdatedAt}

	got, err := e.uc.NotifyAssigned(context.Background(), in)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	n := got[0]
	wantKey := fmt.Sprintf("task.assigned:%s:%s:%s", task.ID, bob, task.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if n.WorkspaceID != ws || n.UserID != bob || n.Kind != "task.assigned" || n.Key != wantKey || n.Link != "/chat/"+ch {
		t.Fatalf("notification = %+v", n)
	}
	if n.Title != "Alice assigned you a task" || n.Body != "Fix login" {
		t.Fatalf("title/body = %q / %q", n.Title, n.Body)
	}
	if n.Data["task_id"] != task.ID || n.Data["channel_id"] != ch || n.Data["actor_id"] != alice || n.Data["title"] != "Fix login" {
		t.Fatalf("data = %+v", n.Data)
	}
}

func TestNotifyAssignedSkipsSelfAssignmentAndGoneTasks(t *testing.T) {
	e := newEnv(t)
	task := e.assignedTask()
	ctx := context.Background()

	self := usecase.AssignedInput{WorkspaceID: ws, TaskID: task.ID, AssigneeID: bob, ActorID: bob, UpdatedAt: task.UpdatedAt}
	if got, err := e.uc.NotifyAssigned(ctx, self); err != nil || len(got) != 0 {
		t.Fatalf("self assignment: %+v, %v", got, err)
	}
	gone := usecase.AssignedInput{WorkspaceID: ws, TaskID: "0193f0f0-0000-7000-8000-00000000eeee", AssigneeID: bob, ActorID: alice, UpdatedAt: clock}
	if got, err := e.uc.NotifyAssigned(ctx, gone); err != nil || len(got) != 0 {
		t.Fatalf("gone task: %+v, %v", got, err)
	}
	none := usecase.AssignedInput{WorkspaceID: ws, TaskID: task.ID, ActorID: alice, UpdatedAt: clock}
	if got, err := e.uc.NotifyAssigned(ctx, none); err != nil || len(got) != 0 {
		t.Fatalf("no assignee: %+v, %v", got, err)
	}
}

func TestNotifyAssignedWithoutADirectoryStillNotifies(t *testing.T) {
	e := newEnv(t)
	task := e.assignedTask()
	e.dir = nil
	e.build()
	got, err := e.uc.NotifyAssigned(context.Background(), usecase.AssignedInput{WorkspaceID: ws, TaskID: task.ID, AssigneeID: bob, ActorID: alice, UpdatedAt: task.UpdatedAt})
	if err != nil || len(got) != 1 || got[0].Title != "You were assigned a task" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNotifyDue(t *testing.T) {
	e := newEnv(t)
	task := e.assignedTask()
	ctx := context.Background()
	in := usecase.DueInput{WorkspaceID: ws, TaskID: task.ID, AssigneeID: bob, DueAt: *task.DueAt}

	soon, err := e.uc.NotifyDue(ctx, usecase.DueSoon, in)
	if err != nil || len(soon) != 1 {
		t.Fatalf("due soon: %+v, %v", soon, err)
	}
	n := soon[0]
	if n.UserID != bob || n.Kind != "task.due_soon" || n.Link != "/chat/"+ch || n.Body != "Fix login" ||
		n.Key != fmt.Sprintf("task.due_soon:%s:%s", task.ID, task.DueAt.UTC().Format(time.RFC3339Nano)) {
		t.Fatalf("due soon = %+v", n)
	}
	if n.Data["task_id"] != task.ID || n.Data["due_at"] == nil {
		t.Fatalf("data = %+v", n.Data)
	}

	over, _ := e.uc.NotifyDue(ctx, usecase.Overdue, in)
	if len(over) != 1 || over[0].Kind != "task.overdue" || over[0].Key != fmt.Sprintf("task.overdue:%s:%s", task.ID, task.DueAt.UTC().Format(time.RFC3339Nano)) {
		t.Fatalf("overdue = %+v", over)
	}
}

func TestNotifyDueSkipsStaleEvents(t *testing.T) {
	e := newEnv(t)
	task := e.assignedTask()
	ctx := context.Background()
	in := usecase.DueInput{WorkspaceID: ws, TaskID: task.ID, AssigneeID: bob, DueAt: *task.DueAt}

	// The task was reassigned, rescheduled or finished before the event was handled.
	cases := map[string]func(*domain.Task){
		"reassigned":  func(k *domain.Task) { k.AssigneeID = carol },
		"rescheduled": func(k *domain.Task) { d := k.DueAt.Add(time.Hour); k.DueAt = &d },
		"done":        func(k *domain.Task) { k.Status = domain.StatusDone },
		"unassigned":  func(k *domain.Task) { k.AssigneeID = "" },
	}
	for name, mod := range cases {
		stored := e.repo.tasks[task.ID]
		changed := stored
		mod(&changed)
		e.repo.put(changed)
		if got, err := e.uc.NotifyDue(ctx, usecase.DueSoon, in); err != nil || len(got) != 0 {
			t.Errorf("%s: got %+v, %v, want nothing", name, got, err)
		}
		e.repo.put(stored)
	}
	gone := in
	gone.TaskID = "0193f0f0-0000-7000-8000-00000000eeee"
	if got, err := e.uc.NotifyDue(ctx, usecase.DueSoon, gone); err != nil || len(got) != 0 {
		t.Errorf("gone task: %+v, %v", got, err)
	}
}

func TestDueScanPublishesAndStops(t *testing.T) {
	e := newEnv(t)
	for i := range 3 { // due within 24 hours
		e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: fmt.Sprintf("soon %d", i), AssigneeID: bob, DueAt: ptr(clock.Add(time.Duration(i+1) * time.Hour))})
	}
	for i := range 2 { // already due
		e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: fmt.Sprintf("late %d", i), AssigneeID: bob, DueAt: ptr(clock.Add(-time.Duration(i+1) * time.Hour))})
	}
	e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "far", AssigneeID: bob, DueAt: ptr(clock.Add(48 * time.Hour))})
	e.events.names, e.events.outTx = nil, nil

	if err := e.uc.DueScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, n := range e.events.got() {
		counts[n]++
	}
	if counts["due_soon"] != 3 || counts["overdue"] != 2 || len(counts) != 2 {
		t.Fatalf("events = %v", counts)
	}
	if len(e.events.outTx) != 0 {
		t.Fatalf("published outside the transaction: %v", e.events.outTx)
	}
	d := e.events.last["overdue"].(usecase.Due)
	if d.WorkspaceID != ws || d.AssigneeID != bob || d.DueAt.IsZero() {
		t.Fatalf("due event = %+v", d)
	}

	// Everything is marked, so a second scan announces nothing.
	e.events.names = nil
	if err := e.uc.DueScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.events.got(); len(got) != 0 {
		t.Fatalf("second scan published %v", got)
	}
}

func TestDueScanHandlesMoreThanOneBatch(t *testing.T) {
	e := newEnv(t)
	n := usecase.DueBatch*2 + 7
	for i := range n {
		e.repo.put(domain.Task{
			ID: fmt.Sprintf("0193f0f0-2000-7000-8000-%012d", i), WorkspaceID: ws, ChannelID: ch, Title: "late", CreatorID: alice,
			AssigneeID: bob, DueAt: ptr(clock.Add(-time.Hour)), Priority: domain.PriorityNormal, Status: domain.StatusTodo,
		})
	}
	if err := e.uc.DueScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(e.events.got()); got != n {
		t.Fatalf("events = %d, want %d", got, n)
	}
}

func TestDueScanStopsOnAPublishError(t *testing.T) {
	e := newEnv(t)
	e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "late", AssigneeID: bob, DueAt: ptr(clock.Add(-time.Hour))})
	e.events.failOn = "overdue"
	if err := e.uc.DueScan(context.Background()); err == nil {
		t.Fatal("want the error so the job is retried and the mark rolls back")
	}
}

func TestCreateFromCommand(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	err := e.uc.CreateFromCommand(ctx, asAlice, app.CommandInput{ChannelID: ch, Args: "Fix the login <@" + bob + "> page"})
	if err != nil {
		t.Fatal(err)
	}
	var got domain.Task
	for _, v := range e.repo.tasks {
		got = v
	}
	if got.Title != "Fix the login page" || got.AssigneeID != bob || got.CreatorID != alice || got.ChannelID != ch || got.Status != domain.StatusTodo {
		t.Fatalf("task = %+v", got)
	}
	if ev := e.events.got(); len(ev) != 2 || ev[0] != "created" || ev[1] != "assigned" {
		t.Fatalf("events = %v", ev)
	}
}

func TestCreateFromCommandErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		p    app.Principal
		in   app.CommandInput
		want error
	}{
		"no title":        {asAlice, app.CommandInput{ChannelID: ch, Args: ""}, app.ErrInvalid},
		"only a mention":  {asAlice, app.CommandInput{ChannelID: ch, Args: "<@" + bob + ">"}, app.ErrInvalid},
		"cannot read":     {asCarol, app.CommandInput{ChannelID: ch, Args: "x"}, app.ErrNotFound},
		"outsider assign": {asAlice, app.CommandInput{ChannelID: ch, Args: "x <@" + carol + ">"}, app.ErrInvalid},
	} {
		if err := e.uc.CreateFromCommand(ctx, tc.p, tc.in); !isErr(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if len(e.repo.tasks) != 0 {
		t.Fatalf("tasks = %d, want none", len(e.repo.tasks))
	}
}
