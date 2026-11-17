package chat_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type commandBody struct {
	Name        string `json:"name"`
	Usage       string `json:"usage"`
	Description string `json:"description"`
	Module      string `json:"module"`
}

type extensionsBody struct {
	Commands []commandBody `json:"commands"`
	Actions  []struct {
		ID     string `json:"id"`
		Module string `json:"module"`
	} `json:"message_actions"`
}

// callLog records what a command handler saw.
type callLog struct {
	mu        sync.Mutex
	principal app.Principal
	hasP      bool
	input     app.CommandInput
	calls     int
}

func (l *callLog) handler(res app.CommandResult, err error) app.CommandHandler {
	return func(ctx context.Context, in app.CommandInput) (app.CommandResult, error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.principal, l.hasP = app.PrincipalFrom(ctx)
		l.input = in
		l.calls++
		return res, err
	}
}

func (e *env) extensions() app.ChatExtensions {
	e.t.Helper()
	x, ok := app.Lookup[app.ChatExtensions](e.svc)
	if !ok {
		e.t.Fatal("chat does not provide app.ChatExtensions")
	}
	return x
}

func (e *env) access() app.ChannelAccess {
	e.t.Helper()
	a, ok := app.Lookup[app.ChannelAccess](e.svc)
	if !ok {
		e.t.Fatal("chat does not provide app.ChannelAccess")
	}
	return a
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}

func TestExtensionsListIsEmptyWithoutRegistrations(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")

	r := e.call("GET", "/chat/extensions", alice, ws, nil)
	if r.Code != 200 {
		t.Fatalf("list = %d %s", r.Code, r.Body)
	}
	// Both lists are arrays, never null.
	if !strings.Contains(string(r.Body), `"commands":[]`) || !strings.Contains(string(r.Body), `"message_actions":[]`) {
		t.Errorf("body = %s", r.Body)
	}
	if r := e.call("GET", "/chat/extensions", e.newUser("nobody"), ws, nil); r.Code != 403 {
		t.Errorf("non-member = %d, want 403", r.Code)
	}
}

func TestExtensionsListShowsRegistrationsByName(t *testing.T) {
	e := newEnv(t)
	x := e.extensions()
	noop := (&callLog{}).handler(app.CommandResult{}, nil)
	x.RegisterCommand(app.Command{Name: "task", Module: "task", Usage: "/task <title>", Description: "Create a task", Handler: noop})
	x.RegisterCommand(app.Command{Name: "approve", Module: "approval", Usage: "/approve", Description: "Ask for approval", Handler: noop})
	x.RegisterMessageAction(app.MessageAction{ID: "task.create_from_message", Module: "task"})
	x.RegisterMessageAction(app.MessageAction{ID: "approval.ask", Module: "approval"})
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")

	r := e.call("GET", "/chat/extensions", alice, ws, nil)
	var got extensionsBody
	r.json(t, &got)
	if r.Code != 200 || len(got.Commands) != 2 || len(got.Actions) != 2 {
		t.Fatalf("list = %d %s", r.Code, r.Body)
	}
	if got.Commands[0] != (commandBody{Name: "approve", Usage: "/approve", Description: "Ask for approval", Module: "approval"}) ||
		got.Commands[1] != (commandBody{Name: "task", Usage: "/task <title>", Description: "Create a task", Module: "task"}) {
		t.Errorf("commands = %+v", got.Commands)
	}
	if got.Actions[0].ID != "approval.ask" || got.Actions[0].Module != "approval" || got.Actions[1].ID != "task.create_from_message" {
		t.Errorf("actions = %+v", got.Actions)
	}
}

func TestRegisteringCommandsPanicsOnBadInput(t *testing.T) {
	e := newEnv(t)
	x := e.extensions()
	noop := (&callLog{}).handler(app.CommandResult{}, nil)
	x.RegisterCommand(app.Command{Name: "task", Module: "task", Handler: noop})

	mustPanic(t, "a duplicate name", func() { x.RegisterCommand(app.Command{Name: "task", Module: "other", Handler: noop}) })
	for _, name := range []string{"", "Task", "ta sk", "/task", "task_x", "tâsk", "ถาม", strings.Repeat("a", 33)} {
		mustPanic(t, fmt.Sprintf("name %q", name), func() { x.RegisterCommand(app.Command{Name: name, Module: "m", Handler: noop}) })
	}
	mustPanic(t, "a missing handler", func() { x.RegisterCommand(app.Command{Name: "nohandler", Module: "m"}) })
	mustPanic(t, "a missing module", func() { x.RegisterCommand(app.Command{Name: "nomodule", Handler: noop}) })

	// The accepted shapes: letters, digits and "-", 1 to 32 characters.
	for _, name := range []string{"a", "t-1", "0", strings.Repeat("a", 32)} {
		x.RegisterCommand(app.Command{Name: name, Module: "m", Handler: noop})
	}

	x.RegisterMessageAction(app.MessageAction{ID: "task.create_from_message", Module: "task"})
	mustPanic(t, "a duplicate action", func() { x.RegisterMessageAction(app.MessageAction{ID: "task.create_from_message", Module: "task"}) })
	for _, a := range []app.MessageAction{{ID: "", Module: "task"}, {ID: "nodot", Module: "nodot"}, {ID: "task.x", Module: "approval"}, {ID: "task.", Module: "task"}, {ID: "Task.x", Module: "Task"}} {
		mustPanic(t, fmt.Sprintf("action %+v", a), func() { x.RegisterMessageAction(a) })
	}
}

func TestRunCommandPassesPrincipalAndInput(t *testing.T) {
	e := newEnv(t)
	log := &callLog{}
	e.extensions().RegisterCommand(app.Command{Name: "echo", Module: "demo", Handler: log.handler(app.CommandResult{Text: "done"}, nil)})
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "general")
	root := e.send(alice, ws, ch.ID, "root")

	r := e.call("POST", "/chat/channels/"+ch.ID+"/commands", alice, ws, map[string]any{"name": "echo", "args": "  hello world  ", "thread_root_id": root.ID})
	var out struct {
		Text string `json:"text"`
	}
	r.json(t, &out)
	if r.Code != 200 || out.Text != "done" {
		t.Fatalf("run = %d %s", r.Code, r.Body)
	}
	if !log.hasP || log.principal.UserID != alice.ID || log.principal.WorkspaceID != ws || log.principal.Role != app.RoleMember {
		t.Errorf("principal = %+v (present %v)", log.principal, log.hasP)
	}
	if want := (app.CommandInput{ChannelID: ch.ID, ThreadRootID: root.ID, Args: "hello world"}); log.input != want {
		t.Errorf("input = %+v, want %+v", log.input, want)
	}

	// No args and no thread: both empty.
	r = e.call("POST", "/chat/channels/"+ch.ID+"/commands", alice, ws, map[string]any{"name": "echo"})
	if r.Code != 200 || log.input != (app.CommandInput{ChannelID: ch.ID}) {
		t.Errorf("bare run = %d %s, input %+v", r.Code, r.Body, log.input)
	}
}

func TestRunCommandMapsHandlerErrors(t *testing.T) {
	e := newEnv(t)
	x := e.extensions()
	for name, err := range map[string]error{
		"bad":    fmt.Errorf("title is required: %w", app.ErrInvalid),
		"nope":   fmt.Errorf("not yours: %w", app.ErrForbidden),
		"broken": errors.New("database exploded"),
	} {
		x.RegisterCommand(app.Command{Name: name, Module: "demo", Handler: (&callLog{}).handler(app.CommandResult{}, err)})
	}
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "general")

	for name, want := range map[string]int{"bad": 400, "nope": 403, "broken": 500, "missing": 404} {
		r := e.call("POST", "/chat/channels/"+ch.ID+"/commands", alice, ws, map[string]any{"name": name})
		if r.Code != want {
			t.Errorf("command %q = %d %s, want %d", name, r.Code, r.Body, want)
		}
		if name == "broken" && strings.Contains(string(r.Body), "exploded") {
			t.Errorf("500 leaks the error: %s", r.Body)
		}
	}
}

func TestRunCommandNeedsAReadableChannel(t *testing.T) {
	e := newEnv(t)
	log := &callLog{}
	e.extensions().RegisterCommand(app.Command{Name: "echo", Module: "demo", Handler: log.handler(app.CommandResult{Text: "ok"}, nil)})
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	guest := e.newUser("guest")
	e.join(ws, guest, app.RoleGuest)
	private := e.createChannel(alice, ws, "private", "secret")
	public := e.createChannel(alice, ws, "public", "open")
	run := func(u user, channelID, name string) int {
		return e.call("POST", "/chat/channels/"+channelID+"/commands", u, ws, map[string]any{"name": name}).Code
	}

	if got := run(bob, private.ID, "echo"); got != 404 {
		t.Errorf("non-member in a private channel = %d, want 404", got)
	}
	// An unknown command in an unreadable channel is still 404 and the handler never ran.
	if got := run(bob, private.ID, "missing"); got != 404 {
		t.Errorf("unknown command in a private channel = %d, want 404", got)
	}
	if got := run(guest, public.ID, "echo"); got != 404 {
		t.Errorf("guest outside a public channel = %d, want 404", got)
	}
	if got := run(alice, "not-a-uuid", "echo"); got != 404 {
		t.Errorf("bad channel ID = %d, want 404", got)
	}
	if log.calls != 0 {
		t.Fatalf("handler ran %d times for unreadable channels", log.calls)
	}
	// A non-member can use a command in a public channel (they can read it), and members in private ones.
	if got := run(bob, public.ID, "echo"); got != 200 {
		t.Errorf("public channel = %d, want 200", got)
	}
	if got := run(alice, private.ID, "echo"); got != 200 {
		t.Errorf("private member = %d, want 200", got)
	}
	// A user who is not in the workspace never gets this far.
	if r := e.call("POST", "/chat/channels/"+public.ID+"/commands", e.newUser("outsider"), ws, map[string]any{"name": "echo"}); r.Code != 403 {
		t.Errorf("outsider = %d, want 403", r.Code)
	}
}

func TestRunCommandChecksThreadRootAndArgs(t *testing.T) {
	e := newEnv(t)
	log := &callLog{}
	e.extensions().RegisterCommand(app.Command{Name: "echo", Module: "demo", Handler: log.handler(app.CommandResult{}, nil)})
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	a := e.createChannel(alice, ws, "public", "a")
	b := e.createChannel(alice, ws, "public", "b")
	other := e.send(alice, ws, b.ID, "elsewhere")
	reply := e.sendWith(alice, ws, a.ID, "0192a3b4-0000-7000-8000-0000000000a1", "reply", e.send(alice, ws, a.ID, "root").ID)
	run := func(body map[string]any) int {
		body["name"] = "echo"
		return e.call("POST", "/chat/channels/"+a.ID+"/commands", alice, ws, body).Code
	}

	for what, root := range map[string]string{"malformed": "nope", "another channel": other.ID, "a reply": reply.ID, "unknown": "0192a3b4-0000-7000-8000-0000000000ff"} {
		if got := run(map[string]any{"thread_root_id": root}); got != 400 {
			t.Errorf("thread root %s = %d, want 400", what, got)
		}
	}
	if got := run(map[string]any{"args": strings.Repeat("x", 4001)}); got != 400 {
		t.Errorf("args of 4001 characters = %d, want 400", got)
	}
	if log.calls != 0 {
		t.Errorf("handler ran %d times for bad input", log.calls)
	}
}

func TestChannelAccessMatchesChannelReadRules(t *testing.T) {
	e := newEnv(t)
	access := e.access()
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	guest, gone := e.newUser("guest"), e.newUser("gone")
	e.join(ws, guest, app.RoleGuest)
	e.join(ws, gone, app.RoleMember)
	admin := e.newUser("admin")
	e.join(ws, admin, app.RoleAdmin)

	public := e.createChannel(alice, ws, "public", "open")
	private := e.createChannel(alice, ws, "private", "secret", bob.ID)
	dm := e.call("POST", "/chat/dms", alice, ws, map[string]any{"user_ids": []string{bob.ID}})
	var dmCh channelBody
	dm.json(t, &dmCh)
	if dm.Code != 201 && dm.Code != 200 {
		t.Fatalf("open dm = %d %s", dm.Code, dm.Body)
	}
	// A guest who was added to a private channel can read it.
	guestIn := e.createChannel(alice, ws, "private", "guests", guest.ID)
	archived := e.createChannel(alice, ws, "public", "old")
	if r := e.call("PATCH", "/chat/channels/"+archived.ID, admin, ws, map[string]any{"archived": true}); r.Code != 200 {
		t.Fatalf("archive = %d %s", r.Code, r.Body)
	}
	privateArchived := e.createChannel(alice, ws, "private", "old-secret", bob.ID)
	if r := e.call("PATCH", "/chat/channels/"+privateArchived.ID, alice, ws, map[string]any{"archived": true}); r.Code != 200 {
		t.Fatalf("archive private = %d %s", r.Code, r.Body)
	}

	for _, tc := range []struct {
		what   string
		ws, ch string
		user   string
		want   bool
	}{
		{"active member, public, not joined", ws, public.ID, carol.ID, true},
		{"creator, public", ws, public.ID, alice.ID, true},
		{"guest, public, not joined", ws, public.ID, guest.ID, false},
		{"guest, private they are in", ws, guestIn.ID, guest.ID, true},
		{"member of private", ws, private.ID, bob.ID, true},
		{"non-member of private", ws, private.ID, carol.ID, false},
		{"admin who is not in private", ws, private.ID, admin.ID, false},
		{"dm member", ws, dmCh.ID, bob.ID, true},
		{"not in the dm", ws, dmCh.ID, carol.ID, false},
		{"archived public still readable", ws, archived.ID, carol.ID, true},
		{"archived private, member", ws, privateArchived.ID, bob.ID, true},
		{"archived private, non-member", ws, privateArchived.ID, carol.ID, false},
		{"user outside the workspace", ws, public.ID, e.newUser("outsider").ID, false},
		{"channel of another workspace", other, public.ID, alice.ID, false},
		{"unknown channel", ws, "0192a3b4-0000-7000-8000-0000000000ff", alice.ID, false},
		{"malformed channel ID", ws, "nope", alice.ID, false},
		{"malformed user ID", ws, public.ID, "nope", false},
		{"malformed workspace ID", "nope", public.ID, alice.ID, false},
	} {
		got, err := access.CanRead(context.Background(), tc.ws, tc.ch, tc.user)
		if err != nil || got != tc.want {
			t.Errorf("%s: CanRead = %v, %v; want %v", tc.what, got, err, tc.want)
		}
	}

	// The answer follows membership at once (WSP-08) and agrees with the REST read rule.
	e.members.remove(ws, gone.ID)
	if got, err := access.CanRead(context.Background(), ws, public.ID, gone.ID); err != nil || got {
		t.Errorf("removed member: CanRead = %v, %v; want false", got, err)
	}
	for _, c := range []channelBody{public, private, guestIn} {
		for _, u := range []user{alice, bob, carol, guest, admin} {
			rest := e.call("GET", "/chat/channels/"+c.ID, u, ws, nil).Code == 200
			got, err := access.CanRead(context.Background(), ws, c.ID, u.ID)
			if err != nil || got != rest {
				t.Errorf("%s as %s: CanRead = %v, %v; REST read = %v", c.Name, u.Name, got, err, rest)
			}
		}
	}
}
