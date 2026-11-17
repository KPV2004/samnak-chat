package usecase_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

func (e *env) update(p app.Principal, id string, patch domain.Patch) (domain.Task, error) {
	e.t.Helper()
	return e.uc.Update(context.Background(), p, id, patch)
}

// editable is a task alice created for bob in ch.
func (e *env) editable() domain.Task {
	e.t.Helper()
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Fix login", AssigneeID: bob})
	e.events.names = nil
	e.now = clock.Add(time.Hour)
	return task
}

func TestUpdateRights(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	e.channels.allow(ch, carol) // carol can read the channel now, but neither made nor owns the task
	patch := domain.Patch{Title: ptr("Changed")}

	for _, c := range []struct {
		name string
		p    app.Principal
		want error
	}{
		{"creator", asAlice, nil},
		{"assignee", asBob, nil},
		{"admin", asAdmin, nil},
		{"owner", principal(admin, app.RoleOwner), nil},
		{"member who can read", asCarol, app.ErrForbidden},
		{"manager who can read", principal(carol, app.RoleManager), app.ErrForbidden},
	} {
		_, err := e.update(c.p, task.ID, patch)
		if !errors.Is(err, c.want) || (c.want == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestUpdateHidesATaskFromPeopleWhoCannotReadTheChannel(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	// carol cannot read ch: 404 even though she would otherwise be refused.
	if _, err := e.update(asCarol, task.ID, domain.Patch{Title: ptr("x")}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// An admin who cannot read a private channel cannot edit its tasks either.
	e.channels.readers[ch+"/"+admin] = false
	if _, err := e.update(asAdmin, task.ID, domain.Patch{Title: ptr("x")}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("admin: err = %v, want ErrNotFound", err)
	}
	if _, err := e.update(asAlice, "nope", domain.Patch{Title: ptr("x")}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("bad id: err = %v, want ErrNotFound", err)
	}
}

func TestUpdateStatusChangeFollowsTheSameRule(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	e.channels.allow(ch, carol)
	if _, err := e.update(asCarol, task.ID, domain.Patch{Status: ptr(domain.StatusDone)}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	got, err := e.update(asBob, task.ID, domain.Patch{Status: ptr(domain.StatusDoing)})
	if err != nil || got.Status != domain.StatusDoing {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestUpdatePublishesTheEventsThatMatch(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	ctx := context.Background()

	if _, err := e.uc.Update(ctx, asBob, task.ID, domain.Patch{Status: ptr(domain.StatusDoing)}); err != nil {
		t.Fatal(err)
	}
	if got := e.events.got(); !slices.Equal(got, []string{"status_changed"}) {
		t.Fatalf("status events = %v", got)
	}
	se := e.events.last["status_changed"].(statusEvent)
	if se.From != domain.StatusTodo || se.Task.Status != domain.StatusDoing || se.Actor != bob {
		t.Fatalf("status event = %+v", se)
	}

	e.events.names = nil
	e.channels.allow(ch, admin)
	if _, err := e.uc.Update(ctx, asAlice, task.ID, domain.Patch{AssigneeSet: true, AssigneeID: admin}); err != nil {
		t.Fatal(err)
	}
	if got := e.events.got(); !slices.Equal(got, []string{"assigned"}) {
		t.Fatalf("assign events = %v", got)
	}
	as := e.events.last["assigned"].(assignedEvent)
	if as.Actor != alice || as.Task.AssigneeID != admin || !as.Task.UpdatedAt.Equal(e.now) {
		t.Fatalf("assigned = %+v", as)
	}

	e.events.names = nil
	if _, err := e.uc.Update(ctx, asAlice, task.ID, domain.Patch{Title: ptr("New title"), Priority: ptr(domain.PriorityHigh)}); err != nil {
		t.Fatal(err)
	}
	if got := e.events.got(); !slices.Equal(got, []string{"updated"}) {
		t.Fatalf("field events = %v", got)
	}

	e.events.names = nil
	patch := domain.Patch{Status: ptr(domain.StatusDone), AssigneeSet: true, AssigneeID: bob, Description: ptr("d")}
	if _, err := e.uc.Update(ctx, asAlice, task.ID, patch); err != nil {
		t.Fatal(err)
	}
	if got := e.events.got(); !slices.Equal(got, []string{"assigned", "status_changed", "updated"}) {
		t.Fatalf("combined events = %v", got)
	}
	if len(e.events.outTx) != 0 {
		t.Fatalf("published outside the transaction: %v", e.events.outTx)
	}
}

func TestUpdateWithNoChangeWritesAndPublishesNothing(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	e.repo.updateErr = errors.New("must not write")
	got, err := e.update(asAlice, task.ID, domain.Patch{Title: ptr("Fix login"), Status: ptr(domain.StatusTodo), AssigneeSet: true, AssigneeID: bob})
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(task.UpdatedAt) || len(e.events.got()) != 0 {
		t.Fatalf("got %+v events %v", got, e.events.got())
	}
}

func TestUpdateClearsAssigneeAndDue(t *testing.T) {
	e := newEnv(t)
	due := clock.Add(24 * time.Hour)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x", AssigneeID: bob, DueAt: &due})
	e.events.names = nil
	got, err := e.update(asAlice, task.ID, domain.Patch{AssigneeSet: true, DueSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.AssigneeID != "" || got.DueAt != nil {
		t.Fatalf("got %+v", got)
	}
	if ev := e.events.got(); !slices.Equal(ev, []string{"updated"}) {
		t.Fatalf("events = %v, want a plain update", ev)
	}
}

func TestUpdateChangingTheDueDateRearmsTheScan(t *testing.T) {
	e := newEnv(t)
	due := clock.Add(10 * time.Hour)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x", AssigneeID: bob, DueAt: &due})
	stored := e.repo.tasks[task.ID]
	stored.DueSoonSentAt, stored.OverdueSentAt = &clock, &clock
	e.repo.put(stored)

	later := due.Add(48 * time.Hour)
	got, err := e.update(asAlice, task.ID, domain.Patch{DueSet: true, DueAt: &later})
	if err != nil {
		t.Fatal(err)
	}
	if got.DueSoonSentAt != nil || got.OverdueSentAt != nil {
		t.Fatalf("marks survived a new due date: %+v", got)
	}
}

func TestUpdateValidatesTheNewAssignee(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	for name, id := range map[string]string{"cannot read the channel": carol, "not a member": dave, "bad id": "nope"} {
		e.channels.allow(ch, dave)
		if _, err := e.update(asAlice, task.ID, domain.Patch{AssigneeSet: true, AssigneeID: id}); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if len(e.events.got()) != 0 {
		t.Fatalf("events = %v", e.events.got())
	}
}

func TestUpdateKeepsAnExistingAssigneeWhoLeft(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	delete(e.members.(fakeMembers), bob) // bob left the workspace
	// Editing other fields must not fail on an assignee nobody changed.
	if _, err := e.update(asAlice, task.ID, domain.Patch{Title: ptr("Renamed")}); err != nil {
		t.Fatal(err)
	}
	// Re-sending the same assignee is a no-op and is not re-validated.
	if _, err := e.update(asAlice, task.ID, domain.Patch{AssigneeSet: true, AssigneeID: bob}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateInvalidFieldsAreRejected(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	for name, p := range map[string]domain.Patch{
		"empty title": {Title: ptr(" ")},
		"status":      {Status: ptr(domain.Status("blocked"))},
		"priority":    {Priority: ptr(domain.Priority("urgent"))},
	} {
		if _, err := e.update(asAlice, task.ID, p); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestUpdateFailsWhenPublishingFails(t *testing.T) {
	e := newEnv(t)
	task := e.editable()
	e.events.failOn = "status_changed"
	if _, err := e.update(asAlice, task.ID, domain.Patch{Status: ptr(domain.StatusDone)}); err == nil {
		t.Fatal("want the publish error so the transaction rolls back")
	}
}
