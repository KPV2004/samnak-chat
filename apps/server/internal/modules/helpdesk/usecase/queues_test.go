package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

func (e *env) createQueue(t *testing.T, who, name, prefix string) domain.Queue {
	t.Helper()
	q, err := e.svc.CreateQueue(context.Background(), e.p[who], domain.QueueInput{Name: name, Prefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestOnlyAdminsManageQueues(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreateQueue(context.Background(), e.p["carol"], domain.QueueInput{Name: "IT", Prefix: "IT"})
	if !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("carol: err = %v, want ErrForbidden", err)
	}
	q := e.createQueue(t, "alice", "IT", "IT")
	if q.Prefix != "IT" || q.WorkspaceID != ws || q.ID == "" {
		t.Errorf("queue = %+v", q)
	}
	name := "Renamed"
	if _, err := e.svc.UpdateQueue(context.Background(), e.p["carol"], q.ID, domain.QueuePatch{Name: &name}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("carol renaming: err = %v, want ErrForbidden", err)
	}
	if err := e.svc.SetAgents(context.Background(), e.p["carol"], q.ID, []string{carol}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("carol setting agents: err = %v, want ErrForbidden", err)
	}
}

func TestPrefixesAreUniquePerWorkspaceIgnoringCase(t *testing.T) {
	e := newEnv(t)
	e.createQueue(t, "alice", "IT", "IT")
	_, err := e.svc.CreateQueue(context.Background(), e.p["alice"], domain.QueueInput{Name: "Other", Prefix: "it"})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	_, err = e.svc.CreateQueue(context.Background(), e.p["alice"], domain.QueueInput{Name: "Bad", Prefix: "I1"})
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestListShowsActiveQueuesWithAgentsAndCategories(t *testing.T) {
	e := newEnv(t)
	it := e.createQueue(t, "alice", "IT", "IT")
	e.createQueue(t, "alice", "HR", "HR")
	ctx := context.Background()
	if err := e.svc.SetAgents(ctx, e.p["alice"], it.ID, []string{carol}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: it.ID, Name: "Laptop", IsDefault: true}); err != nil {
		t.Fatal(err)
	}

	list, err := e.svc.ListQueues(ctx, e.p["carol"])
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("queues = %d, want 2", len(list))
	}
	var gotIT usecaseSummary
	for _, s := range list {
		if s.Queue.Prefix == "IT" {
			gotIT = usecaseSummary{agents: s.Queue.AgentIDs, categories: len(s.Categories)}
		}
	}
	if len(gotIT.agents) != 1 || gotIT.agents[0] != carol || gotIT.categories != 1 {
		t.Errorf("IT = %+v, want carol as agent and one category", gotIT)
	}

	archived := true
	if _, err := e.svc.UpdateQueue(ctx, e.p["alice"], it.ID, domain.QueuePatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	list, err = e.svc.ListQueues(ctx, e.p["carol"])
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Queue.Prefix != "HR" {
		t.Errorf("after archiving IT the list = %+v, want HR only", list)
	}
}

type usecaseSummary struct {
	agents     []string
	categories int
}

func TestRenameAndClearTheChannel(t *testing.T) {
	e := newEnv(t)
	channel := newID()
	q, err := e.svc.CreateQueue(context.Background(), e.p["alice"], domain.QueueInput{Name: "IT", Prefix: "IT", ChannelID: channel})
	if err != nil {
		t.Fatal(err)
	}
	if q.ChannelID != channel {
		t.Fatalf("channel = %q", q.ChannelID)
	}
	name := "Service desk"
	got, err := e.svc.UpdateQueue(context.Background(), e.p["alice"], q.ID, domain.QueuePatch{Name: &name, ChannelSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.ChannelID != "" {
		t.Errorf("after update: %+v", got)
	}
}

func TestSetAgentsNeedsActiveMembersAndReplacesTheList(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")

	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, []string{carol, outside}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("outsider as agent: err = %v, want ErrInvalid", err)
	}
	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, []string{"not-a-uuid"}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("bad ID: err = %v, want ErrInvalid", err)
	}
	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, []string{carol, carol, bob}); err != nil {
		t.Fatal(err)
	}
	stored, _ := e.repo.Queue(ctx, ws, q.ID)
	if len(stored.AgentIDs) != 2 {
		t.Errorf("agents = %v, want carol and bob once each", stored.AgentIDs)
	}
	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, nil); err != nil {
		t.Fatal(err)
	}
	stored, _ = e.repo.Queue(ctx, ws, q.ID)
	if len(stored.AgentIDs) != 0 {
		t.Errorf("agents = %v after an empty list", stored.AgentIDs)
	}
}

func TestQueuesOfAnotherWorkspaceAreNotFound(t *testing.T) {
	e := newEnv(t)
	q := e.createQueue(t, "alice", "IT", "IT")
	other := e.p["alice"]
	other.WorkspaceID = ws2
	name := "X"
	if _, err := e.svc.UpdateQueue(context.Background(), other, q.ID, domain.QueuePatch{Name: &name}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := e.svc.SetAgents(context.Background(), other, q.ID, nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("SetAgents err = %v, want ErrNotFound", err)
	}
}
