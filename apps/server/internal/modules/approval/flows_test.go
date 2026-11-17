package approval_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval"
)

// ruleNotifications runs the rules the module registered for evt.
func (e *env) ruleNotifications(evt app.Event) []app.Notification {
	e.t.Helper()
	var out []app.Notification
	for _, rule := range e.rules.Rules()[evt.EventName()] {
		ns, err := rule(context.Background(), evt)
		if err != nil {
			e.t.Fatal(err)
		}
		out = append(out, ns...)
	}
	return out
}

func TestLeaveRequestFlowFromSubmitToApproval(t *testing.T) {
	e := newEnv(t)
	e.start()

	// Alice names no approver: her group manager is chosen (APR-02).
	req := e.submit(e.alice, leave())
	if req.Status != "pending" || req.Type != "leave" || req.RequesterID != e.alice.ID || req.ApproverID != e.boss.ID {
		t.Fatalf("request = %+v", req)
	}
	if req.Payload["leave_type"] != "annual" || req.MessageID != nil && *req.MessageID == "" {
		t.Errorf("payload = %v", req.Payload)
	}
	if len(req.History) != 1 || req.History[0].Action != "created" || req.History[0].ActorID != e.alice.ID {
		t.Errorf("history = %+v", req.History)
	}

	// The event was delivered after the commit and the card is in the DM,
	// sent as the requester (APR-03).
	if e.cards.posts != 1 {
		t.Fatalf("cards posted = %d, want 1", e.cards.posts)
	}
	stored := e.fetch(e.alice, req.ID)
	if stored.MessageID == nil {
		t.Fatal("the card was posted but the request does not know its message")
	}
	card, body := e.cards.card(t, *stored.MessageID)
	if card.Kind != "approval.request" || card.SenderID != e.alice.ID || card.WorkspaceID != e.ws || card.ChannelID == "" || card.ThreadRootID != "" {
		t.Errorf("card = %+v", card)
	}
	dm, _ := e.cards.DirectChannel(context.Background(), e.ws, []string{e.boss.ID, e.alice.ID})
	if card.ChannelID != dm {
		t.Errorf("card went to %s, want the requester-approver DM %s", card.ChannelID, dm)
	}
	if body.Status != "pending" || body.Summary != "2026-12-01 to 2026-12-03" || body.RequesterName != "Alice" || body.ApproverName != "Boss" || body.Version != req.UpdatedAt.UnixMilli() {
		t.Errorf("card payload = %+v", body)
	}
	if card.FallbackText != "Approval request: 2026-12-01 to 2026-12-03 (pending)" {
		t.Errorf("fallback = %q", card.FallbackText)
	}

	// The approver sees it in "waiting for me", the requester in "my requests".
	for _, c := range []struct {
		u    user
		role string
	}{{e.boss, "approver"}, {e.alice, "requester"}} {
		r := e.call("GET", "/approval/requests?role="+c.role+"&status=pending", c.u, nil)
		var page struct {
			Items      []requestBody `json:"items"`
			NextCursor string        `json:"next_cursor"`
		}
		r.json(t, &page)
		if r.Code != 200 || len(page.Items) != 1 || page.Items[0].ID != req.ID || page.NextCursor != "" {
			t.Errorf("%s list = %d %s", c.role, r.Code, r.Body)
		}
	}

	// A third member cannot see it or decide it (APR-04, APR-06).
	if code, _ := e.code("GET", "/approval/requests/"+req.ID, e.carol, nil); code != http.StatusNotFound {
		t.Errorf("third user GET = %d, want 404", code)
	}
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/approve", e.carol, nil); code != http.StatusForbidden {
		t.Errorf("third user approve = %d, want 403", code)
	}
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/approve", e.alice, nil); code != http.StatusForbidden {
		t.Errorf("requester approve = %d, want 403", code)
	}

	// The approver approves; the card follows.
	r := e.call("POST", "/approval/requests/"+req.ID+"/approve", e.boss, nil)
	if r.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", r.Code, r.Body)
	}
	var approved requestBody
	r.json(t, &approved)
	if approved.Status != "approved" || approved.DecidedAt == nil || len(approved.History) != 2 || approved.History[1].Action != "approved" || approved.History[1].ActorID != e.boss.ID {
		t.Errorf("approved = %+v", approved)
	}
	_, after := e.cards.card(t, *stored.MessageID)
	if after.Status != "approved" || after.DecidedAt == nil || after.Version <= body.Version {
		t.Errorf("card after approval = %+v, version before %d", after, body.Version)
	}
	if e.cards.posts != 1 || e.cards.updates != 1 {
		t.Errorf("posts = %d, updates = %d, want 1 and 1", e.cards.posts, e.cards.updates)
	}

	// Deciding again is a conflict (APR-04).
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/approve", e.boss, nil); code != http.StatusConflict {
		t.Errorf("approve twice = %d, want 409", code)
	}
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/cancel", e.alice, nil); code != http.StatusConflict {
		t.Errorf("cancel after approval = %d, want 409", code)
	}
}

func TestRejectNeedsAReasonOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	path := "/approval/requests/" + req.ID + "/reject"

	for _, body := range []any{map[string]any{"reason": ""}, map[string]any{"reason": "   "}, map[string]any{}} {
		if code, _ := e.code("POST", path, e.bob, body); code != http.StatusBadRequest {
			t.Errorf("reject with %v = %d, want 400", body, code)
		}
	}
	if got := e.fetch(e.bob, req.ID); got.Status != "pending" {
		t.Fatalf("status = %s", got.Status)
	}
	r := e.call("POST", path, e.bob, map[string]any{"reason": "no cover that week"})
	if r.Code != http.StatusOK {
		t.Fatalf("reject = %d %s", r.Code, r.Body)
	}
	got := e.fetch(e.alice, req.ID)
	if got.Status != "rejected" || got.Reason != "no cover that week" || got.History[1].Reason != "no cover that week" {
		t.Errorf("rejected = %+v", got)
	}
	_, card := e.cards.card(t, *got.MessageID)
	if card.Status != "rejected" || card.Reason != "no cover that week" {
		t.Errorf("card = %+v", card)
	}
}

func TestCancelFlow(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/cancel", e.bob, nil); code != http.StatusForbidden {
		t.Errorf("approver cancel = %d, want 403", code)
	}
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/cancel", e.carol, nil); code != http.StatusForbidden {
		t.Errorf("third user cancel = %d, want 403", code)
	}
	if r := e.call("POST", "/approval/requests/"+req.ID+"/cancel", e.alice, nil); r.Code != http.StatusOK {
		t.Fatalf("cancel = %d %s", r.Code, r.Body)
	}
	got := e.fetch(e.bob, req.ID)
	if got.Status != "cancelled" || got.History[len(got.History)-1].Action != "cancelled" {
		t.Errorf("cancelled = %+v", got)
	}
	_, card := e.cards.card(t, *got.MessageID)
	if card.Status != "cancelled" {
		t.Errorf("card = %+v", card)
	}
	if code, _ := e.code("POST", "/approval/requests/"+req.ID+"/approve", e.bob, nil); code != http.StatusConflict {
		t.Errorf("approve after cancel = %d, want 409", code)
	}
}

func TestApproverRequiredHasItsOwnCode(t *testing.T) {
	cases := map[string]*env{}
	noManager := newEnv(t)
	noManager.groups[noManager.alice.ID] = nil
	cases["a requester without a manager"] = noManager
	cases["a server without groups"] = newEnvWith(t, true, false)

	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			e.start()
			code, body := e.code("POST", "/approval/requests", e.alice, leave())
			if code != http.StatusBadRequest || body.Error.Code != "approver_required" {
				t.Fatalf("= %d %+v, want 400 approver_required", code, body)
			}
			r := e.call("GET", "/approval/requests", e.alice, nil)
			var page struct{ Items []requestBody }
			r.json(t, &page)
			if len(page.Items) != 0 {
				t.Error("a refused request was stored")
			}
		})
	}
}

func TestSubmitValidatesTheBody(t *testing.T) {
	e := newEnv(t)
	e.start()
	bad := map[string]map[string]any{
		"unknown type": {"type": "expense", "payload": map[string]any{}},
		"empty leave":  {"type": "leave", "payload": map[string]any{}},
		"no payload":   {"type": "leave"},
		"end before start": {"type": "leave", "payload": map[string]any{
			"leave_type": "sick", "start_date": "2026-12-03", "end_date": "2026-12-01", "reason": "x"}},
		"general without subject":   {"type": "general", "payload": map[string]any{"details": "d"}},
		"negative amount":           {"type": "general", "payload": map[string]any{"subject": "s", "amount": "-5"}},
		"approver is the requester": withApprover(leave(), e.alice),
		"approver is not a member":  {"type": "leave", "approver_id": newID(), "payload": leave()["payload"]},
		"approver is not an ID":     {"type": "leave", "approver_id": "bob", "payload": leave()["payload"]},
	}
	for name, body := range bad {
		if code, eb := e.code("POST", "/approval/requests", e.alice, body); code != http.StatusBadRequest || eb.Error.Code == "approver_required" {
			t.Errorf("%s = %d %+v, want a plain 400", name, code, eb)
		}
	}
}

func TestGeneralRequestKeepsTheAmount(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, map[string]any{
		"type": "general", "approver_id": e.bob.ID,
		"payload": map[string]any{"subject": "New laptop", "details": "for the new hire", "amount": "32000"},
	})
	if req.Payload["amount"] != "32000.00" || req.Payload["currency"] != "THB" || req.Payload["subject"] != "New laptop" {
		t.Errorf("payload = %v", req.Payload)
	}
	_, card := e.cards.card(t, *e.fetch(e.alice, req.ID).MessageID)
	if card.Summary != "New laptop" || card.Fields["amount"] != "32000.00" {
		t.Errorf("card = %+v", card)
	}
}

func TestOnlyTheWorkspaceOfTheHeaderIsSeen(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))

	other := newID()
	e.members[other+"/"+e.alice.ID] = app.RoleMember
	e.members[other+"/"+e.bob.ID] = app.RoleMember
	if r := e.callIn(other, "GET", "/approval/requests/"+req.ID, e.alice, nil); r.Code != http.StatusNotFound {
		t.Errorf("GET in another workspace = %d, want 404", r.Code)
	}
	if r := e.callIn(other, "POST", "/approval/requests/"+req.ID+"/approve", e.bob, nil); r.Code != http.StatusNotFound {
		t.Errorf("approve in another workspace = %d, want 404", r.Code)
	}
	r := e.callIn(other, "GET", "/approval/requests?role=requester", e.alice, nil)
	var page struct{ Items []requestBody }
	r.json(t, &page)
	if len(page.Items) != 0 {
		t.Errorf("list in another workspace = %s", r.Body)
	}
}

func TestEveryRouteNeedsAMember(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	for _, c := range []struct{ method, path string }{
		{"GET", "/approval/requests"}, {"POST", "/approval/requests"}, {"GET", "/approval/requests/" + req.ID},
		{"POST", "/approval/requests/" + req.ID + "/approve"}, {"POST", "/approval/requests/" + req.ID + "/reject"},
		{"POST", "/approval/requests/" + req.ID + "/cancel"}, {"POST", "/approval/uploads"}, {"GET", "/approval/attachments/" + newID()},
	} {
		if r := e.call(c.method, c.path, user{}, nil); r.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", c.method, c.path, r.Code)
		}
		stranger := user{Token: "tok-stranger"}
		e.auth[stranger.Token] = app.Identity{UserID: newID(), DeviceID: "d"}
		if r := e.call(c.method, c.path, stranger, nil); r.Code != http.StatusForbidden {
			t.Errorf("%s %s by a non-member = %d, want 403", c.method, c.path, r.Code)
		}
	}
}

func TestListPagesOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.start()
	var want []string
	for range 5 {
		want = append([]string{e.submit(e.alice, withApprover(leave(), e.bob)).ID}, want...)
	}
	var got []string
	cursor := ""
	for range 10 {
		r := e.call("GET", "/approval/requests?role=requester&limit=2&cursor="+cursor, e.alice, nil)
		var page struct {
			Items      []requestBody `json:"items"`
			NextCursor string        `json:"next_cursor"`
		}
		r.json(t, &page)
		if r.Code != 200 {
			t.Fatalf("list = %d %s", r.Code, r.Body)
		}
		for _, it := range page.Items {
			got = append(got, it.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("paged = %v, want %v", got, want)
	}
	for _, q := range []string{"role=owner", "status=done", "limit=0", "limit=101", "cursor=" + newID()} {
		if code, _ := e.code("GET", "/approval/requests?"+q, e.alice, nil); code != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", q, code)
		}
	}
}

func TestUploadsAndAttachmentDownloads(t *testing.T) {
	e := newEnv(t)
	e.start()

	r := e.call("POST", "/approval/uploads", e.alice, map[string]any{"file_name": "medical cert.pdf", "size": 2048, "mime_type": "application/pdf"})
	if r.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", r.Code, r.Body)
	}
	var up struct {
		AttachmentID string `json:"attachment_id"`
		Upload       struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		} `json:"upload"`
	}
	r.json(t, &up)
	key := "approval/" + e.ws + "/" + up.AttachmentID
	if up.Upload.Method != "PUT" || !strings.HasSuffix(up.Upload.URL, key) || up.Upload.Headers["Content-Type"] != "application/pdf" {
		t.Errorf("upload = %+v", up)
	}
	if code, _ := e.code("POST", "/approval/uploads", e.alice, map[string]any{"file_name": "big.bin", "size": 60 << 20, "mime_type": "application/octet-stream"}); code != http.StatusBadRequest {
		t.Errorf("a 60 MB upload = %d, want 400", code)
	}

	// Submitting before the bytes arrived is refused; after, it works.
	body := withApprover(leave(), e.bob)
	body["attachment_ids"] = []string{up.AttachmentID}
	if code, _ := e.code("POST", "/approval/requests", e.alice, body); code != http.StatusBadRequest {
		t.Errorf("submit before upload = %d, want 400", code)
	}
	e.store.put(key, 2048)
	if code, _ := e.code("POST", "/approval/requests", e.carol, body); code != http.StatusNotFound {
		t.Errorf("submit with someone else's file = %d, want 404", code)
	}
	req := e.submit(e.alice, body)
	if len(req.Attachments) != 1 || req.Attachments[0].ID != up.AttachmentID || req.Attachments[0].FileName != "medical cert.pdf" || req.Attachments[0].Size != 2048 {
		t.Fatalf("attachments = %+v", req.Attachments)
	}
	if code, _ := e.code("POST", "/approval/requests", e.alice, body); code != http.StatusBadRequest {
		t.Errorf("reusing a file = %d, want 400", code)
	}

	for _, u := range []user{e.alice, e.bob} {
		r := e.call("GET", "/approval/attachments/"+up.AttachmentID, u, nil)
		var dl struct {
			URL       string `json:"url"`
			ExpiresAt string `json:"expires_at"`
		}
		r.json(t, &dl)
		if r.Code != 200 || !strings.HasSuffix(dl.URL, key) || dl.ExpiresAt == "" {
			t.Errorf("download by %s = %d %s", u.ID, r.Code, r.Body)
		}
	}
	if code, _ := e.code("GET", "/approval/attachments/"+up.AttachmentID, e.carol, nil); code != http.StatusNotFound {
		t.Errorf("download by a third user = %d, want 404", code)
	}
	if code, _ := e.code("GET", "/approval/attachments/"+newID(), e.alice, nil); code != http.StatusNotFound {
		t.Errorf("unknown file = %d, want 404", code)
	}
}

func TestEventsAreDeclaredAndRoundTrip(t *testing.T) {
	// The simulated outbox refuses an undeclared event or one that does not
	// survive JSON, so completing every transition proves both (PLT-10).
	e := newEnv(t)
	e.start()
	for _, name := range []string{
		"approval.request_created", "approval.request_approved", "approval.request_rejected", "approval.request_cancelled",
	} {
		if _, ok := e.bus.declared[name]; !ok {
			t.Errorf("%s is not declared", name)
		}
	}
	a := e.submit(e.alice, withApprover(leave(), e.bob))
	b := e.submit(e.alice, withApprover(leave(), e.bob))
	c := e.submit(e.alice, withApprover(leave(), e.bob))
	for path, u := range map[string]user{
		"/approval/requests/" + a.ID + "/approve": e.bob,
		"/approval/requests/" + b.ID + "/cancel":  e.alice,
	} {
		if r := e.call("POST", path, u, nil); r.Code != 200 {
			t.Errorf("%s = %d %s", path, r.Code, r.Body)
		}
	}
	if r := e.call("POST", "/approval/requests/"+c.ID+"/reject", e.bob, map[string]any{"reason": "no"}); r.Code != 200 {
		t.Errorf("reject = %d %s", r.Code, r.Body)
	}
}

func TestRedeliveredEventsDoNotPostASecondCard(t *testing.T) {
	// The relay may deliver an event twice, e.g. after a crash before it
	// marked it published. The handler must only refresh the card.
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	evt := approval.RequestCreated{WorkspaceID: e.ws, RequestID: req.ID, Type: "leave", RequesterID: e.alice.ID, ApproverID: e.bob.ID, ActorID: e.alice.ID}
	for range 3 {
		if err := e.bus.Bus.Publish(context.Background(), evt); err != nil {
			t.Fatal(err)
		}
	}
	if e.cards.posts != 1 {
		t.Errorf("cards posted = %d, want 1", e.cards.posts)
	}
	if e.cards.updates != 3 {
		t.Errorf("card updates = %d, want one per redelivery", e.cards.updates)
	}
}

func TestNotificationRules(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))

	created := e.ruleNotifications(approval.RequestCreated{WorkspaceID: e.ws, RequestID: req.ID, Type: "leave", RequesterID: e.alice.ID, ApproverID: e.bob.ID, ActorID: e.alice.ID})
	if len(created) != 1 || created[0].UserID != e.bob.ID || created[0].Kind != "approval.requested" || created[0].Key != "approval:"+req.ID+":created" || !strings.Contains(created[0].Title, "Alice") {
		t.Errorf("created = %+v", created)
	}
	approved := e.ruleNotifications(approval.RequestApproved{WorkspaceID: e.ws, RequestID: req.ID, Type: "leave", RequesterID: e.alice.ID, ApproverID: e.bob.ID, ActorID: e.bob.ID})
	if len(approved) != 1 || approved[0].UserID != e.alice.ID || approved[0].Kind != "approval.approved" {
		t.Errorf("approved = %+v", approved)
	}
	rejected := e.ruleNotifications(approval.RequestRejected{WorkspaceID: e.ws, RequestID: req.ID, Type: "leave", RequesterID: e.alice.ID, ApproverID: e.bob.ID, ActorID: e.bob.ID, Reason: "no"})
	if len(rejected) != 1 || rejected[0].UserID != e.alice.ID || rejected[0].Kind != "approval.rejected" || !strings.Contains(rejected[0].Body, "no") {
		t.Errorf("rejected = %+v", rejected)
	}
	cancelled := e.ruleNotifications(approval.RequestCancelled{WorkspaceID: e.ws, RequestID: req.ID, Type: "leave", RequesterID: e.alice.ID, ApproverID: e.bob.ID, ActorID: e.alice.ID})
	if len(cancelled) != 1 || cancelled[0].UserID != e.bob.ID || cancelled[0].Kind != "approval.cancelled" {
		t.Errorf("cancelled = %+v", cancelled)
	}
	for _, n := range append(append(created, approved...), append(rejected, cancelled...)...) {
		if n.WorkspaceID != e.ws || n.Link != "/approvals/"+req.ID || n.Data["request_id"] != req.ID {
			t.Errorf("notification = %+v", n)
		}
	}
}

func TestRegistersItsCardKind(t *testing.T) {
	e := newEnv(t)
	e.start()
	if len(e.cards.kinds) != 1 || e.cards.kinds[0] != (app.CardKind{Kind: "approval.request", Module: "approval"}) {
		t.Errorf("card kinds = %+v", e.cards.kinds)
	}
}

func TestWorksWithoutChatAndWithoutNotifications(t *testing.T) {
	// A server where the card registry is missing and notify is off still
	// runs approvals (NOTI-05, PLT-11): only the card is absent.
	e := newEnvWith(t, false, true)
	e.deps.Notifications = app.NoopNotificationRegistry{}
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	if r := e.call("POST", "/approval/requests/"+req.ID+"/approve", e.bob, nil); r.Code != 200 {
		t.Fatalf("approve = %d %s", r.Code, r.Body)
	}
	got := e.fetch(e.alice, req.ID)
	if got.Status != "approved" || got.MessageID != nil {
		t.Errorf("request = %+v", got)
	}
	if e.cards.posts != 0 {
		t.Errorf("cards posted = %d", e.cards.posts)
	}
}

func TestACardFailureNeverFailsTheDecision(t *testing.T) {
	e := newEnv(t)
	e.start()
	req := e.submit(e.alice, withApprover(leave(), e.bob))
	e.cards.mu.Lock()
	e.cards.cards = nil // Update now fails for every card
	e.cards.mu.Unlock()
	if r := e.call("POST", "/approval/requests/"+req.ID+"/approve", e.bob, nil); r.Code != 200 {
		t.Fatalf("approve = %d %s", r.Code, r.Body)
	}
	if got := e.fetch(e.bob, req.ID); got.Status != "approved" {
		t.Errorf("status = %s", got.Status)
	}
}
