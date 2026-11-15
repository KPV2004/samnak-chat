package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// sameJSON reports whether a and b hold the same JSON value; jsonb does not
// keep the key order of what was written.
func sameJSON(a, b string) bool {
	var x, y any
	return json.Unmarshal([]byte(a), &x) == nil && json.Unmarshal([]byte(b), &y) == nil && reflect.DeepEqual(x, y)
}

// demoCard is what a module's payload looks like.
type demoCard struct {
	Status string `json:"status"`
	Votes  int    `json:"votes"`
}

func (e *env) registerDemo() {
	e.cards.Register(app.CardKind{Kind: "demo.poll", Module: "demo"})
}

func (e *env) postCard(ws, channelID, sender string, payload any) (string, error) {
	return e.cards.Post(context.Background(), app.Card{
		WorkspaceID: ws, ChannelID: channelID, SenderID: sender, Kind: "demo.poll", Payload: payload, FallbackText: "Poll: lunch?",
	})
}

func TestCardPostSendsMessageCreated(t *testing.T) {
	e := newEnv(t)
	e.registerDemo()
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "private", "team", bob.ID)
	e.rt.reset()

	id, err := e.postCard(ws, ch.ID, alice.ID, demoCard{Status: "open", Votes: 1})
	if err != nil {
		t.Fatal(err)
	}
	created := e.rt.of("message.created")
	if len(created) != 1 || len(created[0].Users) != 2 {
		t.Fatalf("message.created = %+v", created)
	}
	var ev struct {
		WorkspaceID string      `json:"workspace_id"`
		ChannelID   string      `json:"channel_id"`
		Message     messageBody `json:"message"`
	}
	if err := json.Unmarshal(created[0].Data, &ev); err != nil {
		t.Fatal(err)
	}
	m := ev.Message
	if ev.WorkspaceID != ws || m.ID != id || m.Kind != "demo.poll" || m.FallbackText != "Poll: lunch?" || m.Body != "" || m.SenderID != alice.ID || m.Seq != 1 {
		t.Errorf("event message = %+v", m)
	}
	if !sameJSON(string(m.Payload), `{"status":"open","votes":1}`) {
		t.Errorf("payload = %s", m.Payload)
	}

	// It is an ordinary message in history, counts as unread, and is published on the bus.
	page := e.messages(bob, ws, ch.ID, "")
	if len(page.Items) != 1 || page.Items[0].ID != id || page.Items[0].Kind != "demo.poll" || !sameJSON(string(page.Items[0].Payload), `{"status":"open","votes":1}`) {
		t.Errorf("history = %+v", page)
	}
	if got := e.getChannel(bob, ws, ch.ID); got.UnreadCount != 1 || got.LastSeq != 1 {
		t.Errorf("channel = %+v", got)
	}
	var published bool
	for _, p := range e.events.all() {
		published = published || p.MessageID == id
	}
	if !published {
		t.Error("chat.message_created was not published for the card")
	}

	// Cards cannot be edited as text.
	if r := e.call("PATCH", "/chat/messages/"+id, alice, ws, map[string]any{"body": "x"}); r.Code != 400 {
		t.Errorf("edit card = %d, want 400", r.Code)
	}
}

func TestCardPostRules(t *testing.T) {
	e := newEnv(t)
	e.registerDemo()
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice := e.member(ws, "alice")
	outsider := e.newUser("outsider")
	mallory := e.member(other, "mallory")
	ch := e.createChannel(alice, ws, "public", "open")
	foreign := e.createChannel(mallory, other, "public", "foreign")
	archived := e.createChannel(alice, ws, "public", "old")
	if r := e.call("PATCH", "/chat/channels/"+archived.ID, alice, ws, map[string]any{"archived": true}); r.Code != 200 {
		t.Fatal(r.Code)
	}
	two := e.createChannel(alice, ws, "public", "two")
	root := e.send(alice, ws, ch.ID, "root")
	ctx := context.Background()
	card := func(mod func(*app.Card)) error {
		c := app.Card{WorkspaceID: ws, ChannelID: ch.ID, SenderID: alice.ID, Kind: "demo.poll", Payload: demoCard{Status: "open"}, FallbackText: "Poll"}
		mod(&c)
		_, err := e.cards.Post(ctx, c)
		return err
	}
	for _, c := range []struct {
		name string
		mod  func(*app.Card)
		want error
	}{
		{"unregistered kind", func(c *app.Card) { c.Kind = "demo.other" }, app.ErrInvalid},
		{"no fallback text", func(c *app.Card) { c.FallbackText = "  " }, app.ErrInvalid},
		{"fallback text too long", func(c *app.Card) { c.FallbackText = string(make([]rune, 4001)) }, app.ErrInvalid},
		{"payload that cannot be encoded", func(c *app.Card) { c.Payload = math.Inf(1) }, app.ErrInvalid},
		{"payload with NUL", func(c *app.Card) { c.Payload = map[string]string{"a": "x\x00y"} }, app.ErrInvalid},
		{"nil payload", func(c *app.Card) { c.Payload = nil }, app.ErrInvalid},
		{"bad workspace id", func(c *app.Card) { c.WorkspaceID = "nope" }, app.ErrInvalid},
		{"bad sender id", func(c *app.Card) { c.SenderID = "nope" }, app.ErrInvalid},
		{"sender outside the workspace", func(c *app.Card) { c.SenderID = outsider.ID }, app.ErrInvalid},
		{"channel of another workspace", func(c *app.Card) { c.ChannelID = foreign.ID }, app.ErrNotFound},
		{"unknown channel", func(c *app.Card) { c.ChannelID = uuid.NewString() }, app.ErrNotFound},
		{"archived channel", func(c *app.Card) { c.ChannelID = archived.ID }, app.ErrConflict},
		{"thread root in another channel", func(c *app.Card) { c.ChannelID = two.ID; c.ThreadRootID = root.ID }, app.ErrInvalid},
		{"thread root that does not exist", func(c *app.Card) { c.ThreadRootID = uuid.NewString() }, app.ErrInvalid},
		{"payload too large", func(c *app.Card) { c.Payload = map[string]string{"a": strings.Repeat("a", 70<<10)} }, app.ErrInvalid},
	} {
		if err := card(c.mod); !errors.Is(err, c.want) {
			t.Errorf("%s = %v, want %v", c.name, err, c.want)
		}
	}
	if got := e.getChannel(alice, ws, ch.ID); got.LastSeq != 1 {
		t.Errorf("a rejected card used a seq: last_seq = %d", got.LastSeq)
	}

	// A card can be a thread reply.
	var replyID string
	var err error
	if replyID, err = e.cards.Post(ctx, app.Card{WorkspaceID: ws, ChannelID: ch.ID, SenderID: alice.ID, Kind: "demo.poll", Payload: demoCard{}, FallbackText: "Poll", ThreadRootID: root.ID}); err != nil {
		t.Fatal(err)
	}
	if got := e.messages(alice, ws, ch.ID, "?thread_root_id="+root.ID).Items; len(got) != 1 || got[0].ID != replyID {
		t.Errorf("thread = %+v", got)
	}
}

func TestCardUpdateSendsCardUpdated(t *testing.T) {
	e := newEnv(t)
	e.registerDemo()
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	ch := e.createChannel(alice, ws, "private", "team", bob.ID)
	id, err := e.postCard(ws, ch.ID, alice.ID, demoCard{Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	e.rt.reset()
	ctx := context.Background()

	if err := e.cards.Update(ctx, ws, id, demoCard{Status: "closed", Votes: 3}); err != nil {
		t.Fatal(err)
	}
	got := e.rt.of("card.updated")
	if len(got) != 1 {
		t.Fatalf("card.updated deliveries = %d", len(got))
	}
	users := slices.Clone(got[0].Users)
	slices.Sort(users)
	want := []string{alice.ID, bob.ID}
	slices.Sort(want)
	if !slices.Equal(users, want) || slices.Contains(users, carol.ID) {
		t.Errorf("recipients = %v, want %v", users, want)
	}
	var data struct {
		WorkspaceID string          `json:"workspace_id"`
		ChannelID   string          `json:"channel_id"`
		MessageID   string          `json:"message_id"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(got[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.WorkspaceID != ws || data.ChannelID != ch.ID || data.MessageID != id || !sameJSON(string(data.Payload), `{"status":"closed","votes":3}`) {
		t.Errorf("card.updated data = %s", got[0].Data)
	}
	if m := e.messages(bob, ws, ch.ID, "").Items[0]; !sameJSON(string(m.Payload), `{"status":"closed","votes":3}`) || m.FallbackText != "Poll: lunch?" {
		t.Errorf("stored card = %+v", m)
	}
	if len(e.rt.of("message.updated")) != 0 {
		t.Error("a card update must not send message.updated")
	}

	// Only cards of a registered kind in this workspace can be updated.
	text := e.send(alice, ws, ch.ID, "plain")
	if err := e.cards.Update(ctx, ws, text.ID, demoCard{}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("update a text message = %v, want ErrInvalid", err)
	}
	if err := e.cards.Update(ctx, other, id, demoCard{}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("update from another workspace = %v, want ErrNotFound", err)
	}
	if err := e.cards.Update(ctx, ws, uuid.NewString(), demoCard{}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("update unknown = %v, want ErrNotFound", err)
	}
	if err := e.cards.Update(ctx, ws, id, nil); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("update with nil payload = %v, want ErrInvalid", err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE chat_messages SET kind = 'ghost.card' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.cards.Update(ctx, ws, id, demoCard{}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("update an unregistered kind = %v, want ErrInvalid", err)
	}

	// PLT-11: a kind nobody registered still reaches clients, with its
	// payload and the fallback text to show.
	m := e.messages(bob, ws, ch.ID, "").Items[0]
	if m.Kind != "ghost.card" || m.FallbackText != "Poll: lunch?" || !sameJSON(string(m.Payload), `{"status":"closed","votes":3}`) {
		t.Errorf("unregistered kind as clients see it = %+v", m)
	}
	if r := e.call("DELETE", "/chat/messages/"+id, alice, ws, nil); r.Code != 204 {
		t.Fatal(r.Code)
	}
	if err := e.cards.Update(ctx, ws, id, demoCard{}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("update a deleted card = %v, want ErrNotFound", err)
	}
}

func TestCardDirectChannelReusesDMRules(t *testing.T) {
	e := newEnv(t)
	e.registerDemo()
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	outsider := e.newUser("outsider")
	ctx := context.Background()
	e.rt.reset()

	id, err := e.cards.DirectChannel(ctx, ws, []string{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.rt.of("channel.updated"); len(got) != 1 || len(got[0].Users) != 2 {
		t.Errorf("channel.updated for a new DM = %+v", got)
	}
	again, err := e.cards.DirectChannel(ctx, ws, []string{bob.ID, alice.ID, bob.ID})
	if err != nil || again != id {
		t.Errorf("same people = %q, %v; want %q", again, err, id)
	}
	// The REST endpoint finds the same channel.
	r := e.call("POST", "/chat/dms", alice, ws, map[string]any{"user_ids": []string{bob.ID}})
	var viaAPI channelBody
	r.json(t, &viaAPI)
	if r.Code != 200 || viaAPI.ID != id || viaAPI.Type != "dm" {
		t.Errorf("POST /chat/dms = %d %s, want the DM %s", r.Code, r.Body, id)
	}
	group, err := e.cards.DirectChannel(ctx, ws, []string{alice.ID, bob.ID, carol.ID})
	if err != nil || group == id {
		t.Errorf("group DM = %q, %v", group, err)
	}

	for name, users := range map[string][]string{
		"nobody":                 nil,
		"one person":             {alice.ID},
		"the same person":        {alice.ID, alice.ID},
		"not a workspace member": {alice.ID, outsider.ID},
		"bad id":                 {alice.ID, "nope"},
	} {
		if _, err := e.cards.DirectChannel(ctx, ws, users); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := e.cards.DirectChannel(ctx, other, []string{alice.ID, bob.ID}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("users of another workspace = %v, want ErrInvalid", err)
	}

	// A card to a person: post into their DM.
	if _, err := e.postCard(ws, id, alice.ID, demoCard{Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if got := e.getChannel(bob, ws, id); got.UnreadCount != 1 {
		t.Errorf("bob's DM = %+v", got)
	}
}
