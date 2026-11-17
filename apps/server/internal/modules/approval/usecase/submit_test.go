package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

func submitIn(approver string) usecase.SubmitInput {
	return usecase.SubmitInput{Type: "leave", Payload: []byte(leaveJSON), ApproverID: approver}
}

func TestSubmitStoresARequestWithHistoryAndAnEvent(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), p(alice), submitIn(bob))
	if err != nil {
		t.Fatal(err)
	}
	req := d.Request
	if req.Status != domain.StatusPending || req.Type != domain.TypeLeave || req.RequesterID != alice || req.ApproverID != bob || req.WorkspaceID != ws {
		t.Errorf("request = %+v", req)
	}
	if len(req.ID) != 36 || req.ID[14] != '7' {
		t.Errorf("ID %q is not a UUIDv7", req.ID)
	}
	if !req.CreatedAt.Equal(t0.Truncate(time.Microsecond)) || !req.UpdatedAt.Equal(req.CreatedAt) {
		t.Errorf("times = %v / %v, want %v at microsecond precision", req.CreatedAt, req.UpdatedAt, t0.Truncate(time.Microsecond))
	}
	if len(d.History) != 1 || d.History[0].Action != domain.ActionCreated || d.History[0].ActorID != alice {
		t.Errorf("history = %+v", d.History)
	}
	if r.repo.steps[req.ID].ApproverID != bob || r.repo.steps[req.ID].StepNo != 1 {
		t.Errorf("step = %+v", r.repo.steps[req.ID])
	}
	evt, ok := r.bus.last().(domain.RequestCreated)
	if !ok {
		t.Fatalf("last event = %#v", r.bus.last())
	}
	want := domain.RequestCreated{WorkspaceID: ws, RequestID: req.ID, Type: "leave", RequesterID: alice, ApproverID: bob, ActorID: alice}
	if evt != want {
		t.Errorf("event = %+v, want %+v", evt, want)
	}
}

func TestSubmitDefaultsToTheFirstGroupManager(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), p(alice), submitIn(""))
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.ApproverID != manager {
		t.Errorf("approver = %s, want the first manager", d.Request.ApproverID)
	}
}

func TestSubmitNamedApproverWinsOverTheManager(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), p(alice), submitIn(carol))
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.ApproverID != carol || r.managers.calls != 0 {
		t.Errorf("approver = %s, manager lookups = %d", d.Request.ApproverID, r.managers.calls)
	}
}

func TestSubmitSkipsAManagerWhoIsNoLongerActive(t *testing.T) {
	r := newRig(t)
	r.managers.byUser[alice] = []string{outsider, manager2}
	d, err := r.svc.Submit(context.Background(), p(alice), submitIn(""))
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.ApproverID != manager2 {
		t.Errorf("approver = %s, want the first active manager", d.Request.ApproverID)
	}
}

func TestSubmitWithoutAnApproverIsApproverRequired(t *testing.T) {
	cases := map[string]func(r *rig){
		"no managers":          func(r *rig) { r.managers.byUser = nil },
		"no group service":     func(r *rig) { r.svc = newServiceWithoutManagers(r) },
		"only inactive people": func(r *rig) { r.managers.byUser[alice] = []string{outsider} },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			setup(r)
			_, err := r.svc.Submit(context.Background(), p(alice), submitIn(""))
			if !errors.Is(err, domain.ErrApproverRequired) || !errors.Is(err, app.ErrInvalid) {
				t.Fatalf("error = %v, want ErrApproverRequired", err)
			}
			if len(r.repo.requests) != 0 || len(r.bus.names()) != 0 {
				t.Error("a refused request left a row or an event")
			}
		})
	}
}

func TestSubmitManagerLookupFailureIsNotApproverRequired(t *testing.T) {
	r := newRig(t)
	boom := errors.New("db down")
	r.managers.err = boom
	_, err := r.svc.Submit(context.Background(), p(alice), submitIn(""))
	if !errors.Is(err, boom) || errors.Is(err, app.ErrInvalid) {
		t.Fatalf("error = %v, want the storage error unchanged", err)
	}
}

func TestSubmitRejectsBadApprovers(t *testing.T) {
	for name, approver := range map[string]string{
		"the requester": alice, "a non-member": outsider, "not an ID": "bob",
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			_, err := r.svc.Submit(context.Background(), p(alice), submitIn(approver))
			if !errors.Is(err, app.ErrInvalid) || errors.Is(err, domain.ErrApproverRequired) {
				t.Fatalf("error = %v, want a plain ErrInvalid", err)
			}
		})
	}
}

func TestSubmitRejectsBadInput(t *testing.T) {
	r := newRig(t)
	for name, in := range map[string]usecase.SubmitInput{
		"unknown type":              {Type: "expense", Payload: []byte(`{}`), ApproverID: bob},
		"invalid payload":           {Type: "leave", Payload: []byte(`{"leave_type":"annual"}`), ApproverID: bob},
		"payload of the other type": {Type: "general", Payload: []byte(leaveJSON), ApproverID: bob},
	} {
		if _, err := r.svc.Submit(context.Background(), p(alice), in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
	if len(r.repo.requests) != 0 {
		t.Error("a refused request was stored")
	}
}

func TestSubmitStoresTheNormalisedPayload(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), p(alice), usecase.SubmitInput{
		Type: "general", Payload: []byte(`{"subject":" Laptop ","details":"","amount":"12.5"}`), ApproverID: bob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(d.Request.Payload); !strings.Contains(got, `"amount":"12.50"`) || !strings.Contains(got, `"currency":"THB"`) || !strings.Contains(got, `"subject":"Laptop"`) {
		t.Errorf("payload = %s", got)
	}
}

func TestSubmitUsesTheWorkspaceOfThePrincipalOnly(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), app.Principal{UserID: alice, WorkspaceID: otherWS}, submitIn(carol))
	// carol is not a member of otherWS, so nothing can be sent there.
	if err == nil {
		t.Fatalf("submitted into %s with approver %s", d.Request.WorkspaceID, carol)
	}
}

func TestSubmitRollsBackWhenTheEventCannotBeWritten(t *testing.T) {
	r := newRig(t)
	boom := errors.New("outbox write failed")
	r.bus.publishFn = func(app.Event) error { return boom }
	if _, err := r.svc.Submit(context.Background(), p(alice), submitIn(bob)); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if len(r.repo.requests) != 0 || len(r.repo.history) != 0 {
		t.Errorf("request survived: %d rows, %d history lines", len(r.repo.requests), len(r.repo.history))
	}
	if len(r.bus.names()) != 0 {
		t.Errorf("events = %v, want none", r.bus.names())
	}
}

// uploaded makes a file of alice's that is in object storage.
func (r *rig) uploaded(t *testing.T, owner string, size int64) string {
	t.Helper()
	up, err := r.svc.RequestUpload(context.Background(), p(owner), usecase.UploadInput{FileName: "cert.pdf", Size: size, MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	r.store.stored[domain.ObjectKey(ws, up.AttachmentID)] = size
	return up.AttachmentID
}

func TestSubmitLinksAttachments(t *testing.T) {
	r := newRig(t)
	a, b := r.uploaded(t, alice, 100), r.uploaded(t, alice, 200)
	in := submitIn(bob)
	in.AttachmentIDs = []string{a, b}
	d, err := r.svc.Submit(context.Background(), p(alice), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Attachments) != 2 || d.Attachments[0].RequestID != d.Request.ID {
		t.Errorf("attachments = %+v", d.Attachments)
	}
	// A file belongs to one request.
	in2 := submitIn(bob)
	in2.AttachmentIDs = []string{a}
	if _, err := r.svc.Submit(context.Background(), p(alice), in2); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("reusing a file: %v, want ErrInvalid", err)
	}
}

func TestSubmitRefusesBadAttachments(t *testing.T) {
	r := newRig(t)
	mine := r.uploaded(t, alice, 100)
	theirs := r.uploaded(t, bob, 100)
	notSent, err := r.svc.RequestUpload(context.Background(), p(alice), usecase.UploadInput{FileName: "a.pdf", Size: 10, MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	wrongSize, err := r.svc.RequestUpload(context.Background(), p(alice), usecase.UploadInput{FileName: "b.pdf", Size: 10, MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	r.store.stored[domain.ObjectKey(ws, wrongSize.AttachmentID)] = 11

	tooMany := make([]string, domain.MaxAttachments+1)
	for i := range tooMany {
		tooMany[i] = r.uploaded(t, alice, 1)
	}
	cases := []struct {
		name string
		ids  []string
		is   error
	}{
		{"someone else's file", []string{theirs}, app.ErrNotFound},
		{"unknown file", []string{"0193f0f0-0000-7000-8000-0000000000ff"}, app.ErrNotFound},
		{"not an ID", []string{"x"}, app.ErrInvalid},
		{"the same file twice", []string{mine, mine}, app.ErrInvalid},
		{"bytes never uploaded", []string{notSent.AttachmentID}, app.ErrInvalid},
		{"bytes of another size", []string{wrongSize.AttachmentID}, app.ErrInvalid},
		{"too many", tooMany, app.ErrInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := submitIn(bob)
			in.AttachmentIDs = c.ids
			if _, err := r.svc.Submit(context.Background(), p(alice), in); !errors.Is(err, c.is) {
				t.Fatalf("error = %v, want %v", err, c.is)
			}
			if len(r.repo.requests) != 0 {
				t.Error("a refused request was stored")
			}
		})
	}
}

func TestSubmitWithoutStorageRefusesAttachments(t *testing.T) {
	r := newRig(t)
	r.svc = newServiceWithoutStorage(r)
	in := submitIn(bob)
	in.AttachmentIDs = []string{"0193f0f0-0000-7000-8000-0000000000ff"}
	if _, err := r.svc.Submit(context.Background(), p(alice), in); err == nil {
		t.Fatal("expected an error")
	}
	// Without files nothing needs storage.
	if _, err := r.svc.Submit(context.Background(), p(alice), submitIn(bob)); err != nil {
		t.Fatal(err)
	}
}
