package acceptance

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// allModulesConfig is cmd/server's wiring with every module on, so the
// endpoints of all seven modules exist: the real rules registry, the job queue,
// the outbox bus and object storage for presigned URLs.
func allModulesConfig(store app.ObjectStorage) instanceConfig {
	return instanceConfig{
		modules:       []string{"auth", "workspace", "chat", "notify", "task", "approval", "helpdesk"},
		notifications: app.NewNotificationRules(),
		jobs:          true,
		outbox:        true,
		storage:       store,
	}
}

// unscopedOps are the operations that do not act in a workspace: they concern
// the caller's own account, a one-time token, or a public step, so NFR-06 does
// not apply to them. Every other operation of the spec needs a case in
// scopedOps.
var unscopedOps = map[string]string{
	"get-meta":                 "public",
	"auth-register":            "public",
	"auth-login":               "public",
	"auth-refresh":             "refresh token",
	"auth-logout":              "the caller's own session",
	"auth-forgot-password":     "public; always 204 so it does not reveal accounts",
	"auth-reset-password":      "emailed one-time token",
	"auth-verify-email":        "emailed one-time token",
	"auth-resend-verification": "the caller's own account",
	"auth-get-me":              "the caller's own profile",
	"auth-update-me":           "the caller's own profile",
	"auth-list-devices":        "the caller's own devices",
	"auth-revoke-device":       "the caller's own devices",
	"workspace-list":           "the caller's own workspaces",
	"workspace-create":         "creates a workspace for the caller",
	"workspace-accept-invite":  "the holder of an invite token joins",
}

// scopedOp is one workspace-scoped operation and a request that aims at the
// victim workspace's resources. path names the victim's IDs wherever the route
// has an ID; body is valid for the operation, so validation does not answer
// before authorization does.
type scopedOp struct {
	id     string
	method string
	path   string
	body   any
}

// victim is a workspace with one of every kind of resource the modules keep.
// Its owner builds them, so each operation has a real ID from the victim to aim
// at, and not just an ID that does not exist.
type victim struct {
	ws                      string
	owner, member, carol    user
	channel, taskRoom       string // channels with bob; the task belongs to taskRoom
	message                 string // alice's message in channel
	upload                  string // a chat attachment alice uploaded and has not sent
	notification            string // bob's notification from a mention in channel
	task                    string
	queue, category, ticket string
	ticketFile              string // a helpdesk attachment alice uploaded
	request, requestFile    string // an approval request to carol; alice's uploaded file
	group, invite           string
}

// ids returns every ID the victim owns, so a response from another workspace
// can be checked for any of them.
func (v victim) ids() []string {
	return []string{
		v.ws, v.channel, v.taskRoom, v.message, v.upload, v.notification, v.task,
		v.queue, v.category, v.ticket, v.ticketFile, v.request, v.requestFile,
		v.group, v.invite, v.owner.id, v.member.id, v.carol.id,
	}
}

// seedVictim builds the victim workspace as its owner, alice. bob is a member
// with a mention, and carol is the approver of the request.
func seedVictim(t *testing.T, c client) victim {
	t.Helper()
	alice := c.signUp("alice")
	bob := c.signUp("bob")
	carol := c.signUp("carol")
	ws := c.createWorkspace(alice)
	for _, u := range []user{bob, carol} {
		c.addMember(alice, ws, u)
	}
	v := victim{ws: ws, owner: alice, member: bob, carol: carol}

	v.channel = c.createChannel(alice, ws, unique("sweep-room"), []string{bob.id})
	v.taskRoom = c.createChannel(alice, ws, unique("sweep-tasks"), []string{bob.id})
	agents := c.createChannel(alice, ws, unique("sweep-agents"), []string{bob.id})

	msg := c.want(c.send(alice, ws, v.channel, uuid.Must(uuid.NewV7()).String(), "please review <@"+bob.id+">"), 201)
	v.message = msg.str("id")
	v.notification = awaitNotification(t, c, bob, ws, 5*time.Second)

	v.upload = c.want(c.do("POST", "/chat/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
		"channel_id": v.channel, "file_name": "notes.txt", "size": 12, "mime_type": "text/plain",
	}}), 201).str("attachment_id")

	v.task = c.want(c.do("POST", "/task/items", call{token: alice.access, workspace: ws, body: map[string]any{
		"channel_id": v.taskRoom, "title": "Prepare the slides",
	}}), 201).str("id")

	v.queue = c.createQueue(alice, ws, "Support", "SWP", agents)
	c.setQueueAgents(alice, ws, v.queue, []string{bob.id})
	v.category = c.createCategory(alice, ws, v.queue, "General")
	v.ticket = c.want(c.do("POST", "/helpdesk/tickets", call{token: alice.access, workspace: ws, body: map[string]any{
		"category_id": v.category, "title": "Printer jammed", "priority": "normal",
	}}), 201).str("id")
	v.ticketFile = c.want(c.do("POST", "/helpdesk/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
		"file_name": "log.txt", "size": 10, "mime_type": "text/plain",
	}}), 201).str("attachment_id")

	v.request = c.want(c.do("POST", "/approval/requests", call{token: alice.access, workspace: ws, body: map[string]any{
		"type": "general", "payload": map[string]any{"subject": "A monitor", "details": "For design"}, "approver_id": carol.id,
	}}), 201).str("id")
	v.requestFile = c.want(c.do("POST", "/approval/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
		"file_name": "quote.pdf", "size": 20, "mime_type": "application/pdf",
	}}), 201).str("attachment_id")

	v.group = c.createGroup(alice, ws, "Design")
	c.setGroupMembers(alice, ws, v.group, []map[string]any{{"user_id": alice.id, "is_manager": true}})
	v.invite = c.want(c.do("POST", "/workspace/invites", call{token: alice.access, workspace: ws, body: map[string]any{
		"email": unique("invitee") + "@example.com", "role": "member",
	}}), 201).str("id")

	// A reply without the field would give "". A path with an empty ID answers
	// 404 whatever the workspace, so the sweep would pass for no reason.
	for name, id := range map[string]string{
		"channel": v.channel, "taskRoom": v.taskRoom, "message": v.message, "upload": v.upload,
		"notification": v.notification, "task": v.task, "queue": v.queue, "category": v.category,
		"ticket": v.ticket, "ticketFile": v.ticketFile, "request": v.request, "requestFile": v.requestFile,
		"group": v.group, "invite": v.invite,
	} {
		if id == "" {
			t.Fatalf("seeding the victim: no %s ID", name)
		}
	}
	return v
}

// awaitNotification waits for u to have a notification in ws and returns its ID.
// Notices are written after the commit that caused them (PLT-10), so they can
// arrive a moment after the request.
func awaitNotification(t *testing.T, c client, u user, ws string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		items := c.want(c.do("GET", "/notify/notifications", call{token: u.access, workspace: ws}), 200).items()
		if len(items) > 0 {
			id, _ := items[0]["id"].(string)
			return id
		}
		if time.Now().After(deadline) {
			t.Fatalf("no notification for %s within %s", u.email, within)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// scopedOps is one case per workspace-scoped operation, grouped by module.
func scopedOps(v victim) []scopedOp {
	return []scopedOp{
		// workspace
		{"workspace-list-members", http.MethodGet, "/workspace/members", nil},
		{"workspace-update-member", http.MethodPatch, "/workspace/members/" + v.member.id, map[string]any{"role": "guest"}},
		{"workspace-remove-member", http.MethodDelete, "/workspace/members/" + v.member.id, nil},
		{"workspace-list-groups", http.MethodGet, "/workspace/groups", nil},
		{"workspace-create-group", http.MethodPost, "/workspace/groups", map[string]any{"name": "Intruders"}},
		{"workspace-rename-group", http.MethodPatch, "/workspace/groups/" + v.group, map[string]any{"name": "Hijacked"}},
		{"workspace-delete-group", http.MethodDelete, "/workspace/groups/" + v.group, nil},
		{"workspace-list-group-members", http.MethodGet, "/workspace/groups/" + v.group + "/members", nil},
		{"workspace-set-group-members", http.MethodPut, "/workspace/groups/" + v.group + "/members",
			map[string]any{"members": []map[string]any{{"user_id": v.member.id, "is_manager": false}}}},
		{"workspace-list-audit", http.MethodGet, "/workspace/audit", nil},
		{"workspace-create-invite", http.MethodPost, "/workspace/invites", map[string]any{"email": unique("intruder") + "@example.com", "role": "member"}},
		{"workspace-list-invites", http.MethodGet, "/workspace/invites", nil},
		{"workspace-revoke-invite", http.MethodDelete, "/workspace/invites/" + v.invite, nil},

		// chat
		{"chat-list-channels", http.MethodGet, "/chat/channels", nil},
		{"chat-create-channel", http.MethodPost, "/chat/channels", map[string]any{"type": "public", "name": unique("intruder")}},
		{"chat-open-dm", http.MethodPost, "/chat/dms", map[string]any{"user_ids": []string{v.member.id}}},
		{"chat-get-channel", http.MethodGet, "/chat/channels/" + v.channel, nil},
		{"chat-update-channel", http.MethodPatch, "/chat/channels/" + v.channel, map[string]any{"name": unique("hijacked")}},
		{"chat-join-channel", http.MethodPost, "/chat/channels/" + v.channel + "/join", nil},
		{"chat-leave-channel", http.MethodPost, "/chat/channels/" + v.channel + "/leave", nil},
		{"chat-list-members", http.MethodGet, "/chat/channels/" + v.channel + "/members", nil},
		{"chat-add-members", http.MethodPost, "/chat/channels/" + v.channel + "/members", map[string]any{"user_ids": []string{v.member.id}}},
		{"chat-list-messages", http.MethodGet, "/chat/channels/" + v.channel + "/messages", nil},
		{"chat-send-message", http.MethodPost, "/chat/channels/" + v.channel + "/messages",
			map[string]any{"client_msg_id": uuid.Must(uuid.NewV7()).String(), "body": "intruder"}},
		{"chat-edit-message", http.MethodPatch, "/chat/messages/" + v.message, map[string]any{"body": "hijacked"}},
		{"chat-delete-message", http.MethodDelete, "/chat/messages/" + v.message, nil},
		{"chat-request-upload", http.MethodPost, "/chat/uploads",
			map[string]any{"channel_id": v.channel, "file_name": "x.txt", "size": 10, "mime_type": "text/plain"}},
		{"chat-get-attachment", http.MethodGet, "/chat/attachments/" + v.upload, nil},
		{"chat-add-reaction", http.MethodPost, "/chat/messages/" + v.message + "/reactions", map[string]any{"emoji": "👍"}},
		{"chat-remove-reaction", http.MethodDelete, "/chat/messages/" + v.message + "/reactions/" + url.PathEscape("👍"), nil},
		{"chat-list-extensions", http.MethodGet, "/chat/extensions", nil},
		{"chat-run-command", http.MethodPost, "/chat/channels/" + v.channel + "/commands", map[string]any{"name": "task", "args": "x"}},
		{"chat-mark-read", http.MethodPost, "/chat/channels/" + v.channel + "/read", map[string]any{"seq": 1}},
		{"chat-pin-channel", http.MethodPut, "/chat/channels/" + v.channel + "/pin", nil},
		{"chat-unpin-channel", http.MethodDelete, "/chat/channels/" + v.channel + "/pin", nil},

		// notify
		{"notify-list-notifications", http.MethodGet, "/notify/notifications", nil},
		{"notify-unread-count", http.MethodGet, "/notify/unread-count", nil},
		{"notify-mark-read", http.MethodPost, "/notify/notifications/" + v.notification + "/read", nil},
		{"notify-read-all", http.MethodPost, "/notify/read-all", nil},
		{"notify-get-preferences", http.MethodGet, "/notify/preferences", nil},
		{"notify-set-preferences", http.MethodPut, "/notify/preferences",
			map[string]any{"disabled_kinds": []string{}, "muted_channel_ids": []string{}, "dnd_start": nil, "dnd_end": nil}},

		// task
		{"task-list-items", http.MethodGet, "/task/items", nil},
		{"task-create-item", http.MethodPost, "/task/items", map[string]any{"channel_id": v.taskRoom, "title": "Intruder task"}},
		{"task-get-item", http.MethodGet, "/task/items/" + v.task, nil},
		{"task-update-item", http.MethodPatch, "/task/items/" + v.task, map[string]any{"status": "done"}},

		// approval
		{"approval-list-requests", http.MethodGet, "/approval/requests", nil},
		{"approval-create-request", http.MethodPost, "/approval/requests", map[string]any{
			"type": "general", "payload": map[string]any{"subject": "x", "details": "y"}, "approver_id": v.carol.id,
		}},
		{"approval-get-request", http.MethodGet, "/approval/requests/" + v.request, nil},
		{"approval-approve-request", http.MethodPost, "/approval/requests/" + v.request + "/approve", nil},
		{"approval-reject-request", http.MethodPost, "/approval/requests/" + v.request + "/reject", map[string]any{"reason": "no"}},
		{"approval-cancel-request", http.MethodPost, "/approval/requests/" + v.request + "/cancel", nil},
		{"approval-request-upload", http.MethodPost, "/approval/uploads",
			map[string]any{"file_name": "x.pdf", "size": 10, "mime_type": "application/pdf"}},
		{"approval-get-attachment", http.MethodGet, "/approval/attachments/" + v.requestFile, nil},

		// helpdesk
		{"helpdesk-list-queues", http.MethodGet, "/helpdesk/queues", nil},
		{"helpdesk-create-queue", http.MethodPost, "/helpdesk/queues", map[string]any{"name": "Intruders", "prefix": "INT"}},
		{"helpdesk-update-queue", http.MethodPatch, "/helpdesk/queues/" + v.queue, map[string]any{"name": "Hijacked"}},
		{"helpdesk-set-queue-agents", http.MethodPut, "/helpdesk/queues/" + v.queue + "/agents", map[string]any{"user_ids": []string{v.member.id}}},
		{"helpdesk-create-category", http.MethodPost, "/helpdesk/categories", map[string]any{"queue_id": v.queue, "name": "Intruders"}},
		{"helpdesk-update-category", http.MethodPatch, "/helpdesk/categories/" + v.category, map[string]any{"name": "Hijacked"}},
		{"helpdesk-list-tickets", http.MethodGet, "/helpdesk/tickets", nil},
		{"helpdesk-create-ticket", http.MethodPost, "/helpdesk/tickets", map[string]any{
			"category_id": v.category, "title": "Intruder ticket", "priority": "low",
		}},
		{"helpdesk-get-ticket", http.MethodGet, "/helpdesk/tickets/" + v.ticket, nil},
		{"helpdesk-assign-ticket", http.MethodPost, "/helpdesk/tickets/" + v.ticket + "/assign", map[string]any{"assignee_id": v.member.id}},
		{"helpdesk-change-ticket-status", http.MethodPost, "/helpdesk/tickets/" + v.ticket + "/status", map[string]any{"status": "open"}},
		{"helpdesk-edit-ticket", http.MethodPatch, "/helpdesk/tickets/" + v.ticket, map[string]any{"priority": "urgent"}},
		{"helpdesk-add-comment", http.MethodPost, "/helpdesk/tickets/" + v.ticket + "/comments", map[string]any{"body": "intruder"}},
		{"helpdesk-request-upload", http.MethodPost, "/helpdesk/uploads",
			map[string]any{"file_name": "x.txt", "size": 10, "mime_type": "text/plain"}},
		{"helpdesk-get-attachment", http.MethodGet, "/helpdesk/attachments/" + v.ticketFile, nil},
	}
}

// specOp is the method and route pattern of one operation of the served spec.
type specOp struct {
	method, path string
}

// specDocument is the OpenAPI document the server serves, as generic JSON, so
// a test can read it without a YAML dependency.
func specDocument(t *testing.T, api huma.API) map[string]any {
	t.Helper()
	raw, err := api.OpenAPI().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// operationIndex returns every operation of the served spec, by operationId.
func operationIndex(t *testing.T, api huma.API) map[string]specOp {
	t.Helper()
	paths, _ := specDocument(t, api)["paths"].(map[string]any)
	out := map[string]specOp{}
	for path, item := range paths {
		methods, _ := item.(map[string]any)
		for method, raw := range methods {
			op, ok := raw.(map[string]any)
			if !ok {
				continue // path-level "parameters" and "summary" are not operations
			}
			if id, _ := op["operationId"].(string); id != "" {
				out[id] = specOp{method: strings.ToUpper(method), path: path}
			}
		}
	}
	return out
}

// routeMatches reports whether a concrete path is an instance of a route
// pattern such as /chat/channels/{id}/messages.
func routeMatches(pattern, path string) bool {
	p, c := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(p) != len(c) {
		return false
	}
	for i := range p {
		if strings.HasPrefix(p[i], "{") {
			if c[i] == "" {
				return false
			}
			continue
		}
		if p[i] != c[i] {
			return false
		}
	}
	return true
}

// checkCoverage fails for every operation of the spec that has no case and is
// not unscoped, and for every case that names no operation, the wrong method
// or the wrong route. A new endpoint therefore cannot ship without a case.
func checkCoverage(t *testing.T, ops map[string]specOp, cases []scopedOp) {
	t.Helper()
	seen := map[string]bool{}
	for _, cs := range cases {
		spec, ok := ops[cs.id]
		switch {
		case !ok:
			t.Errorf("case %q is not an operation of the served spec", cs.id)
		case spec.method != cs.method:
			t.Errorf("case %q sends %s, the spec says %s", cs.id, cs.method, spec.method)
		case !routeMatches(spec.path, cs.path):
			t.Errorf("case %q sends %s, which is not an instance of %s", cs.id, cs.path, spec.path)
		case seen[cs.id]:
			t.Errorf("case %q is listed twice", cs.id)
		}
		if _, ok := unscopedOps[cs.id]; ok {
			t.Errorf("case %q is also listed as unscoped", cs.id)
		}
		seen[cs.id] = true
	}
	for id, spec := range ops {
		if _, ok := unscopedOps[id]; ok || seen[id] {
			continue
		}
		t.Errorf("operation %s (%s %s) has no isolation case: add it to scopedOps, or to unscopedOps with a reason", id, spec.method, spec.path)
	}
	for id := range unscopedOps {
		if _, ok := ops[id]; !ok {
			t.Errorf("unscoped operation %q is not an operation of the served spec", id)
		}
	}
}

// victimState is what the owner sees of the victim workspace. The sweep must
// leave every entry as it was: a write that got through would change one.
// Notifications are left out: notices from earlier work can still be arriving.
func victimState(t *testing.T, c client, v victim) map[string]string {
	t.Helper()
	reads := map[string]string{
		"channel":         "/chat/channels/" + v.channel,
		"channel members": "/chat/channels/" + v.channel + "/members",
		"messages":        "/chat/channels/" + v.channel + "/messages",
		"task":            "/task/items/" + v.task,
		"queues":          "/helpdesk/queues",
		"ticket":          "/helpdesk/tickets/" + v.ticket,
		"request":         "/approval/requests/" + v.request,
		"groups":          "/workspace/groups",
		"group members":   "/workspace/groups/" + v.group + "/members",
		"invites":         "/workspace/invites",
		"members":         "/workspace/members",
		"audit":           "/workspace/audit",
	}
	out := make(map[string]string, len(reads))
	for name, path := range reads {
		body, err := json.Marshal(c.want(c.do("GET", path, call{token: v.owner.access, workspace: v.ws}), 200).Body)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(body)
	}
	return out
}

// TestWorkspaceIsolation is NFR-06: a member of another workspace cannot read
// or write the data of a workspace, in any module. Every workspace-scoped
// operation of the spec is called two ways by mallory, who owns a workspace of
// her own:
//
//  1. with the victim's workspace in X-Workspace-ID, which she is not a member
//     of: 403 or 404;
//  2. with her own workspace in X-Workspace-ID, which she owns, and the
//     victim's IDs in the path or body: 403 or 404 where the route names an ID,
//     and no victim ID in a successful answer otherwise.
//
// The owner's view of the victim is compared before and after, so a write that
// got through would show even if its status looked right.
func TestWorkspaceIsolation(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	in := newInstanceWith(t, pool, rdb, allModulesConfig(testStorage(t)))
	c := client{t: t, url: in.url}

	v := seedVictim(t, c)
	mallory := c.signUp("mallory")
	wsB := c.createWorkspace(mallory)
	ops := operationIndex(t, in.api)
	cases := scopedOps(v)

	t.Run("every workspace-scoped operation has a case", func(t *testing.T) {
		checkCoverage(t, ops, cases)
	})

	before := victimState(t, c, v)
	victimIDs := v.ids()
	for _, cs := range cases {
		t.Run(cs.id, func(t *testing.T) {
			sc := client{t: t, url: in.url}
			spec := ops[cs.id]

			outside := sc.do(cs.method, cs.path, call{token: mallory.access, workspace: v.ws, body: cs.body})
			if outside.Status != http.StatusForbidden && outside.Status != http.StatusNotFound {
				t.Errorf("not a member of workspace %s: %s %s = %d, want 403 or 404: %v",
					v.ws, cs.method, cs.path, outside.Status, outside.Body)
			}

			own := sc.do(cs.method, cs.path, call{token: mallory.access, workspace: wsB, body: cs.body})
			if strings.Contains(spec.path, "{") {
				if own.Status != http.StatusForbidden && own.Status != http.StatusNotFound {
					t.Errorf("own workspace %s with the victim's ID: %s %s = %d, want 403 or 404: %v",
						wsB, cs.method, cs.path, own.Status, own.Body)
				}
				return
			}
			if own.Status >= 500 {
				t.Errorf("own workspace %s: %s %s = %d: %v", wsB, cs.method, cs.path, own.Status, own.Body)
			}
			if own.Status < 300 {
				raw, _ := json.Marshal(own.Body)
				for _, id := range victimIDs {
					if strings.Contains(string(raw), id) {
						t.Errorf("own workspace %s: %s %s answered %d with the victim's ID %s: %s",
							wsB, cs.method, cs.path, own.Status, id, raw)
					}
				}
			}
		})
	}

	after := victimState(t, c, v)
	for name := range before {
		if before[name] != after[name] {
			t.Errorf("the victim's %s changed during the sweep:\nbefore: %s\nafter:  %s", name, before[name], after[name])
		}
	}
	t.Logf("swept %d workspace-scoped operations; %d unscoped operations are excluded by reason", len(cases), len(unscopedOps))
}
