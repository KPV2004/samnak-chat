package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// workSetup makes an IT queue staffed by carol, a Laptop category, and a
// ticket opened by bob. It returns the ticket's ID and category.
func (e *env) workSetup(t *testing.T) (string, domain.Category) {
	t.Helper()
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, []string{carol}); err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	tk, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "Printer jams"})
	if err != nil {
		t.Fatal(err)
	}
	return tk.ID, c
}

func (e *env) historyActions(ticketID string) []domain.Action {
	var out []domain.Action
	for _, h := range e.repo.history {
		if h.TicketID == ticketID {
			out = append(out, h.Action)
		}
	}
	return out
}

func TestAnAgentClaimsANewTicketAndItOpens(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()

	if _, err := e.svc.Assign(ctx, e.p["bob"], id, bob); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("requester claiming: err = %v, want ErrForbidden", err)
	}
	tk, err := e.svc.Assign(ctx, e.p["carol"], id, carol)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != domain.StatusOpen || tk.AssigneeID != carol {
		t.Fatalf("after claim: %+v", tk)
	}
	if acts := e.historyActions(id); len(acts) != 3 || acts[1] != domain.ActionAssigned || acts[2] != domain.ActionStatusChanged {
		t.Errorf("history = %v, want created, assigned, status_changed", acts)
	}
	var sawAssigned, sawOpened bool
	for _, evt := range e.bus.all() {
		switch v := evt.(type) {
		case domain.TicketAssigned:
			sawAssigned = v.AssigneeID == carol && v.ActorID == carol
		case domain.StatusChanged:
			sawOpened = v.From == "new" && v.To == "open"
		}
	}
	if !sawAssigned || !sawOpened {
		t.Errorf("events = %#v, want ticket_assigned and status_changed new->open", e.bus.all())
	}

	// Claiming again by the same agent changes nothing.
	before := len(e.bus.all())
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	if len(e.bus.all()) != before {
		t.Error("a repeated claim published events")
	}
}

func TestAdminsAssignOnlyAgentsOfTheQueue(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["alice"], id, bob); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("admin assigning a non-agent: err = %v, want ErrInvalid", err)
	}
	if _, err := e.svc.Assign(ctx, e.p["alice"], id, "nope"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("malformed assignee: err = %v, want ErrInvalid", err)
	}
	if _, err := e.svc.Assign(ctx, e.p["alice"], id, carol); err != nil {
		t.Fatalf("admin assigning an agent: %v", err)
	}
}

func TestOutsidersDoNotSeeTheTicketToClaimIt(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	if _, err := e.svc.Assign(context.Background(), e.p["outside"], id, outside); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStatusFlowFromWorkToClosedRecordsEveryStepAndResolution(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		who string
		to  domain.Status
	}{
		{"carol", domain.StatusInProgress},
		{"carol", domain.StatusWaiting},
		{"carol", domain.StatusInProgress},
		{"carol", domain.StatusResolved},
		{"bob", domain.StatusClosed},
	}
	for _, s := range steps {
		tk, err := e.svc.ChangeStatus(ctx, e.p[s.who], id, s.to)
		if err != nil {
			t.Fatalf("%s -> %s: %v", s.who, s.to, err)
		}
		if tk.Status != s.to {
			t.Fatalf("status = %s, want %s", tk.Status, s.to)
		}
	}
	closed, err := e.repo.Ticket(ctx, ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if closed.ResolvedAt == nil || closed.ClosedAt == nil {
		t.Errorf("resolved_at = %v, closed_at = %v", closed.ResolvedAt, closed.ClosedAt)
	}
	resolved := 0
	for _, evt := range e.bus.all() {
		if _, ok := evt.(domain.TicketResolved); ok {
			resolved++
		}
	}
	if resolved != 1 {
		t.Errorf("ticket_resolved published %d times, want once", resolved)
	}
}

func TestStatusMovesAreCheckedAndRefusedWithTheRightCode(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ChangeStatus(ctx, e.p["bob"], id, domain.StatusInProgress); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("requester starting work: err = %v, want ErrForbidden", err)
	}
	if _, err := e.svc.ChangeStatus(ctx, e.p["carol"], id, domain.StatusClosed); !errors.Is(err, app.ErrConflict) {
		t.Errorf("open -> closed: err = %v, want ErrConflict", err)
	}
	if _, err := e.svc.ChangeStatus(ctx, e.p["carol"], id, "bogus"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("unknown status: err = %v, want ErrInvalid", err)
	}
}

func TestReopenIsOnlyAllowedWithinSevenDays(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	for _, to := range []domain.Status{domain.StatusInProgress, domain.StatusResolved} {
		if _, err := e.svc.ChangeStatus(ctx, e.p["carol"], id, to); err != nil {
			t.Fatal(err)
		}
	}
	e.now = t0.Add(6 * 24 * time.Hour)
	reopened, err := e.svc.ChangeStatus(ctx, e.p["bob"], id, domain.StatusOpen)
	if err != nil {
		t.Fatalf("reopen on day six: %v", err)
	}
	if reopened.Status != domain.StatusOpen || reopened.AssigneeID != carol || reopened.ResolvedAt != nil {
		t.Errorf("reopened = %+v; the assignee stays and resolved_at clears", reopened)
	}

	for _, to := range []domain.Status{domain.StatusInProgress, domain.StatusResolved} {
		if _, err := e.svc.ChangeStatus(ctx, e.p["carol"], id, to); err != nil {
			t.Fatal(err)
		}
	}
	e.now = t0.Add(20 * 24 * time.Hour)
	if _, err := e.svc.ChangeStatus(ctx, e.p["bob"], id, domain.StatusOpen); !errors.Is(err, app.ErrConflict) {
		t.Errorf("reopen after the window: err = %v, want ErrConflict", err)
	}
}

func TestPriorityIsChangedByAgentsAndRecorded(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	high := domain.PriorityHigh
	if _, err := e.svc.Edit(ctx, e.p["bob"], id, usecase.EditInput{Priority: &high}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("requester editing: err = %v, want ErrForbidden", err)
	}
	tk, err := e.svc.Edit(ctx, e.p["carol"], id, usecase.EditInput{Priority: &high})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Priority != domain.PriorityHigh {
		t.Fatalf("priority = %s", tk.Priority)
	}
	if acts := e.historyActions(id); acts[len(acts)-1] != domain.ActionPriorityChanged {
		t.Errorf("history = %v, want priority_changed last", acts)
	}
	var updated bool
	for _, evt := range e.bus.all() {
		if _, ok := evt.(domain.TicketUpdated); ok {
			updated = true
		}
	}
	if !updated {
		t.Error("no ticket_updated event")
	}
}

func TestMovingToAnotherQueueKeepsTheNumberAndDropsAnAssigneeWhoIsNotThere(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	hr := e.createQueue(t, "alice", "HR", "HR")
	leave, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: hr.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := e.svc.Edit(ctx, e.p["alice"], id, usecase.EditInput{CategoryID: &leave.ID})
	if err != nil {
		t.Fatal(err)
	}
	if moved.QueueID != hr.ID || moved.Number != 1 || moved.Key != "HR-0001" {
		t.Errorf("moved = %+v; the number stays and the key takes the new prefix", moved)
	}
	if moved.AssigneeID != "" {
		t.Errorf("assignee = %q; carol is not an agent of HR", moved.AssigneeID)
	}
}

func TestMovingIntoAQueueWhoseNumberIsTakenIsAConflict(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	hr := e.createQueue(t, "alice", "HR", "HR")
	leave, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: hr.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: leave.ID, Title: "Leave"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Edit(ctx, e.p["alice"], id, usecase.EditInput{CategoryID: &leave.ID}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	stored, err := e.repo.Ticket(ctx, ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Key != "IT-0001" {
		t.Errorf("a refused move changed the ticket to %s", stored.Key)
	}
}

func TestEditRefusesArchivedCategories(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "HR", "HR")
	email, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Email"})
	if err != nil {
		t.Fatal(err)
	}
	archived := true
	if _, err := e.svc.UpdateCategory(ctx, e.p["alice"], email.ID, domain.CategoryPatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Edit(ctx, e.p["carol"], id, usecase.EditInput{CategoryID: &email.ID}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}
