package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// setup makes an IT queue with a Laptop category and returns the category.
func (e *env) setup(t *testing.T) (domain.Queue, domain.Category) {
	t.Helper()
	q := e.createQueue(t, "alice", "IT", "IT")
	c, err := e.svc.CreateCategory(context.Background(), e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	return q, c
}

func (e *env) create(t *testing.T, who string, in usecase.CreateInput) (domain.Ticket, error) {
	t.Helper()
	return e.svc.Create(context.Background(), e.p[who], in)
}

func TestCreateNumbersTicketsPerQueue(t *testing.T) {
	e := newEnv(t)
	q, c := e.setup(t)
	hr := e.createQueue(t, "alice", "HR", "HR")
	hrCat, err := e.svc.CreateCategory(context.Background(), e.p["alice"], domain.CategoryInput{QueueID: hr.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}

	var keys []string
	for i := 0; i < 2; i++ {
		tk, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: fmt.Sprintf("Laptop %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tk.Key)
	}
	leave, err := e.create(t, "carol", usecase.CreateInput{CategoryID: hrCat.ID, Title: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	if keys[0] != "IT-0001" || keys[1] != "IT-0002" || leave.Key != "HR-0001" {
		t.Errorf("keys = %v and %s, want IT-0001, IT-0002 and HR-0001", keys, leave.Key)
	}
	if got := e.repo.ticketsOf(q.ID); len(got) != 2 {
		t.Errorf("IT tickets = %d, want 2", len(got))
	}
}

func TestCreateStoresTheTicketHistoryAndEvent(t *testing.T) {
	e := newEnv(t)
	q, c := e.setup(t)
	tk, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "  Printer jams ", Description: "Tray 2", Priority: domain.PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != domain.StatusNew || tk.AssigneeID != "" || tk.RequesterID != carol || tk.QueueID != q.ID {
		t.Errorf("ticket = %+v", tk)
	}
	if tk.Title != "Printer jams" || tk.Description != "Tray 2" || tk.Priority != domain.PriorityHigh || tk.Number != 1 {
		t.Errorf("ticket fields = %+v", tk)
	}

	if len(e.repo.history) != 1 || e.repo.history[0].Action != domain.ActionCreated || e.repo.history[0].ActorID != carol {
		t.Fatalf("history = %+v, want one created line by carol", e.repo.history)
	}
	events := e.bus.all()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	created, ok := events[0].(domain.TicketCreated)
	if !ok || created.TicketID != tk.ID || created.Key != "IT-0001" || created.RequesterID != carol || created.QueueID != q.ID || created.WorkspaceID != ws {
		t.Errorf("event = %#v", events[0])
	}
}

func TestCreateNeedsAUsableCategory(t *testing.T) {
	e := newEnv(t)
	q, c := e.setup(t)
	ctx := context.Background()

	for name, id := range map[string]string{"not an ID": "nope", "unknown": newID()} {
		if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: id, Title: "x"}); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("category %s: err = %v, want ErrInvalid", name, err)
		}
	}

	archived := true
	if _, err := e.svc.UpdateCategory(ctx, e.p["alice"], c.ID, domain.CategoryPatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("archived category: err = %v, want ErrInvalid", err)
	}

	other, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Email"})
	if err != nil {
		t.Fatal(err)
	}
	gone := true
	if _, err := e.svc.UpdateQueue(ctx, e.p["alice"], q.ID, domain.QueuePatch{Archived: &gone}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: other.ID, Title: "x"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("category of an archived queue: err = %v, want ErrInvalid", err)
	}
	if len(e.bus.all()) != 0 {
		t.Errorf("a refused create published %d events", len(e.bus.all()))
	}
}

func TestCreateRefusesABadTitleOrPriority(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "  "}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("blank title: err = %v, want ErrInvalid", err)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", Priority: "critical"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("unknown priority: err = %v, want ErrInvalid", err)
	}
}

func TestCreateFromAMessageReadsItAsTheCaller(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	channel, msg := newID(), newID()
	e.msgs[msg] = app.MessageSnapshot{ID: msg, ChannelID: channel, SenderID: carol, Kind: "text", Body: "The VPN drops every hour"}

	tk, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "VPN", SourceMessageID: msg})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Description != "The VPN drops every hour" || tk.SourceChannelID != channel || tk.SourceMessageID != msg {
		t.Errorf("ticket from message = %+v", tk)
	}

	given, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "VPN", Description: "Mine", SourceMessageID: msg})
	if err != nil {
		t.Fatal(err)
	}
	if given.Description != "Mine" {
		t.Errorf("description = %q, want the one given", given.Description)
	}
}

func TestCreateFromAMessageNeedsChatAndAReadableMessage(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	msg := newID()

	noChat := usecase.New(usecase.Deps{
		Tx: fakeTx{}, Repo: e.repo, Members: fakeMembers{ws + "/" + carol: true},
		Permissions: fakePerms{}, Now: func() time.Time { return t0 },
	})
	if _, err := noChat.Create(context.Background(), e.p["carol"], usecase.CreateInput{CategoryID: c.ID, Title: "x", SourceMessageID: msg}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("without chat: err = %v, want ErrInvalid", err)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", SourceMessageID: msg}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unreadable message: err = %v, want ErrNotFound", err)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", SourceMessageID: "nope"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("malformed message ID: err = %v, want ErrInvalid", err)
	}
}

func TestCreateFailsWhenTheEventCannotBeWritten(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	e.bus.err = errors.New("outbox is down")
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x"}); err == nil {
		t.Fatal("create succeeded although the event was not written")
	}
}
