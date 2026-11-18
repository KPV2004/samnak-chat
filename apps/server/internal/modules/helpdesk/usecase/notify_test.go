package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// refOf returns the ref every ticket event carries for the stored ticket.
func (e *env) refOf(t *testing.T, id string) domain.TicketRef {
	t.Helper()
	tk, err := e.repo.Ticket(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.TicketRef{
		WorkspaceID: tk.WorkspaceID, TicketID: tk.ID, QueueID: tk.QueueID, Key: tk.Key, RequesterID: tk.RequesterID,
	}
}

// recipients lists the users the notifications go to.
func recipients(ns []app.Notification) map[string]bool {
	out := map[string]bool{}
	for _, n := range ns {
		out[n.UserID] = true
	}
	return out
}

// TestACreatedTicketTellsTheQueueAgents: a new ticket notifies the agents of
// its queue, never its requester, once per user with a stable key.
func TestACreatedTicketTellsTheQueueAgents(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	e.now = e.now.Add(time.Minute)
	// bob, the requester, is also staffed on the queue in this test, and must
	// still not be told about his own ticket.
	q, err := e.repo.Queue(context.Background(), ws, e.refOf(t, id).QueueID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetAgents(context.Background(), e.p["alice"], q.ID, []string{bob, carol}); err != nil {
		t.Fatal(err)
	}
	evt := domain.TicketCreated{TicketRef: e.refOf(t, id)}
	ns, err := e.svc.Notify(context.Background(), evt)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].UserID != carol {
		t.Fatalf("notified %v, want only carol", recipients(ns))
	}
	n := ns[0]
	if n.Kind != usecase.KindTicketCreated || n.Link != "/helpdesk/"+id {
		t.Errorf("notification = %+v", n)
	}
	if n.Data["ticket_id"] != id || n.Data["key"] != e.refOf(t, id).Key {
		t.Errorf("data = %v", n.Data)
	}
	again, err := e.svc.Notify(context.Background(), evt)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Key != n.Key {
		t.Errorf("redelivery key = %q, want %q", again[0].Key, n.Key)
	}
}

// TestAnAssignmentTellsTheAssigneeButNotTheClaimer: an assignment made by
// someone else notifies the assignee; a claim by the assignee does not.
func TestAnAssignmentTellsTheAssigneeButNotTheClaimer(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ref := e.refOf(t, id)
	ctx := context.Background()

	claimed := domain.TicketAssigned{TicketRef: ref, AssigneeID: carol, ActorID: carol}
	if ns, err := e.svc.Notify(ctx, claimed); err != nil || len(ns) != 0 {
		t.Fatalf("a claim notified %v, %v; want nobody", recipients(ns), err)
	}
	assigned := domain.TicketAssigned{TicketRef: ref, AssigneeID: carol, ActorID: alice}
	ns, err := e.svc.Notify(ctx, assigned)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].UserID != carol || ns[0].Kind != usecase.KindTicketAssigned {
		t.Fatalf("assignment = %+v, want carol", ns)
	}
	if ns[0].Key == "" || ns[0].Key == claimed.Key {
		t.Errorf("key = %q", ns[0].Key)
	}
}

// TestOnlyWaitingAndResolvedTellTheRequester.
func TestOnlyWaitingAndResolvedTellTheRequester(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ChangeStatus(ctx, e.p["carol"], id, domain.StatusInProgress); err != nil {
		t.Fatal(err)
	}
	ref := e.refOf(t, id)
	for _, to := range []domain.Status{domain.StatusInProgress, domain.StatusOpen} {
		ns, err := e.svc.Notify(ctx, domain.StatusChanged{TicketRef: ref, From: "x", To: string(to), ActorID: carol})
		if err != nil {
			t.Fatal(err)
		}
		if len(ns) != 0 {
			t.Errorf("%s notified %v, want nobody", to, recipients(ns))
		}
	}
	for _, to := range []domain.Status{domain.StatusWaiting, domain.StatusResolved} {
		ns, err := e.svc.Notify(ctx, domain.StatusChanged{TicketRef: ref, From: "x", To: string(to), ActorID: carol})
		if err != nil {
			t.Fatal(err)
		}
		if len(ns) != 1 || ns[0].UserID != bob || ns[0].Kind != usecase.KindStatusChanged {
			t.Errorf("%s notified %v, want bob", to, recipients(ns))
		}
	}
}

// TestCommentsTellTheOtherSide: a public comment tells the requester and the
// assignee except its author; an internal one tells the assignee only.
func TestCommentsTellTheOtherSide(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	ref := e.refOf(t, id)

	cases := []struct {
		name     string
		author   string
		internal bool
		want     []string
	}{
		{"the requester writes in public", bob, false, []string{carol}},
		{"the agent writes in public", carol, false, []string{bob}},
		{"the agent notes internally about themselves", carol, true, nil},
	}
	for _, c := range cases {
		ns, err := e.svc.Notify(ctx, domain.CommentAdded{
			TicketRef: ref, CommentID: newID(), AuthorID: c.author, Internal: c.internal,
		})
		if err != nil {
			t.Fatal(err)
		}
		got := recipients(ns)
		delete(got, c.author)
		if len(got) != len(c.want) {
			t.Errorf("%s: notified %v, want %v", c.name, recipients(ns), c.want)
		}
		for _, uid := range c.want {
			if !got[uid] {
				t.Errorf("%s: %s was not told", c.name, uid)
			}
		}
	}
}

// TestAnInternalNoteGoesToTheAssigneeOnly: an internal note by someone else
// tells the assignee, and never the requester.
func TestAnInternalNoteGoesToTheAssigneeOnly(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.Assign(ctx, e.p["carol"], id, carol); err != nil {
		t.Fatal(err)
	}
	ns, err := e.svc.Notify(ctx, domain.CommentAdded{
		TicketRef: e.refOf(t, id), CommentID: newID(), AuthorID: alice, Internal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].UserID != carol {
		t.Fatalf("internal note notified %v, want only carol", recipients(ns))
	}
	if ns[0].Data["internal"] != true {
		t.Errorf("data = %v", ns[0].Data)
	}
}

// TestAGoneTicketTellsNobody: an event for a ticket that has been deleted is
// ignored.
func TestAGoneTicketTellsNobody(t *testing.T) {
	e := newEnv(t)
	ns, err := e.svc.Notify(context.Background(), domain.TicketCreated{
		TicketRef: domain.TicketRef{WorkspaceID: ws, TicketID: newID()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 0 {
		t.Fatalf("notified %v for a missing ticket", recipients(ns))
	}
}
