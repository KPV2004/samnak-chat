package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

func TestGetShowsRequesterAndApproverOnly(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	if _, err := r.svc.Reject(context.Background(), p(bob), req.ID, "no"); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{alice, bob} {
		d, err := r.svc.Get(context.Background(), p(who), req.ID)
		if err != nil {
			t.Fatalf("Get by %s: %v", who, err)
		}
		if d.Request.ID != req.ID || len(d.History) != 2 || d.History[0].Action != domain.ActionCreated {
			t.Errorf("detail for %s = %+v", who, d)
		}
	}
	for _, who := range []string{carol, manager} {
		if _, err := r.svc.Get(context.Background(), p(who), req.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("Get by %s = %v, want ErrNotFound", who, err)
		}
	}
	if _, err := r.svc.Get(context.Background(), app.Principal{UserID: alice, WorkspaceID: otherWS}, req.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("other workspace = %v, want ErrNotFound", err)
	}
}

func TestGetListsAttachments(t *testing.T) {
	r := newRig(t)
	a := r.uploaded(t, alice, 100)
	in := submitIn(bob)
	in.AttachmentIDs = []string{a}
	req, err := r.svc.Submit(context.Background(), p(alice), in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.svc.Get(context.Background(), p(bob), req.Request.ID)
	if err != nil || len(d.Attachments) != 1 || d.Attachments[0].ID != a || d.Attachments[0].FileName != "cert.pdf" {
		t.Fatalf("detail = %+v, %v", d, err)
	}
}

func TestListByRoleAndStatus(t *testing.T) {
	r := newRig(t)
	first := r.submit(t).Request
	r.advance(time.Minute)
	second := r.submit(t).Request
	r.advance(time.Minute)
	toAlice, err := r.svc.Submit(context.Background(), p(bob), usecase.SubmitInput{Type: "leave", Payload: []byte(leaveJSON), ApproverID: alice})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(context.Background(), p(bob), first.ID); err != nil {
		t.Fatal(err)
	}

	list := func(who, role, status string) []string {
		t.Helper()
		page, err := r.svc.List(context.Background(), p(who), usecase.ListQuery{Role: role, Status: status})
		if err != nil {
			t.Fatalf("List(%s,%s,%s): %v", who, role, status, err)
		}
		var ids []string
		for _, it := range page.Items {
			ids = append(ids, it.ID)
		}
		return ids
	}
	eq := func(got []string, want ...string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	eq(list(alice, "requester", ""), second.ID, first.ID)
	eq(list(bob, "approver", ""), second.ID, first.ID)
	eq(list(alice, "approver", ""), toAlice.Request.ID)
	eq(list(alice, "", ""), toAlice.Request.ID, second.ID, first.ID)
	eq(list(alice, "requester", "pending"), second.ID)
	eq(list(alice, "requester", "approved,pending"), second.ID, first.ID)
	eq(list(carol, "", ""))
}

func TestListRejectsBadQueries(t *testing.T) {
	r := newRig(t)
	for name, q := range map[string]usecase.ListQuery{
		"role":                    {Role: "owner"},
		"status":                  {Status: "done"},
		"one of several statuses": {Status: "pending,done"},
		"cursor":                  {Cursor: "0193f0f0-0000-7000-8000-0000000000ff"},
		"bad cursor":              {Cursor: "x"},
		"limit":                   {Limit: -1},
	} {
		if _, err := r.svc.List(context.Background(), p(alice), q); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}

func TestListPages(t *testing.T) {
	r := newRig(t)
	var ids []string
	for range 5 {
		ids = append([]string{r.submit(t).Request.ID}, ids...) // newest first
		r.advance(time.Second)
	}
	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		page, err := r.svc.List(context.Background(), p(alice), usecase.ListQuery{Role: "requester", Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range page.Items {
			got = append(got, it.ID)
		}
		if page.NextCursor == "" {
			if len(page.Items) == 0 {
				t.Error("an empty last page")
			}
			break
		}
		if len(page.Items) != 2 || pages > 5 {
			t.Fatalf("page %d = %d items, cursor %q", pages, len(page.Items), page.NextCursor)
		}
		cursor = page.NextCursor
	}
	if fmt.Sprint(got) != fmt.Sprint(ids) {
		t.Errorf("paged = %v, want %v", got, ids)
	}
}

func TestListDefaultLimitAndCap(t *testing.T) {
	r := newRig(t)
	for range 25 {
		r.submit(t)
		r.advance(time.Millisecond)
	}
	page, err := r.svc.List(context.Background(), p(alice), usecase.ListQuery{})
	if err != nil || len(page.Items) != usecase.DefaultLimit || page.NextCursor == "" {
		t.Fatalf("default page = %d items, cursor %q, %v", len(page.Items), page.NextCursor, err)
	}
	page, err = r.svc.List(context.Background(), p(alice), usecase.ListQuery{Limit: 1000})
	if err != nil || len(page.Items) != 25 || page.NextCursor != "" {
		t.Fatalf("capped page = %d items, cursor %q, %v", len(page.Items), page.NextCursor, err)
	}
}
