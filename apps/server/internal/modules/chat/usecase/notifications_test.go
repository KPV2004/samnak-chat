package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

func testID() string { return uuid.Must(uuid.NewV7()).String() }

// nameBook is the Directory port backed by a map.
type nameBook map[string]string

func (b nameBook) Names(_ context.Context, ids []string) map[string]string {
	out := map[string]string{}
	for _, id := range ids {
		if name, ok := b[id]; ok {
			out[id] = name
		}
	}
	return out
}

func (b nameBook) Profiles(_ context.Context, ids []string) map[string]Profile {
	out := map[string]Profile{}
	for _, id := range ids {
		if name, ok := b[id]; ok {
			out[id] = Profile{DisplayName: name}
		}
	}
	return out
}

// notifyRepo is the Repository port chat's notification rule needs, with the
// rest of the interface left nil on purpose: the rule must not call it.
type notifyRepo struct {
	Repository
	channel    domain.Channel
	channelErr error
	message    MessageView
	messageErr error
	members    []string
	membersErr error
	// participants is what ThreadParticipantIDs returns.
	participants []string
	// participantCalls records every thread lookup.
	participantCalls [][3]string
}

func (r *notifyRepo) Message(context.Context, string, string) (MessageView, error) {
	return r.message, r.messageErr
}

func (r *notifyRepo) GetChannel(context.Context, string, string) (domain.Channel, error) {
	return r.channel, r.channelErr
}

func (r *notifyRepo) MemberIDs(context.Context, string, string) ([]string, error) {
	return r.members, r.membersErr
}

func (r *notifyRepo) ThreadParticipantIDs(_ context.Context, workspaceID, channelID, rootID string) ([]string, error) {
	r.participantCalls = append(r.participantCalls, [3]string{workspaceID, channelID, rootID})
	return r.participants, nil
}

func newNotifyService(repo *notifyRepo, names map[string]string) *Service {
	return New(nil, repo, nil, nil, nameBook(names), nil, nil, slog.New(slog.DiscardHandler), time.Now)
}

// notifyEnv is one message in one channel with the users the rule may pick.
type notifyEnv struct {
	repo    *notifyRepo
	service *Service
	ws      string
	channel string
	message string
	sender  string
	names   nameBook
}

func newNotifyEnv(channelType domain.ChannelType, channelName string, members ...string) *notifyEnv {
	ws, chID, msgID := testID(), testID(), testID()
	sender := members[0]
	return &notifyEnv{
		repo: &notifyRepo{
			channel: domain.Channel{ID: chID, WorkspaceID: ws, Type: channelType, Name: channelName},
			message: MessageView{Message: domain.Message{ID: msgID, WorkspaceID: ws, ChannelID: chID, SenderID: sender, Body: "hello"}},
			members: members,
		},
		ws: ws, channel: chID, message: msgID, sender: sender,
		names: nameBook{},
	}
}

func (e *notifyEnv) run(t *testing.T, in NotifyInput) []app.Notification {
	t.Helper()
	if e.service == nil {
		e.service = newNotifyService(e.repo, e.names)
	}
	if in.WorkspaceID == "" {
		in = NotifyInput{
			WorkspaceID: e.ws, ChannelID: e.channel, MessageID: e.message, Seq: 7, SenderID: e.sender,
			ThreadRootID: in.ThreadRootID, Mentions: in.Mentions, MentionsChannel: in.MentionsChannel,
		}
	}
	got, err := e.service.NotifyMessageCreated(t.Context(), in)
	if err != nil {
		t.Fatalf("NotifyMessageCreated: %v", err)
	}
	return got
}

// userIDs returns the recipients of notifications, in order.
func userIDs(list []app.Notification) []string {
	out := make([]string, len(list))
	for i, n := range list {
		out[i] = n.UserID
	}
	return out
}

func TestNotifyMentionNotifiesOnlyTheMentionedMember(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID(), testID(), testID())
	sender, bob, carol := e.repo.members[0], e.repo.members[1], e.repo.members[2]
	e.names[sender], e.names[bob], e.names[carol] = "Alice", "Bob", "Carol"
	e.repo.message.Body = "hello <@" + bob + ">, please look"

	got := e.run(t, NotifyInput{Mentions: []string{bob}})
	if len(got) != 1 || got[0].UserID != bob {
		t.Fatalf("notifications for %v, want only bob", userIDs(got))
	}
	n := got[0]
	if n.WorkspaceID != e.ws || n.Kind != "chat.mention" {
		t.Errorf("workspace/kind = %q/%q", n.WorkspaceID, n.Kind)
	}
	if n.Title != "Alice in #general" {
		t.Errorf("title = %q", n.Title)
	}
	if n.Body != "hello @Bob, please look" {
		t.Errorf("body = %q", n.Body)
	}
	if n.Link != "/chat/"+e.channel {
		t.Errorf("link = %q", n.Link)
	}
	if n.ChannelID != e.channel {
		t.Errorf("channel id = %q, want %q: notify drops muted channels by it (NOTI-07)", n.ChannelID, e.channel)
	}
	if n.Key != "chat.message:"+e.message {
		t.Errorf("key = %q", n.Key)
	}
	wantData := map[string]any{
		"channel_id": e.channel, "message_id": e.message, "seq": int64(7), "thread_root_id": "",
		"sender_name": "Alice", "channel_name": "general",
	}
	for k, want := range wantData {
		if have := n.Data[k]; have != want {
			t.Errorf("data[%q] = %#v, want %#v", k, have, want)
		}
	}
	if len(e.repo.participantCalls) != 0 {
		t.Errorf("the rule looked up thread participants for a top-level message")
	}
}

func TestNotifyChannelMentionNotifiesEveryMemberButTheSender(t *testing.T) {
	e := newNotifyEnv(domain.TypePrivate, "ops", testID(), testID(), testID())
	e.repo.message.Body = "<!channel> deploy is going out"
	e.repo.message.MentionsChannel = true
	e.repo.message.Mentions = []string{}

	got := e.run(t, NotifyInput{MentionsChannel: true})
	if len(got) != 2 {
		t.Fatalf("notifications for %v, want the two other members", userIDs(got))
	}
	for _, n := range got {
		if n.UserID == e.sender {
			t.Errorf("the sender was notified")
		}
		if n.Kind != "chat.mention" {
			t.Errorf("kind = %q, want chat.mention", n.Kind)
		}
		if !strings.Contains(n.Body, "@channel") || strings.Contains(n.Body, "<!channel>") {
			t.Errorf("body = %q, want the channel token rendered", n.Body)
		}
	}
}

func TestNotifyDirectMessageNotifiesTheOtherMembers(t *testing.T) {
	e := newNotifyEnv(domain.TypeDirect, "", testID(), testID(), testID())
	alice := e.repo.members[0]
	bob, carol := e.repo.members[1], e.repo.members[2]
	e.names[alice] = "Alice"

	got := e.run(t, NotifyInput{})
	if len(got) != 2 {
		t.Fatalf("notifications for %v, want bob and carol", userIDs(got))
	}
	for _, n := range got {
		if n.Kind != "chat.dm" {
			t.Errorf("kind = %q, want chat.dm", n.Kind)
		}
		if n.Title != "Alice in DM" {
			t.Errorf("title = %q", n.Title)
		}
		if n.Data["sender_name"] != "Alice" || n.Data["channel_name"] != "" {
			t.Errorf("data names = %#v/%#v, want Alice and no channel name", n.Data["sender_name"], n.Data["channel_name"])
		}
		if n.Link != "/chat/"+e.channel {
			t.Errorf("link = %q", n.Link)
		}
		if n.ChannelID != e.channel {
			t.Errorf("channel id = %q, want %q", n.ChannelID, e.channel)
		}
	}
	if got[0].UserID != bob || got[1].UserID != carol {
		t.Errorf("recipients = %v", userIDs(got))
	}
}

func TestNotifyThreadReplyNotifiesRootAuthorAndEarlierRepliers(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID(), testID(), testID(), testID())
	sender, rootAuthor, replier := e.repo.members[0], e.repo.members[1], e.repo.members[2]
	root := testID()
	e.repo.participants = []string{rootAuthor, replier, sender}
	e.repo.message.ThreadRootID = root

	got := e.run(t, NotifyInput{ThreadRootID: root})
	if len(got) != 2 || got[0].UserID != rootAuthor || got[1].UserID != replier {
		t.Fatalf("notifications for %v, want the root author and the earlier replier", userIDs(got))
	}
	for _, n := range got {
		if n.Kind != "chat.thread_reply" {
			t.Errorf("kind = %q, want chat.thread_reply", n.Kind)
		}
		if n.Data["thread_root_id"] != root {
			t.Errorf("data[thread_root_id] = %#v", n.Data["thread_root_id"])
		}
		if n.ChannelID != e.channel {
			t.Errorf("channel id = %q, want %q", n.ChannelID, e.channel)
		}
	}
	if len(e.repo.participantCalls) != 1 || e.repo.participantCalls[0] != [3]string{e.ws, e.channel, root} {
		t.Errorf("thread lookups = %v, want one for the root", e.repo.participantCalls)
	}
}

func TestNotifyKeepsOneNotificationByPriority(t *testing.T) {
	// A direct message that mentions bob and answers a thread bob follows:
	// bob gets the mention, carol the DM, and nobody gets a thread reply.
	e := newNotifyEnv(domain.TypeDirect, "", testID(), testID(), testID())
	bob, carol := e.repo.members[1], e.repo.members[2]
	root := testID()
	e.repo.participants = []string{bob, carol}

	got := e.run(t, NotifyInput{ThreadRootID: root, Mentions: []string{bob}})
	if len(got) != 2 {
		t.Fatalf("notifications for %v, want one each for bob and carol", userIDs(got))
	}
	kinds := map[string]string{}
	for _, n := range got {
		kinds[n.UserID] = n.Kind
	}
	if kinds[bob] != "chat.mention" {
		t.Errorf("bob's kind = %q, want the mention to win", kinds[bob])
	}
	if kinds[carol] != "chat.dm" {
		t.Errorf("carol's kind = %q, want the DM to win over the thread", kinds[carol])
	}
}

func TestNotifySkipsTheSenderAndNonMembers(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID(), testID())
	outsider, previousMember := testID(), testID()
	root := testID()
	e.repo.message.ThreadRootID = root
	e.repo.participants = []string{e.sender, e.repo.members[1], outsider, previousMember}
	e.repo.members = []string{e.sender, e.repo.members[1]} // the other two left the channel

	got := e.run(t, NotifyInput{ThreadRootID: root, Mentions: []string{e.sender, outsider}})
	if len(got) != 1 || got[0].UserID != e.repo.members[1] {
		t.Fatalf("notifications for %v, want only the remaining member", userIDs(got))
	}
}

func TestNotifyWithoutRecipientsReturnsNothing(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID())
	if got := e.run(t, NotifyInput{}); len(got) != 0 {
		t.Fatalf("notifications for %v, want none", userIDs(got))
	}
}

func TestNotifyExcerptIsPlainShortText(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID(), testID())
	bob := e.repo.members[1]
	e.names[bob] = "Bob"

	e.repo.message.Body = "line one\n\nline   two\t<@" + bob + ">"
	got := e.run(t, NotifyInput{Mentions: []string{bob}})
	if len(got) != 1 {
		t.Fatalf("got %d notifications", len(got))
	}
	if want := "line one line two @Bob"; got[0].Body != want {
		t.Errorf("body = %q, want %q", got[0].Body, want)
	}

	// A user the directory does not know shows as their ID rather than a
	// dangling mention token.
	e.repo.message.Body = "hi <@" + bob + ">"
	for id := range e.names {
		delete(e.names, id)
	}
	got = e.run(t, NotifyInput{Mentions: []string{bob}})
	if want := "hi @" + bob; got[0].Body != want {
		t.Errorf("body = %q, want %q", got[0].Body, want)
	}
}

func TestNotifyExcerptIsAtMost140Runes(t *testing.T) {
	e := newNotifyEnv(domain.TypeDirect, "", testID(), testID())
	e.repo.message.Body = strings.Repeat("ก", 200)

	got := e.run(t, NotifyInput{})
	if len(got) != 1 {
		t.Fatalf("got %d notifications", len(got))
	}
	body := got[0].Body
	if n := utf8.RuneCountInString(body); n != 140 {
		t.Errorf("body has %d runes, want 140", n)
	}
	if !strings.HasSuffix(body, "…") {
		t.Errorf("body = %q, want an ellipsis", body)
	}
}

func TestNotifyToleratesAMissingMessageOrChannel(t *testing.T) {
	e := newNotifyEnv(domain.TypePublic, "general", testID(), testID(), testID())
	e.repo.messageErr = app.ErrNotFound
	if got := e.run(t, NotifyInput{}); len(got) != 0 {
		t.Fatalf("notifications for %v, want none", userIDs(got))
	}

	e.repo.messageErr = errors.New("boom")
	if _, err := e.service.NotifyMessageCreated(t.Context(), NotifyInput{
		WorkspaceID: e.ws, ChannelID: e.channel, MessageID: e.message, SenderID: e.sender,
	}); !errors.Is(err, e.repo.messageErr) {
		t.Errorf("error = %v, want the repository error", err)
	}
}
