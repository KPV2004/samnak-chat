package chat_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

func TestSendAssignsSeqAndDelivers(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	ch := e.createChannel(alice, ws, "private", "team", bob.ID)
	e.rt.reset()

	clientID := uuid.NewString()
	r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", alice, ws, map[string]any{"client_msg_id": clientID, "body": "สวัสดี **team**"})
	var m messageBody
	r.json(t, &m)
	if r.Code != 201 || m.Seq != 1 || m.SenderID != alice.ID || m.ClientMsgID != clientID || m.Kind != "text" || m.Body != "สวัสดี **team**" || m.ThreadRootID != nil {
		t.Fatalf("send = %d %s", r.Code, r.Body)
	}
	if m2 := e.send(bob, ws, ch.ID, "second"); m2.Seq != 2 {
		t.Errorf("second seq = %d, want 2", m2.Seq)
	}
	if got := e.getChannel(alice, ws, ch.ID); got.LastSeq != 2 {
		t.Errorf("last_seq = %d, want 2", got.LastSeq)
	}

	// message.created reaches the members of the channel and nobody else.
	created := e.rt.of("message.created")
	if len(created) != 2 {
		t.Fatalf("message.created deliveries = %d, want 2", len(created))
	}
	first := created[0]
	slices.Sort(first.Users)
	want := []string{alice.ID, bob.ID}
	slices.Sort(want)
	if !slices.Equal(first.Users, want) || slices.Contains(first.Users, carol.ID) {
		t.Errorf("recipients = %v, want %v", first.Users, want)
	}
	var data struct {
		WorkspaceID string      `json:"workspace_id"`
		ChannelID   string      `json:"channel_id"`
		Message     messageBody `json:"message"`
	}
	if err := json.Unmarshal(first.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.WorkspaceID != ws || data.ChannelID != ch.ID || data.Message.ID != m.ID || data.Message.Seq != 1 {
		t.Errorf("event data = %s", first.Data)
	}

	// chat.message_created is published once per message.
	events := e.events.all()
	var mine []string
	for _, ev := range events {
		if ev.ChannelID == ch.ID {
			mine = append(mine, ev.MessageID)
			if ev.WorkspaceID != ws || ev.ChannelType != "private" || ev.SenderID == "" {
				t.Errorf("event = %+v", ev)
			}
		}
	}
	if len(mine) != 2 || mine[0] != m.ID {
		t.Errorf("published message IDs = %v", mine)
	}
}

func TestSendValidation(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "val")
	path := "/chat/channels/" + ch.ID + "/messages"

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"empty body", map[string]any{"client_msg_id": uuid.NewString(), "body": ""}, 400},
		{"blank body", map[string]any{"client_msg_id": uuid.NewString(), "body": "  \n "}, 400},
		{"4001 characters", map[string]any{"client_msg_id": uuid.NewString(), "body": strings.Repeat("ก", 4001)}, 400},
		{"4000 characters", map[string]any{"client_msg_id": uuid.NewString(), "body": strings.Repeat("ก", 4000)}, 201},
		{"NUL character", map[string]any{"client_msg_id": uuid.NewString(), "body": "a\u0000b"}, 400},
		{"client id not a uuid", map[string]any{"client_msg_id": "abc", "body": "x"}, 400},
		{"missing client id", map[string]any{"body": "x"}, 400},
		{"unknown thread root", map[string]any{"client_msg_id": uuid.NewString(), "body": "x", "thread_root_id": uuid.NewString()}, 400},
		{"malformed thread root", map[string]any{"client_msg_id": uuid.NewString(), "body": "x", "thread_root_id": "nope"}, 400},
	}
	for _, tt := range tests {
		if r := e.call("POST", path, alice, ws, tt.body); r.Code != tt.want {
			t.Errorf("%s: status = %d %.120s, want %d", tt.name, r.Code, r.Body, tt.want)
		}
	}
	if r := e.call("POST", "/chat/channels/not-a-uuid/messages", alice, ws, map[string]any{"client_msg_id": uuid.NewString(), "body": "x"}); r.Code != 404 {
		t.Errorf("malformed channel id = %d, want 404", r.Code)
	}
	// The failed sends left no gap: the next message is seq 2 (after the 4000-character one).
	if m := e.send(alice, ws, ch.ID, "after failures"); m.Seq != 2 {
		t.Errorf("seq after failed sends = %d, want 2", m.Seq)
	}
}

func TestDuplicateClientMsgID(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "dup", bob.ID)
	other := e.createChannel(alice, ws, "public", "dup-other")
	clientID := uuid.NewString()
	path := "/chat/channels/" + ch.ID + "/messages"

	r1 := e.call("POST", path, alice, ws, map[string]any{"client_msg_id": clientID, "body": "once"})
	e.rt.reset()
	eventsBefore := len(e.events.all())
	// The retry may carry a different body (an old retry buffer); the original wins.
	r2 := e.call("POST", path, alice, ws, map[string]any{"client_msg_id": clientID, "body": "once, retried"})
	var m1, m2 messageBody
	r1.json(t, &m1)
	r2.json(t, &m2)
	if r1.Code != 201 || r2.Code != 200 || m1.ID != m2.ID || m2.Body != "once" || m2.Seq != 1 {
		t.Fatalf("first = %d, retry = %d %s", r1.Code, r2.Code, r2.Body)
	}
	if n := len(e.rt.of("message.created")); n != 0 {
		t.Errorf("retry published %d message.created", n)
	}
	if n := len(e.events.all()); n != eventsBefore {
		t.Errorf("retry published chat.message_created")
	}
	if got := e.getChannel(alice, ws, ch.ID); got.LastSeq != 1 {
		t.Errorf("last_seq = %d, want 1", got.LastSeq)
	}

	// Another user may use the same client_msg_id: uniqueness is per sender.
	r3 := e.call("POST", path, bob, ws, map[string]any{"client_msg_id": clientID, "body": "bob"})
	if r3.Code != 201 {
		t.Errorf("same id from another sender = %d, want 201", r3.Code)
	}
	// The same sender reusing it in another channel is a conflict, not a retry.
	r4 := e.call("POST", "/chat/channels/"+other.ID+"/messages", alice, ws, map[string]any{"client_msg_id": clientID, "body": "x"})
	if r4.Code != 409 {
		t.Errorf("same id in another channel = %d, want 409", r4.Code)
	}
}

func TestConcurrentDuplicateSend(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "race")
	clientID := uuid.NewString()
	e.rt.reset()

	const n = 12
	codes := make([]int, n)
	got := make([]messageBody, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", alice, ws, map[string]any{"client_msg_id": clientID, "body": "same"})
			codes[i] = r.Code
			_ = json.Unmarshal(r.Body, &got[i])
		}()
	}
	close(start)
	wg.Wait()

	created := 0
	for i := range n {
		switch codes[i] {
		case 201:
			created++
		case 200:
		default:
			t.Fatalf("send %d = %d", i, codes[i])
		}
		if got[i].ID != got[0].ID || got[i].Seq != 1 {
			t.Errorf("send %d returned %+v, want the one message with seq 1", i, got[i])
		}
	}
	if created != 1 {
		t.Errorf("%d sends were 201, want exactly 1", created)
	}
	if k := len(e.rt.of("message.created")); k != 1 {
		t.Errorf("message.created published %d times, want 1", k)
	}
	if page := e.messages(alice, ws, ch.ID, "?after_seq=0"); len(page.Items) != 1 {
		t.Errorf("channel holds %d messages, want 1", len(page.Items))
	}
}

func TestConcurrentSendersGetDenseSeq(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "busy")
	senders := make([]user, 20)
	for i := range senders {
		senders[i] = e.member(ws, fmt.Sprintf("sender-%d", i))
		e.call("POST", "/chat/channels/"+ch.ID+"/join", senders[i], ws, nil)
	}

	seqs := make([]int64, len(senders))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, u := range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", u, ws, map[string]any{"client_msg_id": uuid.NewString(), "body": fmt.Sprint("m", i)})
			var m messageBody
			_ = json.Unmarshal(r.Body, &m)
			if r.Code != 201 {
				t.Errorf("sender %d = %d %s", i, r.Code, r.Body)
			}
			seqs[i] = m.Seq
		}()
	}
	close(start)
	wg.Wait()

	slices.Sort(seqs)
	for i, s := range seqs {
		if s != int64(i+1) {
			t.Fatalf("seqs = %v, want 1..20 without gaps", seqs)
		}
	}
	// What the channel holds matches, in order.
	page := e.messages(alice, ws, ch.ID, "?after_seq=0&limit=100")
	if len(page.Items) != 20 || page.HasMore {
		t.Fatalf("page = %d items, has_more %v", len(page.Items), page.HasMore)
	}
	for i, m := range page.Items {
		if m.Seq != int64(i+1) {
			t.Errorf("item %d has seq %d", i, m.Seq)
		}
	}
}

func TestCatchUpAndHistoryPagination(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "history", bob.ID)
	for i := 1; i <= 25; i++ {
		e.send(alice, ws, ch.ID, fmt.Sprint("m", i))
	}

	// Catch-up after a gap: bob last saw seq 10.
	p := e.messages(bob, ws, ch.ID, "?after_seq=10&limit=10")
	if len(p.Items) != 10 || !p.HasMore || p.Items[0].Seq != 11 || p.Items[9].Seq != 20 {
		t.Fatalf("catch-up page 1 = %d items, has_more %v, first %d", len(p.Items), p.HasMore, p.Items[0].Seq)
	}
	p = e.messages(bob, ws, ch.ID, "?after_seq=20&limit=10")
	if len(p.Items) != 5 || p.HasMore || p.Items[0].Seq != 21 || p.Items[4].Seq != 25 {
		t.Fatalf("catch-up page 2 = %d items, has_more %v", len(p.Items), p.HasMore)
	}
	if p = e.messages(bob, ws, ch.ID, "?after_seq=25"); len(p.Items) != 0 || p.HasMore {
		t.Errorf("caught up = %d items, has_more %v", len(p.Items), p.HasMore)
	}

	// History: the newest page, then older pages, each ascending.
	p = e.messages(bob, ws, ch.ID, "?limit=10")
	if len(p.Items) != 10 || !p.HasMore || p.Items[0].Seq != 16 || p.Items[9].Seq != 25 {
		t.Fatalf("newest page = %d items, first %d, last %d, has_more %v", len(p.Items), p.Items[0].Seq, p.Items[9].Seq, p.HasMore)
	}
	p = e.messages(bob, ws, ch.ID, "?before_seq=16&limit=10")
	if len(p.Items) != 10 || !p.HasMore || p.Items[0].Seq != 6 || p.Items[9].Seq != 15 {
		t.Fatalf("older page = first %d last %d has_more %v", p.Items[0].Seq, p.Items[9].Seq, p.HasMore)
	}
	p = e.messages(bob, ws, ch.ID, "?before_seq=6&limit=10")
	if len(p.Items) != 5 || p.HasMore || p.Items[0].Seq != 1 {
		t.Fatalf("oldest page = %d items, has_more %v", len(p.Items), p.HasMore)
	}

	// The default page is 50; limits above 100 and both cursors at once are rejected.
	for _, q := range []string{"?limit=101", "?limit=0", "?after_seq=1&before_seq=5", "?after_seq=-5", "?thread_root_id=nope"} {
		if r := e.call("GET", "/chat/channels/"+ch.ID+"/messages"+q, bob, ws, nil); r.Code != 400 {
			t.Errorf("query %s = %d, want 400", q, r.Code)
		}
	}
}

func TestEditAndDelete(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	admin := e.newUser("admin")
	e.join(ws, admin, app.RoleAdmin)
	ch := e.createChannel(alice, ws, "public", "edits", bob.ID)
	m := e.send(alice, ws, ch.ID, "orignal")
	e.rt.reset()

	// Only the author edits.
	if r := e.call("PATCH", "/chat/messages/"+m.ID, bob, ws, map[string]any{"body": "hijack"}); r.Code != 403 {
		t.Errorf("edit by other = %d, want 403", r.Code)
	}
	if r := e.call("PATCH", "/chat/messages/"+m.ID, admin, ws, map[string]any{"body": "hijack"}); r.Code != 403 {
		t.Errorf("edit by admin = %d, want 403", r.Code)
	}
	if r := e.call("PATCH", "/chat/messages/"+m.ID, alice, ws, map[string]any{"body": " "}); r.Code != 400 {
		t.Errorf("edit to blank = %d, want 400", r.Code)
	}
	r := e.call("PATCH", "/chat/messages/"+m.ID, alice, ws, map[string]any{"body": "original"})
	var edited messageBody
	r.json(t, &edited)
	if r.Code != 200 || edited.Body != "original" || edited.EditedAt == nil || edited.Seq != m.Seq || edited.ID != m.ID {
		t.Fatalf("edit = %d %s", r.Code, r.Body)
	}
	if ups := e.rt.of("message.updated"); len(ups) != 1 || !slices.Contains(ups[0].Users, bob.ID) {
		t.Errorf("message.updated deliveries = %+v", ups)
	}

	// Delete: members cannot delete others' messages, the author and admins can.
	if r := e.call("DELETE", "/chat/messages/"+m.ID, bob, ws, nil); r.Code != 403 {
		t.Errorf("delete by other = %d, want 403", r.Code)
	}
	e.rt.reset()
	if r := e.call("DELETE", "/chat/messages/"+m.ID, admin, ws, nil); r.Code != 204 {
		t.Fatalf("delete by admin = %d %s", r.Code, r.Body)
	}
	if dels := e.rt.of("message.deleted"); len(dels) != 1 || !slices.Contains(dels[0].Users, alice.ID) {
		t.Errorf("message.deleted deliveries = %+v", dels)
	}
	// The row and seq stay; the body is gone.
	p := e.messages(bob, ws, ch.ID, "?after_seq=0")
	if len(p.Items) != 1 || p.Items[0].Seq != 1 || p.Items[0].Body != "" || p.Items[0].DeletedAt == nil {
		t.Errorf("deleted message listed as %+v", p.Items)
	}
	if r := e.call("DELETE", "/chat/messages/"+m.ID, alice, ws, nil); r.Code != 404 {
		t.Errorf("delete twice = %d, want 404", r.Code)
	}
	if r := e.call("PATCH", "/chat/messages/"+m.ID, alice, ws, map[string]any{"body": "again"}); r.Code != 404 {
		t.Errorf("edit deleted = %d, want 404", r.Code)
	}

	// An author may delete their own message.
	own := e.send(bob, ws, ch.ID, "mine")
	if r := e.call("DELETE", "/chat/messages/"+own.ID, bob, ws, nil); r.Code != 204 {
		t.Errorf("delete own = %d, want 204", r.Code)
	}
	if r := e.call("DELETE", "/chat/messages/"+uuid.NewString(), bob, ws, nil); r.Code != 404 {
		t.Errorf("delete unknown = %d, want 404", r.Code)
	}
}

func TestThreads(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "threads", bob.ID)
	other := e.createChannel(alice, ws, "public", "threads-other")

	root := e.send(alice, ws, ch.ID, "root")
	e.rt.reset()
	reply1 := e.sendWith(bob, ws, ch.ID, uuid.NewString(), "reply 1", root.ID)
	reply2 := e.sendWith(alice, ws, ch.ID, uuid.NewString(), "reply 2", root.ID)
	if reply1.ThreadRootID == nil || *reply1.ThreadRootID != root.ID || reply1.Seq != 2 || reply2.Seq != 3 {
		t.Errorf("replies = %+v %+v; they get their own seq in the channel", reply1, reply2)
	}

	// Each reply tells clients the root's reply_count changed.
	ups := e.rt.of("message.updated")
	if len(ups) != 2 {
		t.Fatalf("message.updated after two replies = %d, want 2", len(ups))
	}
	var last struct {
		Message messageBody `json:"message"`
	}
	_ = json.Unmarshal(ups[1].Data, &last)
	if last.Message.ID != root.ID || last.Message.ReplyCount != 2 {
		t.Errorf("root update = %+v", last.Message)
	}

	// The thread list holds the replies only; the channel list holds everything.
	p := e.messages(alice, ws, ch.ID, "?after_seq=0&thread_root_id="+root.ID)
	if len(p.Items) != 2 || p.Items[0].ID != reply1.ID || p.Items[1].ID != reply2.ID {
		t.Errorf("thread = %+v", p.Items)
	}
	all := e.messages(alice, ws, ch.ID, "?after_seq=0")
	if len(all.Items) != 3 || all.Items[0].ID != root.ID || all.Items[0].ReplyCount != 2 {
		t.Errorf("channel messages = %+v", all.Items)
	}

	// One level deep, same channel, root must exist.
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", bob, ws, map[string]any{"client_msg_id": uuid.NewString(), "body": "nested", "thread_root_id": reply1.ID}); r.Code != 400 {
		t.Errorf("reply to a reply = %d, want 400", r.Code)
	}
	e.call("POST", "/chat/channels/"+other.ID+"/join", bob, ws, nil)
	if r := e.call("POST", "/chat/channels/"+other.ID+"/messages", bob, ws, map[string]any{"client_msg_id": uuid.NewString(), "body": "cross", "thread_root_id": root.ID}); r.Code != 400 {
		t.Errorf("reply into another channel's root = %d, want 400", r.Code)
	}

	// Deleting a reply lowers the count; replying to a deleted root is refused.
	e.rt.reset()
	if r := e.call("DELETE", "/chat/messages/"+reply2.ID, alice, ws, nil); r.Code != 204 {
		t.Fatalf("delete reply = %d", r.Code)
	}
	if got := e.messages(alice, ws, ch.ID, "?after_seq=0").Items[0]; got.ReplyCount != 1 {
		t.Errorf("reply_count after delete = %d, want 1", got.ReplyCount)
	}
	if ups := e.rt.of("message.updated"); len(ups) != 1 {
		t.Errorf("root updates after deleting a reply = %d, want 1", len(ups))
	}
	e.call("DELETE", "/chat/messages/"+root.ID, alice, ws, nil)
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/messages", bob, ws, map[string]any{"client_msg_id": uuid.NewString(), "body": "late", "thread_root_id": root.ID}); r.Code != 400 {
		t.Errorf("reply to deleted root = %d, want 400", r.Code)
	}
}

func TestMentions(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	ch := e.createChannel(alice, ws, "private", "mentions", bob.ID)

	// carol is in the workspace but not in the channel, so she is not stored.
	m := e.send(alice, ws, ch.ID, fmt.Sprintf("hi <@%s> and <@%s> and <@%s>", bob.ID, bob.ID, carol.ID))
	if !slices.Equal(m.Mentions, []string{bob.ID}) || m.MentionsChannel {
		t.Errorf("mentions = %v channel=%v, want [bob] false", m.Mentions, m.MentionsChannel)
	}
	all := e.send(alice, ws, ch.ID, "heads up <!channel>")
	if !all.MentionsChannel || len(all.Mentions) != 0 {
		t.Errorf("channel mention = %+v", all)
	}
	plain := e.send(alice, ws, ch.ID, "no tokens @bob @channel")
	if plain.MentionsChannel || len(plain.Mentions) != 0 {
		t.Errorf("plain text parsed as mention: %+v", plain)
	}
	// The bus event carries the parsed mentions for notify.
	var found bool
	for _, ev := range e.events.all() {
		if ev.MessageID == m.ID && slices.Equal(ev.Mentions, []string{bob.ID}) {
			found = true
		}
		if ev.MessageID == all.ID && !ev.MentionsChannel {
			t.Error("event lost mentions_channel")
		}
	}
	if !found {
		t.Error("chat.message_created did not carry the mention")
	}

	// Editing re-parses the mentions.
	r := e.call("PATCH", "/chat/messages/"+m.ID, alice, ws, map[string]any{"body": "never mind <!channel>"})
	var edited messageBody
	r.json(t, &edited)
	if !edited.MentionsChannel || len(edited.Mentions) != 0 {
		t.Errorf("edited mentions = %+v", edited)
	}
}

func TestReadState(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "reads", bob.ID)

	e.send(alice, ws, ch.ID, "one")
	e.send(alice, ws, ch.ID, fmt.Sprintf("two <@%s>", bob.ID))
	e.send(alice, ws, ch.ID, "three")
	e.send(alice, ws, ch.ID, "four <!channel>")
	e.send(bob, ws, ch.ID, "bob's own message does not count")

	b := e.getChannel(bob, ws, ch.ID)
	if b.UnreadCount != 4 || b.MentionCount != 2 || b.LastReadSeq != 0 || b.LastSeq != 5 {
		t.Errorf("bob before reading = %+v, want unread 4, mentions 2", b)
	}
	if a := e.getChannel(alice, ws, ch.ID); a.UnreadCount != 1 || a.MentionCount != 0 {
		t.Errorf("alice = %+v, want unread 1 (bob's message), mentions 0", a)
	}
	var listed channelBody
	for _, c := range e.listChannels(bob, ws, "") {
		if c.ID == ch.ID {
			listed = c
		}
	}
	if listed.UnreadCount != 4 || listed.MentionCount != 2 {
		t.Errorf("listed counts = %+v", listed)
	}

	e.rt.reset()
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", bob, ws, map[string]any{"seq": 2}); r.Code != 204 {
		t.Fatalf("read = %d %s", r.Code, r.Body)
	}
	b = e.getChannel(bob, ws, ch.ID)
	if b.LastReadSeq != 2 || b.UnreadCount != 2 || b.MentionCount != 1 {
		t.Errorf("after read 2 = %+v, want unread 2 mentions 1", b)
	}
	// channel.read goes to bob's own devices only.
	reads := e.rt.of("channel.read")
	if len(reads) != 1 || !slices.Equal(reads[0].Users, []string{bob.ID}) {
		t.Fatalf("channel.read deliveries = %+v", reads)
	}
	var rd struct {
		WorkspaceID string `json:"workspace_id"`
		ChannelID   string `json:"channel_id"`
		LastReadSeq int64  `json:"last_read_seq"`
		UnreadCount int64  `json:"unread_count"`
	}
	_ = json.Unmarshal(reads[0].Data, &rd)
	if rd.WorkspaceID != ws || rd.ChannelID != ch.ID || rd.LastReadSeq != 2 || rd.UnreadCount != 2 {
		t.Errorf("channel.read data = %s", reads[0].Data)
	}

	// Only forward: an older seq changes nothing and sends nothing.
	e.rt.reset()
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", bob, ws, map[string]any{"seq": 1}); r.Code != 204 {
		t.Fatalf("older read = %d", r.Code)
	}
	if got := e.getChannel(bob, ws, ch.ID); got.LastReadSeq != 2 {
		t.Errorf("last_read_seq moved back to %d", got.LastReadSeq)
	}
	if n := len(e.rt.of("channel.read")); n != 0 {
		t.Errorf("channel.read sent %d times for a no-op", n)
	}

	// A seq past the end is capped at last_seq.
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", bob, ws, map[string]any{"seq": 9999}); r.Code != 204 {
		t.Fatalf("far read = %d", r.Code)
	}
	b = e.getChannel(bob, ws, ch.ID)
	if b.LastReadSeq != 5 || b.UnreadCount != 0 || b.MentionCount != 0 {
		t.Errorf("after reading everything = %+v", b)
	}
	// So a message sent afterwards is unread.
	e.send(alice, ws, ch.ID, "six")
	if got := e.getChannel(bob, ws, ch.ID); got.UnreadCount != 1 {
		t.Errorf("unread after a new message = %d, want 1", got.UnreadCount)
	}

	// A deleted message is no longer unread.
	m := e.send(alice, ws, ch.ID, "seven")
	e.call("DELETE", "/chat/messages/"+m.ID, alice, ws, nil)
	if got := e.getChannel(bob, ws, ch.ID); got.UnreadCount != 1 {
		t.Errorf("unread after a delete = %d, want 1", got.UnreadCount)
	}

	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", bob, ws, map[string]any{"seq": -1}); r.Code != 400 {
		t.Errorf("negative seq = %d, want 400", r.Code)
	}
	carol := e.member(ws, "carol")
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", carol, ws, map[string]any{"seq": 1}); r.Code != 403 {
		t.Errorf("read by non-member of a public channel = %d, want 403", r.Code)
	}
}

func TestChannelUpdatedIsSentToMembers(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	e.rt.reset()

	ch := e.createChannel(alice, ws, "private", "events", bob.ID)
	ups := e.rt.of("channel.updated")
	if len(ups) != 1 {
		t.Fatalf("channel.updated on create = %d, want 1", len(ups))
	}
	got := ups[0]
	slices.Sort(got.Users)
	want := []string{alice.ID, bob.ID}
	slices.Sort(want)
	if !slices.Equal(got.Users, want) {
		t.Errorf("recipients = %v, want %v", got.Users, want)
	}
	var data struct {
		WorkspaceID string `json:"workspace_id"`
		ChannelID   string `json:"channel_id"`
		Name        string `json:"name"`
		MemberCount int64  `json:"member_count"`
	}
	_ = json.Unmarshal(got.Data, &data)
	if data.WorkspaceID != ws || data.ChannelID != ch.ID || data.Name != "events" || data.MemberCount != 2 {
		t.Errorf("data = %s", got.Data)
	}

	e.rt.reset()
	e.call("PATCH", "/chat/channels/"+ch.ID, alice, ws, map[string]any{"topic": "new topic"})
	if len(e.rt.of("channel.updated")) != 1 {
		t.Error("patch did not send channel.updated")
	}
	// A leaver's own devices hear about it too.
	e.rt.reset()
	e.call("POST", "/chat/channels/"+ch.ID+"/leave", bob, ws, nil)
	ups = e.rt.of("channel.updated")
	if len(ups) != 1 || !slices.Contains(ups[0].Users, bob.ID) || !slices.Contains(ups[0].Users, alice.ID) {
		t.Errorf("leave deliveries = %+v", ups)
	}
}

// Deps.Realtime is nil in tests of other modules and in the openapi command;
// chat must keep working without it.
func TestWorksWithoutRealtime(t *testing.T) {
	e := newEnvWith(t, nil)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "quiet", bob.ID)
	m := e.send(alice, ws, ch.ID, "nobody is connected")
	if m.Seq != 1 {
		t.Errorf("seq = %d, want 1", m.Seq)
	}
	if r := e.call("POST", "/chat/channels/"+ch.ID+"/read", bob, ws, map[string]any{"seq": 1}); r.Code != 204 {
		t.Errorf("read = %d", r.Code)
	}
	if len(e.events.all()) == 0 {
		t.Error("chat.message_created was not published")
	}
}

func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "auth")
	stranger := e.newUser("stranger") // authenticated, but not in this workspace
	for _, c := range []struct {
		name string
		u    user
		ws   string
		want int
	}{
		{"no token", user{}, ws, 401},
		{"bad token", user{Token: "nope"}, ws, 401},
		{"no workspace header", alice, "", 400},
		{"not a member", stranger, ws, 403},
	} {
		if r := e.call("GET", "/chat/channels/"+ch.ID, c.u, c.ws, nil); r.Code != c.want {
			t.Errorf("%s = %d, want %d", c.name, r.Code, c.want)
		}
	}
}
