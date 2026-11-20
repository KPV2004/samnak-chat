package chat_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// pinCall sends PUT (pin) or DELETE (unpin) for a channel as u.
func (e *env) pinCall(method string, u user, ws, channelID string) resp {
	e.t.Helper()
	return e.call(method, "/chat/channels/"+channelID+"/pin", u, ws, nil)
}

func TestPinAndUnpin(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "general")
	if ch.PinnedAt != nil {
		t.Fatalf("new channel pinned_at = %s, want null", *ch.PinnedAt)
	}

	if r := e.pinCall("PUT", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("pin = %d %s", r.Code, r.Body)
	}
	pinned := e.getChannel(alice, ws, ch.ID)
	if pinned.PinnedAt == nil {
		t.Fatal("pinned_at is null after pin")
	}

	// Pinning again is fine and keeps the first time: it does not move the
	// channel to the top again.
	e.rt.reset()
	if r := e.pinCall("PUT", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("second pin = %d %s", r.Code, r.Body)
	}
	if again := e.getChannel(alice, ws, ch.ID); again.PinnedAt == nil || *again.PinnedAt != *pinned.PinnedAt {
		t.Errorf("pinned_at after a second pin = %v, want %s", again.PinnedAt, *pinned.PinnedAt)
	}
	if n := len(e.rt.of("channel.pin")); n != 0 {
		t.Errorf("channel.pin sent %d times for a no-op pin", n)
	}

	if r := e.pinCall("DELETE", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("unpin = %d %s", r.Code, r.Body)
	}
	if got := e.getChannel(alice, ws, ch.ID); got.PinnedAt != nil {
		t.Errorf("pinned_at after unpin = %s, want null", *got.PinnedAt)
	}
	// Unpinning what is not pinned is fine too.
	e.rt.reset()
	if r := e.pinCall("DELETE", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("second unpin = %d %s", r.Code, r.Body)
	}
	if n := len(e.rt.of("channel.pin")); n != 0 {
		t.Errorf("channel.pin sent %d times for a no-op unpin", n)
	}
}

func TestPinNeedsMembership(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	open := e.createChannel(alice, ws, "public", "open")
	closed := e.createChannel(alice, ws, "private", "closed")
	missing := uuid.Must(uuid.NewV7()).String()

	// bob is in the workspace but not in either channel: 404 for every
	// channel, public or not, and for a channel that does not exist.
	for _, id := range []string{open.ID, closed.ID, missing} {
		for _, method := range []string{"PUT", "DELETE"} {
			if r := e.pinCall(method, bob, ws, id); r.Code != 404 {
				t.Errorf("bob %s pin of %s = %d %s, want 404", method, id, r.Code, r.Body)
			}
		}
	}
	if got := e.getChannel(alice, ws, open.ID); got.PinnedAt != nil {
		t.Errorf("a refused pin still pinned the channel: %s", *got.PinnedAt)
	}
}

func TestPinIsRefusedInAnotherWorkspace(t *testing.T) {
	e := newEnv(t)
	wsA, wsB := e.newWorkspace(), e.newWorkspace()
	alice := e.member(wsA, "alice")
	e.join(wsB, alice, app.RoleMember)
	ch := e.createChannel(alice, wsA, "public", "general")

	// alice is a member of wsB too, but the channel belongs to wsA.
	for _, method := range []string{"PUT", "DELETE"} {
		if r := e.pinCall(method, alice, wsB, ch.ID); r.Code != 404 {
			t.Errorf("%s pin from another workspace = %d %s, want 404", method, r.Code, r.Body)
		}
	}
	if got := e.getChannel(alice, wsA, ch.ID); got.PinnedAt != nil {
		t.Errorf("a pin from another workspace went through: %s", *got.PinnedAt)
	}
}

func TestPinLimitIsThirtyPerUserAndWorkspace(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	bob := e.member(ws, "bob")

	var chans []channelBody
	for i := range 31 {
		chans = append(chans, e.createChannel(alice, ws, "public", fmt.Sprintf("room-%02d", i)))
	}
	for _, c := range chans[:30] {
		if r := e.pinCall("PUT", alice, ws, c.ID); r.Code != 204 {
			t.Fatalf("pin %s = %d %s, want 204", c.Name, r.Code, r.Body)
		}
	}
	if r := e.pinCall("PUT", alice, ws, chans[30].ID); r.Code != 409 {
		t.Fatalf("31st pin = %d %s, want 409", r.Code, r.Body)
	}
	if got := e.getChannel(alice, ws, chans[30].ID); got.PinnedAt != nil {
		t.Errorf("the refused 31st pin still pinned: %s", *got.PinnedAt)
	}

	// Pinning one of the thirty again is not a new pin.
	if r := e.pinCall("PUT", alice, ws, chans[0].ID); r.Code != 204 {
		t.Errorf("re-pin of a pinned channel at the limit = %d %s, want 204", r.Code, r.Body)
	}
	// After an unpin there is room for one more.
	if r := e.pinCall("DELETE", alice, ws, chans[0].ID); r.Code != 204 {
		t.Fatalf("unpin = %d %s", r.Code, r.Body)
	}
	if r := e.pinCall("PUT", alice, ws, chans[30].ID); r.Code != 204 {
		t.Errorf("pin after an unpin = %d %s, want 204", r.Code, r.Body)
	}

	// The limit is per user: bob has his own thirty.
	if r := e.call("POST", "/chat/channels/"+chans[30].ID+"/join", bob, ws, nil); r.Code != 204 {
		t.Fatalf("bob joins = %d %s", r.Code, r.Body)
	}
	if r := e.pinCall("PUT", bob, ws, chans[30].ID); r.Code != 204 {
		t.Errorf("bob's pin while alice is at the limit = %d %s, want 204", r.Code, r.Body)
	}

	// The limit is per workspace: alice's pins elsewhere do not count here.
	ws2 := e.newWorkspace()
	e.join(ws2, alice, app.RoleMember)
	other := e.createChannel(alice, ws2, "public", "elsewhere")
	if r := e.pinCall("PUT", alice, ws2, other.ID); r.Code != 204 {
		t.Errorf("pin in a second workspace = %d %s, want 204", r.Code, r.Body)
	}
}

// Thirty-one pins at once must still leave thirty: the limit is counted under
// a lock on the caller's memberships, not by two requests both seeing 29.
func TestPinLimitHoldsUnderConcurrentPins(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	var chans []channelBody
	for i := range 31 {
		chans = append(chans, e.createChannel(alice, ws, "public", fmt.Sprintf("race-%02d", i)))
	}

	codes := make(chan int, len(chans))
	var wg sync.WaitGroup
	for _, c := range chans {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			codes <- e.pinCall("PUT", alice, ws, id).Code
		}(c.ID)
	}
	wg.Wait()
	close(codes)

	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[204] != 30 || counts[409] != 1 {
		t.Errorf("concurrent pins: %v, want 30 of 204 and 1 of 409", counts)
	}
	pinned := 0
	for _, c := range e.listChannels(alice, ws, "") {
		if c.PinnedAt != nil {
			pinned++
		}
	}
	if pinned != 30 {
		t.Errorf("pinned channels after the race = %d, want 30", pinned)
	}
}

func TestPinFrameGoesOnlyToTheCaller(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "general")
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/join", bob, ws, nil); r.Code != 204 {
		t.Fatalf("bob joins = %d %s", r.Code, r.Body)
	}
	e.rt.reset()

	if r := e.pinCall("PUT", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("pin = %d %s", r.Code, r.Body)
	}
	frames := e.rt.of("channel.pin")
	if len(frames) != 1 || !slices.Equal(frames[0].Users, []string{alice.ID}) {
		t.Fatalf("channel.pin deliveries = %+v, want one to alice only", frames)
	}
	var pin struct {
		WorkspaceID string  `json:"workspace_id"`
		ChannelID   string  `json:"channel_id"`
		PinnedAt    *string `json:"pinned_at"`
	}
	if err := json.Unmarshal(frames[0].Data, &pin); err != nil {
		t.Fatal(err)
	}
	if pin.WorkspaceID != ws || pin.ChannelID != ch.ID || pin.PinnedAt == nil {
		t.Errorf("pin frame = %s", frames[0].Data)
	}

	if r := e.pinCall("DELETE", alice, ws, ch.ID); r.Code != 204 {
		t.Fatalf("unpin = %d %s", r.Code, r.Body)
	}
	frames = e.rt.of("channel.pin")
	if len(frames) != 2 || !slices.Equal(frames[1].Users, []string{alice.ID}) {
		t.Fatalf("channel.pin deliveries after unpin = %+v", frames)
	}
	var unpin struct {
		PinnedAt *string `json:"pinned_at"`
	}
	if err := json.Unmarshal(frames[1].Data, &unpin); err != nil {
		t.Fatal(err)
	}
	if unpin.PinnedAt != nil {
		t.Errorf("unpin frame pinned_at = %s, want null", *unpin.PinnedAt)
	}
}

func TestChannelListOrdersPinnedThenLatestMessageThenName(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	beta := e.createChannel(alice, ws, "public", "beta")
	alpha := e.createChannel(alice, ws, "public", "alpha")
	gamma := e.createChannel(alice, ws, "public", "gamma")
	empty := e.createChannel(alice, ws, "public", "empty")

	// Latest message: beta first, then alpha, then gamma. empty has none.
	e.send(alice, ws, beta.ID, "one")
	e.send(alice, ws, alpha.ID, "two")
	e.send(alice, ws, gamma.ID, "three")

	order := func() []string {
		t.Helper()
		var out []string
		for _, c := range e.listChannels(alice, ws, "") {
			out = append(out, c.Name)
		}
		return out
	}
	if got, want := order(), []string{"gamma", "alpha", "beta", "empty"}; !slices.Equal(got, want) {
		t.Errorf("order by latest message = %v, want %v", got, want)
	}

	// beta is pinned first, then empty: the newest pin is on top.
	for _, c := range []channelBody{beta, empty} {
		if r := e.pinCall("PUT", alice, ws, c.ID); r.Code != 204 {
			t.Fatalf("pin %s = %d %s", c.Name, r.Code, r.Body)
		}
	}
	if got, want := order(), []string{"empty", "beta", "gamma", "alpha"}; !slices.Equal(got, want) {
		t.Errorf("order with pins = %v, want %v", got, want)
	}
	for _, c := range e.listChannels(alice, ws, "") {
		pinned := c.Name == "empty" || c.Name == "beta"
		if (c.PinnedAt != nil) != pinned {
			t.Errorf("%s pinned_at = %v, want pinned=%v", c.Name, c.PinnedAt, pinned)
		}
	}
}

func TestChannelLastMessagePreview(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	e.registerDemo()
	quiet := e.createChannel(alice, ws, "public", "quiet")
	ch := e.createChannel(alice, ws, "public", "news")

	if quiet.LastMessage != nil {
		t.Fatalf("empty channel last_message = %+v, want null", quiet.LastMessage)
	}

	// Text: the body on one line.
	text := e.send(alice, ws, ch.ID, "hello\n\nthere  everyone")
	got := e.getChannel(alice, ws, ch.ID).LastMessage
	if got == nil || got.CreatedAt == "" {
		t.Fatalf("text last_message = %+v, want a message with created_at", got)
	}
	got.CreatedAt = ""
	if want := (lastMessageBody{ID: text.ID, SenderID: alice.ID, Kind: "text", Preview: "hello there everyone"}); *got != want {
		t.Errorf("text last_message = %+v, want %+v", *got, want)
	}

	// A card: its fallback text.
	cardID, err := e.postCard(ws, ch.ID, alice.ID, demoCard{Status: "open", Votes: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.getChannel(alice, ws, ch.ID).LastMessage; got == nil || got.ID != cardID || got.Kind != "demo.poll" || got.Preview != "Poll: lunch?" {
		t.Errorf("card last_message = %+v, want card %s with its fallback text", got, cardID)
	}

	// Attachments only: no text.
	att := e.upload(alice, ws, ch.ID, "notes.txt", "text/plain", 12, true)
	r := e.sendFiles(alice, ws, ch.ID, uuid.Must(uuid.NewV7()).String(), "", att.AttachmentID)
	if r.Code != 201 {
		t.Fatalf("send attachment only = %d %s", r.Code, r.Body)
	}
	var files messageBody
	r.json(t, &files)
	if got := e.getChannel(alice, ws, ch.ID).LastMessage; got == nil || got.ID != files.ID || got.Kind != "text" || got.Preview != "" {
		t.Errorf("attachment-only last_message = %+v, want empty preview", got)
	}

	// A deleted message keeps its place as the last one, with no text.
	bye := e.send(alice, ws, ch.ID, "soon gone")
	if r := e.call("DELETE", "/chat/messages/"+bye.ID, alice, ws, nil); r.Code != 204 {
		t.Fatalf("delete = %d %s", r.Code, r.Body)
	}
	if got := e.getChannel(alice, ws, ch.ID).LastMessage; got == nil || got.ID != bye.ID || got.Preview != "" {
		t.Errorf("deleted last_message = %+v, want %s with an empty preview", got, bye.ID)
	}

	// The list carries the same preview for every channel, and null for the quiet one.
	listed := map[string]*lastMessageBody{}
	for _, c := range e.listChannels(alice, ws, "") {
		listed[c.Name] = c.LastMessage
	}
	if listed["quiet"] != nil {
		t.Errorf("list: quiet last_message = %+v, want null", listed["quiet"])
	}
	if got := listed["news"]; got == nil || got.ID != bye.ID || got.Preview != "" {
		t.Errorf("list: news last_message = %+v", got)
	}
}
