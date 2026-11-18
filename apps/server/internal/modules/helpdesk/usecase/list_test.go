package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// listSetup makes an IT queue with carol as its agent and a Laptop category,
// and returns the category.
func (e *env) listSetup(t *testing.T) (domain.Queue, domain.Category) {
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
	return q, c
}

func keysOf(page usecase.Page) []string {
	var out []string
	for _, t := range page.Items {
		out = append(out, t.Key)
	}
	return out
}

func TestRequestersSeeTheirOwnAgentsTheQueueAndAdminsEverything(t *testing.T) {
	e := newEnv(t)
	_, c := e.listSetup(t)
	ctx := context.Background()
	if _, err := e.create(t, "alice", usecase.CreateInput{CategoryID: c.ID, Title: "alice's"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "bob's"}); err != nil {
		t.Fatal(err)
	}

	// alice and admin are admins; carol is the agent of IT; bob is only a
	// requester, so he sees his own ticket and nothing else.
	want := map[string]int{"alice": 2, "bob": 1, "carol": 2, "admin": 2}
	for who, n := range want {
		page, err := e.svc.List(ctx, e.p[who], usecase.ListQuery{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != n {
			t.Errorf("%s sees %v, want %d tickets", who, keysOf(page), n)
		}
	}
	outsider, err := e.svc.List(ctx, e.p["outside"], usecase.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outsider.Items) != 0 {
		t.Errorf("outsider sees %v", keysOf(outsider))
	}
}

func TestListFiltersByQueueStatusAssigneeAndRequester(t *testing.T) {
	e := newEnv(t)
	it, c := e.listSetup(t)
	ctx := context.Background()
	hr := e.createQueue(t, "alice", "HR", "HR")
	leave, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: hr.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "alice", usecase.CreateInput{CategoryID: c.ID, Title: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "alice", usecase.CreateInput{CategoryID: leave.ID, Title: "leave"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "two"}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		query usecase.ListQuery
		want  []string
	}{
		{"by queue", usecase.ListQuery{QueueID: it.ID}, []string{"IT-0002", "IT-0001"}},
		{"by status", usecase.ListQuery{Status: "new"}, []string{"IT-0002", "IT-0001"}},
		{"unassigned", usecase.ListQuery{Assignee: "none"}, []string{"IT-0002", "IT-0001"}},
		{"requested by me", usecase.ListQuery{Requester: "me"}, []string{"HR-0001", "IT-0001"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			who := "carol"
			if c.query.Requester == "me" {
				who = "alice"
			}
			page, err := e.svc.List(ctx, e.p[who], c.query)
			if err != nil {
				t.Fatal(err)
			}
			if got := keysOf(page); !sameSet(got, c.want) {
				t.Errorf("keys = %v, want %v", got, c.want)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestListPagesThroughEverySubmitterOnceWithAnEmptyLastCursor(t *testing.T) {
	e := newEnv(t)
	_, c := e.listSetup(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := e.create(t, "alice", usecase.CreateInput{CategoryID: c.ID, Title: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	cursor := ""
	pages := 0
	for {
		page, err := e.svc.List(ctx, e.p["alice"], usecase.ListQuery{Cursor: cursor, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		got = append(got, keysOf(page)...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatal("paging does not end")
		}
	}
	if pages != 3 || len(got) != 5 || !sameSet(got, []string{"IT-0001", "IT-0002", "IT-0003", "IT-0004", "IT-0005"}) {
		t.Errorf("pages = %d, keys = %v; want 3 pages of 5 distinct tickets", pages, got)
	}
}

func TestListRefusesBadFilters(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, q := range map[string]usecase.ListQuery{
		"status":    {Status: "bogus"},
		"assignee":  {Assignee: "someone"},
		"requester": {Requester: "carol"},
		"queue":     {QueueID: "x"},
		"cursor":    {Cursor: "x"},
		"limit":     {Limit: -1},
	} {
		if _, err := e.svc.List(ctx, e.p["alice"], q); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDetailIsOnlyForThoseWhoMaySeeTheTicket(t *testing.T) {
	e := newEnv(t)
	_, c := e.listSetup(t)
	ctx := context.Background()
	tk, err := e.create(t, "alice", usecase.CreateInput{CategoryID: c.ID, Title: "Printer"})
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"alice", "carol", "admin"} {
		d, err := e.svc.Get(ctx, e.p[who], tk.ID)
		if err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		if d.Ticket.Key != "IT-0001" || len(d.History) != 1 || d.History[0].Action != domain.ActionCreated {
			t.Errorf("%s: detail = %+v", who, d)
		}
	}
	if _, err := e.svc.Get(ctx, e.p["bob"], tk.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("bob: err = %v, want ErrNotFound", err)
	}
	other := e.p["alice"]
	other.WorkspaceID = ws2
	if _, err := e.svc.Get(ctx, other, tk.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("another workspace: err = %v, want ErrNotFound", err)
	}
}
