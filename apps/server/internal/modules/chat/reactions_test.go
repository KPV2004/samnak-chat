package chat_test

import (
	"encoding/json"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func (e *env) react(u user, ws, messageID, emoji string) resp {
	e.t.Helper()
	return e.call("POST", "/chat/messages/"+messageID+"/reactions", u, ws, map[string]any{"emoji": emoji})
}

func (e *env) unreact(u user, ws, messageID, emoji string) resp {
	e.t.Helper()
	return e.call("DELETE", "/chat/messages/"+messageID+"/reactions/"+url.PathEscape(emoji), u, ws, nil)
}

// reactionsOf reads a message's reactions as u sees them.
func (e *env) reactionsOf(u user, ws, channelID, messageID string) []reactionBody {
	e.t.Helper()
	for _, m := range e.messages(u, ws, channelID, "").Items {
		if m.ID == messageID {
			return m.Reactions
		}
	}
	e.t.Fatalf("message %s not listed", messageID)
	return nil
}

func TestReactionsAddRemoveAreIdempotent(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	ch := e.createChannel(alice, ws, "private", "team", bob.ID)
	m := e.send(alice, ws, ch.ID, "hello")
	if len(m.Reactions) != 0 {
		t.Fatalf("new message has reactions: %v", m.Reactions)
	}
	e.rt.reset()

	if r := e.react(bob, ws, m.ID, "👍"); r.Code != 204 {
		t.Fatalf("react = %d %s", r.Code, r.Body)
	}
	if r := e.react(bob, ws, m.ID, "👍"); r.Code != 204 {
		t.Fatalf("react again = %d %s", r.Code, r.Body)
	}
	if got := len(e.rt.of("message.updated")); got != 2 { // one per member, for the first add only
		t.Errorf("message.updated deliveries after two identical adds = %d, want 2", got)
	}
	if r := e.react(alice, ws, m.ID, "👍"); r.Code != 204 {
		t.Fatalf("alice react = %d %s", r.Code, r.Body)
	}
	if r := e.react(alice, ws, m.ID, "🎉"); r.Code != 204 {
		t.Fatalf("alice react 2 = %d %s", r.Code, r.Body)
	}

	if got := e.reactionsOf(alice, ws, ch.ID, m.ID); !slices.Equal(got, []reactionBody{{"👍", 2, true}, {"🎉", 1, true}}) {
		t.Errorf("alice sees %v", got)
	}
	if got := e.reactionsOf(bob, ws, ch.ID, m.ID); !slices.Equal(got, []reactionBody{{"👍", 2, true}, {"🎉", 1, false}}) {
		t.Errorf("bob sees %v", got)
	}

	// The event carries the reactions as each recipient sees them.
	e.rt.reset()
	if r := e.react(alice, ws, m.ID, "🔥"); r.Code != 204 {
		t.Fatal(r.Code)
	}
	seen := map[string][]reactionBody{}
	for _, d := range e.rt.of("message.updated") {
		var data struct {
			Message messageBody `json:"message"`
		}
		if err := json.Unmarshal(d.Data, &data); err != nil {
			t.Fatal(err)
		}
		if len(d.Users) != 1 || data.Message.ID != m.ID {
			t.Fatalf("delivery = users %v message %s", d.Users, data.Message.ID)
		}
		seen[d.Users[0]] = data.Message.Reactions
	}
	if len(seen) != 2 || seen[carol.ID] != nil {
		t.Fatalf("delivered to %v", seen)
	}
	if !slices.Contains(seen[alice.ID], reactionBody{"🔥", 1, true}) || !slices.Contains(seen[bob.ID], reactionBody{"🔥", 1, false}) {
		t.Errorf("per-recipient me wrong: alice %v bob %v", seen[alice.ID], seen[bob.ID])
	}

	// Removing is idempotent and only announces a real change.
	e.rt.reset()
	if r := e.unreact(alice, ws, m.ID, "🔥"); r.Code != 204 {
		t.Fatalf("unreact = %d %s", r.Code, r.Body)
	}
	if r := e.unreact(alice, ws, m.ID, "🔥"); r.Code != 204 {
		t.Fatalf("unreact again = %d %s", r.Code, r.Body)
	}
	if r := e.unreact(alice, ws, m.ID, "🚀"); r.Code != 204 {
		t.Fatalf("unreact something never added = %d %s", r.Code, r.Body)
	}
	if got := len(e.rt.of("message.updated")); got != 2 {
		t.Errorf("message.updated deliveries after removing = %d, want 2", got)
	}
	if got := e.reactionsOf(alice, ws, ch.ID, m.ID); len(got) != 2 {
		t.Errorf("after remove: %v", got)
	}
}

func TestReactionEmojiInPathIsDecoded(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "open")
	m := e.send(alice, ws, ch.ID, "hi")
	for _, emoji := range []string{"👨‍👩‍👧", "❤️", "🇹🇭"} {
		if r := e.react(alice, ws, m.ID, emoji); r.Code != 204 {
			t.Fatalf("react %q = %d %s", emoji, r.Code, r.Body)
		}
		if r := e.unreact(alice, ws, m.ID, emoji); r.Code != 204 {
			t.Fatalf("unreact %q = %d %s", emoji, r.Code, r.Body)
		}
	}
	if got := e.reactionsOf(alice, ws, ch.ID, m.ID); len(got) != 0 {
		t.Errorf("reactions left: %v", got)
	}
}

func TestReactionRules(t *testing.T) {
	e := newEnv(t)
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	mallory := e.member(other, "mallory")
	private := e.createChannel(alice, ws, "private", "secret")
	public := e.createChannel(alice, ws, "public", "open")
	pm := e.send(alice, ws, private.ID, "private message")
	om := e.send(alice, ws, public.ID, "public message")

	for _, c := range []struct {
		name string
		r    resp
		code int
	}{
		{"not an emoji", e.react(alice, ws, om.ID, "thumbsup"), 400},
		{"empty emoji", e.react(alice, ws, om.ID, ""), 400},
		{"private message, not a member", e.react(bob, ws, pm.ID, "👍"), 404},
		{"public channel, not joined", e.react(bob, ws, om.ID, "👍"), 403},
		{"other workspace", e.react(mallory, other, pm.ID, "👍"), 404},
		{"unknown message", e.react(alice, ws, uuid.NewString(), "👍"), 404},
		{"malformed id", e.react(alice, ws, "nope", "👍"), 404},
		{"unreact in other workspace", e.unreact(mallory, other, om.ID, "👍"), 404},
		{"unreact, not joined", e.unreact(bob, ws, om.ID, "👍"), 403},
	} {
		if c.r.Code != c.code {
			t.Errorf("%s = %d %s, want %d", c.name, c.r.Code, c.r.Body, c.code)
		}
	}
	if got := e.reactionsOf(alice, ws, public.ID, om.ID); len(got) != 0 {
		t.Errorf("a rejected reaction was stored: %v", got)
	}

	// A deleted message cannot be reacted to.
	if r := e.call("DELETE", "/chat/messages/"+om.ID, alice, ws, nil); r.Code != 204 {
		t.Fatal(r.Code)
	}
	if r := e.react(alice, ws, om.ID, "👍"); r.Code != 404 {
		t.Errorf("react to deleted = %d, want 404", r.Code)
	}
}

func TestReactionKindsAreCapped(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "open")
	m := e.send(alice, ws, ch.ID, "hi")
	emojis := []string{"😀", "😁", "😂", "😃", "😄", "😅", "😆", "😇", "😈", "😉", "😊", "😋", "😌", "😍", "😎", "😏", "😐", "😑", "😒", "😓"}
	for _, em := range emojis {
		if r := e.react(alice, ws, m.ID, em); r.Code != 204 {
			t.Fatalf("react %q = %d %s", em, r.Code, r.Body)
		}
	}
	if r := e.react(alice, ws, m.ID, "😔"); r.Code != 400 {
		t.Errorf("21st kind = %d %s, want 400", r.Code, r.Body)
	}
	if r := e.react(alice, ws, m.ID, "😀"); r.Code != 204 {
		t.Errorf("an existing kind after the cap = %d, want 204", r.Code)
	}
}

func TestDeletedMessageHidesReactions(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "open")
	m := e.send(alice, ws, ch.ID, "hi")
	e.react(alice, ws, m.ID, "👍")
	if r := e.call("DELETE", "/chat/messages/"+m.ID, alice, ws, nil); r.Code != 204 {
		t.Fatal(r.Code)
	}
	if got := e.reactionsOf(alice, ws, ch.ID, m.ID); len(got) != 0 {
		t.Errorf("deleted message shows reactions: %v", got)
	}
}
