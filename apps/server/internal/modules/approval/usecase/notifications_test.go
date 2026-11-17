package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

func notify(t *testing.T, r *rig, evt app.Event) []app.Notification {
	t.Helper()
	out, err := r.svc.Notify(context.Background(), evt)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNotifyCreatedGoesToTheApprover(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	out := notify(t, r, domain.RequestCreated{WorkspaceID: ws, RequestID: req.ID, Type: "leave", RequesterID: alice, ApproverID: bob, ActorID: alice})
	if len(out) != 1 {
		t.Fatalf("notifications = %+v", out)
	}
	n := out[0]
	if n.UserID != bob || n.WorkspaceID != ws || n.Kind != "approval.requested" || n.Key != "approval:"+req.ID+":created" {
		t.Errorf("notification = %+v", n)
	}
	if n.Link != "/approvals/"+req.ID || n.Title == "" || n.Body == "" {
		t.Errorf("notification = %+v", n)
	}
	if n.Data["request_id"] != req.ID || n.Data["type"] != "leave" {
		t.Errorf("data = %v", n.Data)
	}
}

func TestNotifyDecisionsGoToTheRequesterAndCancelToTheApprover(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	base := func() (string, string, string, string, string, string) { return ws, req.ID, "leave", alice, bob, bob }
	w, id, typ, requester, approver, actor := base()

	approved := notify(t, r, domain.RequestApproved{WorkspaceID: w, RequestID: id, Type: typ, RequesterID: requester, ApproverID: approver, ActorID: actor})
	rejected := notify(t, r, domain.RequestRejected{WorkspaceID: w, RequestID: id, Type: typ, RequesterID: requester, ApproverID: approver, ActorID: actor, Reason: "no cover"})
	cancelled := notify(t, r, domain.RequestCancelled{WorkspaceID: w, RequestID: id, Type: typ, RequesterID: requester, ApproverID: approver, ActorID: alice})

	for _, c := range []struct {
		name string
		got  []app.Notification
		user string
		kind string
		key  string
	}{
		{"approved", approved, alice, "approval.approved", "approval:" + id + ":approved"},
		{"rejected", rejected, alice, "approval.rejected", "approval:" + id + ":rejected"},
		{"cancelled", cancelled, bob, "approval.cancelled", "approval:" + id + ":cancelled"},
	} {
		if len(c.got) != 1 || c.got[0].UserID != c.user || c.got[0].Kind != c.kind || c.got[0].Key != c.key || c.got[0].WorkspaceID != ws {
			t.Errorf("%s: %+v", c.name, c.got)
		}
	}
	if got := rejected[0].Body; got == "" || !strings.Contains(got, "no cover") {
		t.Errorf("a rejection notice must carry the reason, body = %q", got)
	}
}

func TestNotifyUsesTheEventNotTheCurrentState(t *testing.T) {
	// The rule runs after the commit, perhaps late; what it says is decided
	// by the event, so a rejected-then-... request never produces the wrong
	// notice. With no request row there is no summary, but the notice still goes out.
	r := newRig(t)
	out := notify(t, r, domain.RequestApproved{WorkspaceID: ws, RequestID: "0193f0f0-0000-7000-8000-0000000000ff", Type: "leave", RequesterID: alice, ApproverID: bob, ActorID: bob})
	if len(out) != 1 || out[0].UserID != alice {
		t.Fatalf("notifications = %+v", out)
	}
}

func TestNotifyIgnoresOtherEvents(t *testing.T) {
	r := newRig(t)
	if out, err := r.svc.Notify(context.Background(), nil); err != nil || out != nil {
		t.Errorf("nil event = %v, %v", out, err)
	}
}
