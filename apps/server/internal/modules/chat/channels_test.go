package chat_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
)

func ids(list []channelBody) []string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = c.ID
	}
	return out
}

func (e *env) listChannels(u user, ws, query string) []channelBody {
	e.t.Helper()
	r := e.call("GET", "/chat/channels"+query, u, ws, nil)
	if r.Code != 200 {
		e.t.Fatalf("list channels = %d %s", r.Code, r.Body)
	}
	var b struct {
		Items []channelBody `json:"items"`
	}
	r.json(e.t, &b)
	return b.Items
}

func TestCreateChannelAndList(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")

	pub := e.createChannel(alice, ws, "public", "general")
	if pub.Type != "public" || pub.Name != "general" || pub.MemberCount != 1 || pub.LastSeq != 0 {
		t.Errorf("created channel = %+v", pub)
	}
	// A private channel with bob in it.
	priv := e.createChannel(alice, ws, "private", "secret", bob.ID)
	if priv.MemberCount != 2 {
		t.Errorf("member_count = %d, want 2", priv.MemberCount)
	}

	if got := ids(e.listChannels(alice, ws, "")); !slices.Contains(got, pub.ID) || !slices.Contains(got, priv.ID) {
		t.Errorf("alice joined = %v, want both channels", got)
	}
	got := ids(e.listChannels(bob, ws, ""))
	if slices.Contains(got, pub.ID) || !slices.Contains(got, priv.ID) {
		t.Errorf("bob joined = %v, want only the private channel he was added to", got)
	}
	if got := ids(e.listChannels(bob, ws, "?scope=public")); !slices.Contains(got, pub.ID) || slices.Contains(got, priv.ID) {
		t.Errorf("bob public = %v, want the public channel only", got)
	}
}

func TestCreateChannelValidation(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	outsider := e.newUser("outsider")
	e.createChannel(alice, ws, "public", "Dupe")

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"empty name", map[string]any{"type": "public", "name": "  "}, 400},
		{"long name", map[string]any{"type": "public", "name": strings.Repeat("a", 81)}, 400},
		{"bad type", map[string]any{"type": "dm", "name": "x"}, 400},
		{"duplicate name, any case", map[string]any{"type": "private", "name": "dupe"}, 409},
		{"member who is not in the workspace", map[string]any{"type": "public", "name": "ok1", "member_ids": []string{outsider.ID}}, 400},
		{"malformed member id", map[string]any{"type": "public", "name": "ok2", "member_ids": []string{"nope"}}, 400},
	}
	for _, tt := range tests {
		if r := e.call("POST", "/chat/channels", alice, ws, tt.body); r.Code != tt.want {
			t.Errorf("%s: status = %d %s, want %d", tt.name, r.Code, r.Body, tt.want)
		}
	}
}

func TestGuestCannotCreateChannels(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	guest := e.newUser("guest")
	e.join(ws, guest, app.RoleGuest)
	if r := e.call("POST", "/chat/channels", guest, ws, map[string]any{"type": "public", "name": "x"}); r.Code != 403 {
		t.Errorf("guest create = %d, want 403", r.Code)
	}
}

func TestVisibilityRules(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	admin := e.newUser("admin")
	e.join(ws, admin, app.RoleAdmin)
	guest := e.newUser("guest")
	e.join(ws, guest, app.RoleGuest)

	pub := e.createChannel(alice, ws, "public", "open")
	priv := e.createChannel(alice, ws, "private", "closed", bob.ID)
	dm := openDM(t, e, alice, ws, bob.ID)

	// Private channels and DMs: 404 for everyone else, even admins, on every operation.
	for _, u := range []user{carol, admin, guest} {
		for _, ch := range []channelBody{priv, dm} {
			for _, c := range []struct{ method, path string }{
				{"GET", "/chat/channels/" + ch.ID},
				{"GET", "/chat/channels/" + ch.ID + "/members"},
				{"GET", "/chat/channels/" + ch.ID + "/messages"},
				{"POST", "/chat/channels/" + ch.ID + "/join"},
				{"POST", "/chat/channels/" + ch.ID + "/leave"},
			} {
				if r := e.call(c.method, c.path, u, ws, nil); r.Code != 404 {
					t.Errorf("%s %s %s = %d, want 404", u.Name, c.method, c.path, r.Code)
				}
			}
			r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", u, ws, map[string]any{"client_msg_id": "0192a3b4-0000-7000-8000-000000000001", "body": "hi"})
			if r.Code != 404 {
				t.Errorf("%s send to %s = %d, want 404", u.Name, ch.Type, r.Code)
			}
			r = e.call("POST", "/chat/channels/"+ch.ID+"/members", u, ws, map[string]any{"user_ids": []string{u.ID}})
			if r.Code != 404 {
				t.Errorf("%s add members to %s = %d, want 404", u.Name, ch.Type, r.Code)
			}
		}
		// And they do not show up in listings.
		for _, q := range []string{"", "?scope=public"} {
			for _, c := range e.listChannels(u, ws, q) {
				if c.ID == priv.ID || c.ID == dm.ID {
					t.Errorf("%s sees %s in list %q", u.Name, c.Type, q)
				}
			}
		}
	}

	// Public channels: members read and join; guests do not see them until added.
	if r := e.call("GET", "/chat/channels/"+pub.ID, carol, ws, nil); r.Code != 200 {
		t.Errorf("member reads public = %d", r.Code)
	}
	if r := e.call("GET", "/chat/channels/"+pub.ID+"/messages", carol, ws, nil); r.Code != 200 {
		t.Errorf("member reads public messages = %d", r.Code)
	}
	if r := e.call("GET", "/chat/channels/"+pub.ID, guest, ws, nil); r.Code != 404 {
		t.Errorf("guest reads public = %d, want 404", r.Code)
	}
	if r := e.call("POST", "/chat/channels/"+pub.ID+"/join", guest, ws, nil); r.Code != 404 {
		t.Errorf("guest joins public = %d, want 404", r.Code)
	}
	if got := e.listChannels(guest, ws, "?scope=public"); len(got) != 0 {
		t.Errorf("guest public list = %v, want empty", got)
	}
	// Reading without joining is allowed, but writing is not.
	r := e.call("POST", "/chat/channels/"+pub.ID+"/messages", carol, ws, map[string]any{"client_msg_id": "0192a3b4-0000-7000-8000-000000000002", "body": "hi"})
	if r.Code != 403 {
		t.Errorf("send without joining = %d, want 403", r.Code)
	}
	// Once added, a guest sees exactly that channel.
	if r := e.call("POST", "/chat/channels/"+pub.ID+"/members", alice, ws, map[string]any{"user_ids": []string{guest.ID}}); r.Code != 204 {
		t.Fatalf("add guest = %d %s", r.Code, r.Body)
	}
	if got := ids(e.listChannels(guest, ws, "")); !slices.Equal(got, []string{pub.ID}) {
		t.Errorf("guest joined = %v, want only %s", got, pub.ID)
	}
	if r := e.call("GET", "/chat/channels/"+pub.ID, guest, ws, nil); r.Code != 200 {
		t.Errorf("added guest reads = %d", r.Code)
	}
}

func TestChannelsAreIsolatedBetweenWorkspaces(t *testing.T) {
	e := newEnv(t)
	wsA, wsB := e.newWorkspace(), e.newWorkspace()
	alice := e.member(wsA, "alice")
	mallory := e.member(wsB, "mallory")
	e.join(wsA, mallory, app.RoleMember) // mallory is in both workspaces

	chA := e.createChannel(alice, wsA, "public", "a-general")
	msg := e.send(alice, wsA, chA.ID, "secret of A")
	chB := e.createChannel(mallory, wsB, "public", "b-general")

	// Listing in B never shows A's channels, and a-general is not found through B.
	for _, q := range []string{"", "?scope=public"} {
		if got := ids(e.listChannels(mallory, wsB, q)); slices.Contains(got, chA.ID) {
			t.Errorf("workspace B lists A's channel (%q)", q)
		}
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/chat/channels/" + chA.ID},
		{"GET", "/chat/channels/" + chA.ID + "/messages"},
		{"GET", "/chat/channels/" + chA.ID + "/members"},
		{"POST", "/chat/channels/" + chA.ID + "/join"},
		{"PATCH", "/chat/messages/" + msg.ID},
		{"DELETE", "/chat/messages/" + msg.ID},
	} {
		var body any
		if c.method == "PATCH" {
			body = map[string]any{"body": "x"}
		}
		if r := e.call(c.method, c.path, mallory, wsB, body); r.Code != 404 {
			t.Errorf("%s %s through workspace B = %d, want 404", c.method, c.path, r.Code)
		}
	}
	if r := e.call("POST", "/chat/channels/"+chA.ID+"/messages", mallory, wsB, map[string]any{"client_msg_id": "0192a3b4-0000-7000-8000-000000000003", "body": "x"}); r.Code != 404 {
		t.Errorf("send into A through B = %d, want 404", r.Code)
	}
	// Same channel name in both workspaces is fine.
	e.createChannel(mallory, wsB, "public", "a-general")
	_ = chB

	// A member of only A cannot reach B at all.
	if r := e.call("GET", "/chat/channels", alice, wsB, nil); r.Code != 403 {
		t.Errorf("alice in workspace B = %d, want 403", r.Code)
	}
}

func openDM(t *testing.T, e *env, u user, ws string, others ...string) channelBody {
	t.Helper()
	r := e.call("POST", "/chat/dms", u, ws, map[string]any{"user_ids": others})
	if r.Code != 200 && r.Code != 201 {
		t.Fatalf("open dm = %d %s", r.Code, r.Body)
	}
	var c channelBody
	r.json(t, &c)
	return c
}

func TestDirectMessages(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	outsider := e.newUser("outsider")

	r := e.call("POST", "/chat/dms", alice, ws, map[string]any{"user_ids": []string{bob.ID}})
	if r.Code != 201 {
		t.Fatalf("first open = %d %s", r.Code, r.Body)
	}
	var first channelBody
	r.json(t, &first)
	if first.Type != "dm" || first.Name != "" || first.MemberCount != 2 || len(first.DMUserIDs) != 2 {
		t.Errorf("dm = %+v", first)
	}

	// The same set from either side returns the same channel with 200.
	for _, c := range []struct {
		u      user
		others []string
	}{{alice, []string{bob.ID}}, {bob, []string{alice.ID}}, {alice, []string{bob.ID, bob.ID, alice.ID}}} {
		r := e.call("POST", "/chat/dms", c.u, ws, map[string]any{"user_ids": c.others})
		var again channelBody
		r.json(t, &again)
		if r.Code != 200 || again.ID != first.ID {
			t.Errorf("reopen as %s = %d, id %s want %s", c.u.Name, r.Code, again.ID, first.ID)
		}
	}

	// A group DM is its own channel, and the same group again is that channel.
	group := openDM(t, e, alice, ws, bob.ID, carol.ID)
	if group.ID == first.ID || group.MemberCount != 3 {
		t.Errorf("group dm = %+v", group)
	}
	if again := openDM(t, e, carol, ws, alice.ID, bob.ID); again.ID != group.ID {
		t.Errorf("group reopened as %s, want %s", again.ID, group.ID)
	}

	for name, body := range map[string]any{
		"with nobody":       map[string]any{"user_ids": []string{}},
		"only yourself":     map[string]any{"user_ids": []string{alice.ID}},
		"outside workspace": map[string]any{"user_ids": []string{outsider.ID}},
		"not a user id":     map[string]any{"user_ids": []string{"nope"}},
		"nine other people": map[string]any{"user_ids": slices.Repeat([]string{bob.ID}, 9)},
	} {
		if r := e.call("POST", "/chat/dms", alice, ws, body); r.Code != 400 {
			t.Errorf("dm %s = %d %s, want 400", name, r.Code, r.Body)
		}
	}

	// A DM cannot be left, renamed or extended.
	if r := e.call("POST", "/chat/channels/"+first.ID+"/leave", alice, ws, nil); r.Code != 400 {
		t.Errorf("leave dm = %d, want 400", r.Code)
	}
	if r := e.call("PATCH", "/chat/channels/"+first.ID, alice, ws, map[string]any{"name": "x"}); r.Code != 400 {
		t.Errorf("rename dm = %d, want 400", r.Code)
	}
	if r := e.call("POST", "/chat/channels/"+first.ID+"/members", alice, ws, map[string]any{"user_ids": []string{carol.ID}}); r.Code != 400 {
		t.Errorf("add to dm = %d, want 400", r.Code)
	}
}

func TestUpdateChannelAndArchive(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	admin := e.newUser("admin")
	e.join(ws, admin, app.RoleAdmin)
	e.createChannel(alice, ws, "public", "taken")
	ch := e.createChannel(alice, ws, "public", "ideas")

	// bob is not the creator and not an admin.
	if r := e.call("PATCH", "/chat/channels/"+ch.ID, bob, ws, map[string]any{"topic": "mine"}); r.Code != 403 {
		t.Errorf("member patch = %d, want 403", r.Code)
	}
	r := e.call("PATCH", "/chat/channels/"+ch.ID, alice, ws, map[string]any{"name": "ideas-2", "topic": "  brainstorm "})
	var got channelBody
	r.json(t, &got)
	if r.Code != 200 || got.Name != "ideas-2" || got.Topic != "brainstorm" {
		t.Errorf("creator patch = %d %+v", r.Code, got)
	}
	if r := e.call("PATCH", "/chat/channels/"+ch.ID, alice, ws, map[string]any{"name": "TAKEN"}); r.Code != 409 {
		t.Errorf("rename to taken name = %d, want 409", r.Code)
	}
	if r := e.call("PATCH", "/chat/channels/"+ch.ID, alice, ws, map[string]any{"name": ""}); r.Code != 400 {
		t.Errorf("empty name = %d, want 400", r.Code)
	}

	// An admin can archive a public channel they have not joined.
	r = e.call("PATCH", "/chat/channels/"+ch.ID, admin, ws, map[string]any{"archived": true})
	r.json(t, &got)
	if r.Code != 200 || got.ArchivedAt == nil {
		t.Fatalf("admin archive = %d %s", r.Code, r.Body)
	}
	// Archived: no new messages, no joining, hidden from the public list but kept in joined.
	body := map[string]any{"client_msg_id": "0192a3b4-0000-7000-8000-000000000004", "body": "late"}
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", alice, ws, body); r.Code != 409 {
		t.Errorf("send to archived = %d, want 409", r.Code)
	}
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/join", bob, ws, nil); r.Code != 409 {
		t.Errorf("join archived = %d, want 409", r.Code)
	}
	if slices.Contains(ids(e.listChannels(bob, ws, "?scope=public")), ch.ID) {
		t.Error("archived channel is in the public list")
	}
	if !slices.Contains(ids(e.listChannels(alice, ws, "")), ch.ID) {
		t.Error("archived channel is missing from the joined list")
	}
	// Restoring it re-opens it.
	r = e.call("PATCH", "/chat/channels/"+ch.ID, alice, ws, map[string]any{"archived": false})
	r.json(t, &got)
	if got.ArchivedAt != nil {
		t.Errorf("restored channel still archived: %+v", got)
	}
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", alice, ws, body); r.Code != 201 {
		t.Errorf("send after restore = %d, want 201", r.Code)
	}
}

func TestChannelMembersCarryAvatars(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "town-hall")
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/join", bob, ws, nil); r.Code != 204 {
		t.Fatalf("join = %d %s", r.Code, r.Body)
	}
	const avatar = "https://cdn.example.com/bob.png"
	e.dir.mu.Lock()
	e.dir.users[bob.ID] = auth.UserSummary{ID: bob.ID, DisplayName: "bob", AvatarURL: avatar}
	e.dir.mu.Unlock()

	r := e.call("GET", "/chat/channels/"+ch.ID+"/members", alice, ws, nil)
	var members struct {
		Items []struct {
			UserID    string `json:"user_id"`
			AvatarURL string `json:"avatar_url"`
		} `json:"items"`
	}
	r.json(t, &members)
	if r.Code != 200 || len(members.Items) != 2 {
		t.Fatalf("members = %d %s", r.Code, r.Body)
	}
	avatars := map[string]string{}
	for _, m := range members.Items {
		avatars[m.UserID] = m.AvatarURL
	}
	if avatars[bob.ID] != avatar || avatars[alice.ID] != "" {
		t.Errorf("avatars = %v", avatars)
	}
	if !strings.Contains(string(r.Body), `"avatar_url":""`) {
		t.Errorf("no empty avatar_url in %s", r.Body)
	}
}

func TestJoinLeaveAndMembers(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	outsider := e.newUser("outsider")
	ch := e.createChannel(alice, ws, "public", "town-hall")

	for i := 0; i < 2; i++ { // joining twice is fine
		if r := e.call("POST", "/chat/channels/"+ch.ID+"/join", bob, ws, nil); r.Code != 204 {
			t.Fatalf("join #%d = %d %s", i, r.Code, r.Body)
		}
	}
	if got := e.getChannel(bob, ws, ch.ID); got.MemberCount != 2 {
		t.Errorf("member_count = %d, want 2", got.MemberCount)
	}

	r := e.call("GET", "/chat/channels/"+ch.ID+"/members", bob, ws, nil)
	var members struct {
		Items []struct {
			UserID      string `json:"user_id"`
			DisplayName string `json:"display_name"`
		} `json:"items"`
	}
	r.json(t, &members)
	if r.Code != 200 || len(members.Items) != 2 {
		t.Fatalf("members = %d %s", r.Code, r.Body)
	}
	names := map[string]string{}
	for _, m := range members.Items {
		names[m.UserID] = m.DisplayName
	}
	if names[alice.ID] != "alice" || names[bob.ID] != "bob" {
		t.Errorf("display names = %v", names)
	}

	// Members add others; people outside the workspace cannot be added.
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/members", bob, ws, map[string]any{"user_ids": []string{carol.ID}}); r.Code != 204 {
		t.Errorf("add carol = %d %s", r.Code, r.Body)
	}
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/members", bob, ws, map[string]any{"user_ids": []string{outsider.ID}}); r.Code != 400 {
		t.Errorf("add outsider = %d, want 400", r.Code)
	}
	// Someone who has not joined cannot add people.
	dave := e.member(ws, "dave")
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/members", dave, ws, map[string]any{"user_ids": []string{dave.ID}}); r.Code != 403 {
		t.Errorf("add by non-member = %d, want 403", r.Code)
	}

	for i := 0; i < 2; i++ { // leaving twice is fine
		if r := e.call("POST", "/chat/channels/"+ch.ID+"/leave", bob, ws, nil); r.Code != 204 {
			t.Fatalf("leave #%d = %d %s", i, r.Code, r.Body)
		}
	}
	if slices.Contains(ids(e.listChannels(bob, ws, "")), ch.ID) {
		t.Error("channel still in bob's joined list after leaving")
	}
	if got := e.getChannel(alice, ws, ch.ID); got.MemberCount != 2 {
		t.Errorf("member_count after leave = %d, want 2", got.MemberCount)
	}
}

func TestMemberRemovedLeavesChannels(t *testing.T) {
	e := newEnv(t)
	ws, otherWS := e.newWorkspace(), e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	e.join(otherWS, bob, app.RoleMember)
	pub := e.createChannel(alice, ws, "public", "all-hands", bob.ID)
	priv := e.createChannel(alice, ws, "private", "leads", bob.ID)
	other := e.createChannel(bob, otherWS, "public", "elsewhere")
	e.rt.reset()

	evt := workspaceMemberRemoved(ws, bob.ID, alice.ID)
	if err := e.bus.Publish(t.Context(), evt); err != nil {
		t.Fatal(err)
	}
	// Run twice: delivery is at least once.
	if err := e.bus.Publish(t.Context(), evt); err != nil {
		t.Fatal(err)
	}

	for _, ch := range []channelBody{pub, priv} {
		if got := e.getChannel(alice, ws, ch.ID); got.MemberCount != 1 {
			t.Errorf("%s member_count = %d, want 1", ch.Name, got.MemberCount)
		}
	}
	if !slices.Contains(ids(e.listChannels(bob, otherWS, "")), other.ID) {
		t.Error("bob lost a channel in another workspace")
	}
	// The remaining member is told about the new member count.
	var told int
	for _, d := range e.rt.of("channel.updated") {
		if slices.Contains(d.Users, alice.ID) {
			told++
		}
	}
	if told < 2 {
		t.Errorf("alice got %d channel.updated, want at least 2", told)
	}
}
