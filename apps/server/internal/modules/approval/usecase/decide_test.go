package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

func TestApprove(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	r.advance(time.Hour)

	d, err := r.svc.Approve(context.Background(), p(bob), req.ID)
	if err != nil {
		t.Fatal(err)
	}
	decidedAt := t0.Add(time.Hour).Truncate(time.Microsecond)
	if d.Request.Status != domain.StatusApproved || d.Request.DecidedAt == nil || !d.Request.DecidedAt.Equal(decidedAt) || !d.Request.UpdatedAt.Equal(decidedAt) {
		t.Errorf("request = %+v", d.Request)
	}
	if len(d.History) != 2 || d.History[1].Action != domain.ActionApproved || d.History[1].ActorID != bob || !d.History[1].At.Equal(decidedAt) {
		t.Errorf("history = %+v", d.History)
	}
	if r.repo.steps[req.ID].Status != domain.StatusApproved {
		t.Errorf("step = %+v", r.repo.steps[req.ID])
	}
	evt, ok := r.bus.last().(domain.RequestApproved)
	if !ok || evt.RequestID != req.ID || evt.ActorID != bob || evt.RequesterID != alice || evt.ApproverID != bob || evt.WorkspaceID != ws {
		t.Errorf("last event = %#v", r.bus.last())
	}
}

func TestRejectNeedsAReasonAndKeepsIt(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request

	for _, reason := range []string{"", "   ", strings.Repeat("x", 1001)} {
		if _, err := r.svc.Reject(context.Background(), p(bob), req.ID, reason); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("Reject(%d chars) = %v, want ErrInvalid", len(reason), err)
		}
	}
	if got := r.repo.request(t, req.ID); got.Status != domain.StatusPending {
		t.Fatalf("a refused reject changed the request: %+v", got)
	}

	d, err := r.svc.Reject(context.Background(), p(bob), req.ID, "  not enough cover ")
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.Status != domain.StatusRejected || d.Request.Reason != "not enough cover" {
		t.Errorf("request = %+v", d.Request)
	}
	if h := d.History[len(d.History)-1]; h.Action != domain.ActionRejected || h.Reason != "not enough cover" || h.ActorID != bob {
		t.Errorf("history = %+v", d.History)
	}
	evt, ok := r.bus.last().(domain.RequestRejected)
	if !ok || evt.Reason != "not enough cover" || evt.ActorID != bob {
		t.Errorf("last event = %#v", r.bus.last())
	}
}

func TestOnlyTheApproverDecides(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	for _, who := range []string{alice, carol, manager} {
		if _, err := r.svc.Approve(context.Background(), p(who), req.ID); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("Approve by %s = %v, want ErrForbidden", who, err)
		}
		if _, err := r.svc.Reject(context.Background(), p(who), req.ID, "no"); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("Reject by %s = %v, want ErrForbidden", who, err)
		}
	}
	if got := r.repo.request(t, req.ID); got.Status != domain.StatusPending {
		t.Errorf("status = %s", got.Status)
	}
	if len(r.bus.names()) != 1 {
		t.Errorf("events = %v, want only created", r.bus.names())
	}
}

func TestPermissionIsCheckedBeforeTheReason(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	if _, err := r.svc.Reject(context.Background(), p(carol), req.ID, ""); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
}

func TestDecidingTwiceIsAConflict(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); err != nil {
		t.Fatal(err)
	}
	n := len(r.bus.names())
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); !errors.Is(err, app.ErrConflict) {
		t.Errorf("approve again = %v, want ErrConflict", err)
	}
	if _, err := r.svc.Reject(context.Background(), p(bob), req.ID, "changed my mind"); !errors.Is(err, app.ErrConflict) {
		t.Errorf("reject after approve = %v, want ErrConflict", err)
	}
	if _, err := r.svc.Cancel(context.Background(), p(alice), req.ID); !errors.Is(err, app.ErrConflict) {
		t.Errorf("cancel after approve = %v, want ErrConflict", err)
	}
	if got := len(r.bus.names()); got != n {
		t.Errorf("a refused decision published an event: %v", r.bus.names())
	}
	if got := len(r.repo.history); got != 2 {
		t.Errorf("history lines = %d, want created and approved only", got)
	}
}

func TestDecidingAfterAConcurrentDecisionIsAConflict(t *testing.T) {
	// The request looked pending when it was read, but another decision won
	// before this one wrote: the conditional update reports no change.
	r := newRig(t)
	req := r.submit(t).Request
	r.svc = newServiceWithRepo(r, &racingRepo{fakeRepo: r.repo})
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if len(r.bus.names()) != 1 || len(r.repo.history) != 1 {
		t.Errorf("the losing decision left events %v or history %d", r.bus.names(), len(r.repo.history))
	}
}

func TestCancel(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	for _, who := range []string{bob, carol} {
		if _, err := r.svc.Cancel(context.Background(), p(who), req.ID); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("Cancel by %s = %v, want ErrForbidden", who, err)
		}
	}
	d, err := r.svc.Cancel(context.Background(), p(alice), req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.Status != domain.StatusCancelled || d.Request.DecidedAt == nil {
		t.Errorf("request = %+v", d.Request)
	}
	if h := d.History[len(d.History)-1]; h.Action != domain.ActionCancelled || h.ActorID != alice {
		t.Errorf("history = %+v", d.History)
	}
	if evt, ok := r.bus.last().(domain.RequestCancelled); !ok || evt.ActorID != alice || evt.ApproverID != bob {
		t.Errorf("last event = %#v", r.bus.last())
	}
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); !errors.Is(err, app.ErrConflict) {
		t.Errorf("approve after cancel = %v, want ErrConflict", err)
	}
	if _, err := r.svc.Cancel(context.Background(), p(alice), req.ID); !errors.Is(err, app.ErrConflict) {
		t.Errorf("cancel twice = %v, want ErrConflict", err)
	}
}

func TestDecisionsOnOtherWorkspacesAndMissingRequestsAreNotFound(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	elsewhere := app.Principal{UserID: bob, WorkspaceID: otherWS}
	if _, err := r.svc.Approve(context.Background(), elsewhere, req.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("other workspace = %v, want ErrNotFound", err)
	}
	if _, err := r.svc.Approve(context.Background(), p(bob), "0193f0f0-0000-7000-8000-0000000000ff"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown ID = %v, want ErrNotFound", err)
	}
	if _, err := r.svc.Cancel(context.Background(), p(alice), "nope"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("bad ID = %v, want ErrNotFound", err)
	}
}

func TestDecisionRollsBackWhenTheEventCannotBeWritten(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	r.bus.publishFn = func(app.Event) error { return errors.New("outbox write failed") }
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); err == nil {
		t.Fatal("expected an error")
	}
	if got := r.repo.request(t, req.ID); got.Status != domain.StatusPending || got.DecidedAt != nil {
		t.Errorf("request = %+v, want it still pending", got)
	}
	if len(r.repo.history) != 1 {
		t.Errorf("history = %+v", r.repo.history)
	}
}
