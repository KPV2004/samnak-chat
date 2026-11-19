package chat_test

import (
	"log/slog"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// chat registers exactly one rule for its event, without a database or a
// bus carrying anything (NOTI-01).
func TestChatRegistersNotificationRule(t *testing.T) {
	rules := app.NewNotificationRules()
	mod := chat.New()
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"chat"})
	if err != nil {
		t.Fatal(err)
	}
	deps := &app.Deps{Logger: slog.New(slog.DiscardHandler), Services: app.NewServices(), Notifications: rules}
	if err := reg.Start(mods, deps, httpx.NewAPI(chi.NewRouter(), "test")); err != nil {
		t.Fatal(err)
	}
	if n := len(rules.Rules()["chat.message_created"]); n != 1 {
		t.Fatalf("chat registered %d rules on chat.message_created, want 1", n)
	}
}

// The rule reads chat's own tables and produces the plan's kinds, titles,
// bodies and keys for mentions, direct messages and followed threads.
func TestNotificationRule(t *testing.T) {
	rules := app.NewNotificationRules()
	e := newEnvWithRules(t, &fakeRealtime{}, rules)
	ws := e.newWorkspace()
	owner := e.newUser("Owner")
	e.join(ws, owner, app.RoleOwner)
	alice := e.member(ws, "Alice")
	bob := e.member(ws, "Bob")
	carol := e.member(ws, "Carol")
	ch := e.createChannel(owner, ws, "public", "general", alice.ID, bob.ID, carol.ID)

	t.Run("mention", func(t *testing.T) {
		msg := e.send(alice, ws, ch.ID, "hello <@"+bob.ID+">")
		got := runChatRule(t, rules, eventFor(t, e, msg.ID))
		if len(got) != 1 || got[0].UserID != bob.ID {
			t.Fatalf("notifications for %v, want only Bob", recipients(got))
		}
		n := got[0]
		if n.Kind != "chat.mention" || n.Title != "Alice in #general" || n.Body != "hello @Bob" {
			t.Errorf("kind/title/body = %q/%q/%q", n.Kind, n.Title, n.Body)
		}
		if n.Link != "/chat/"+ch.ID || n.Key != "chat.message:"+msg.ID {
			t.Errorf("link/key = %q/%q", n.Link, n.Key)
		}
		if n.ChannelID != ch.ID {
			t.Errorf("channel id = %q, want %q", n.ChannelID, ch.ID)
		}
		if n.Data["channel_id"] != ch.ID || n.Data["message_id"] != msg.ID || n.Data["seq"] != msg.Seq {
			t.Errorf("data = %v", n.Data)
		}
	})

	t.Run("direct message", func(t *testing.T) {
		r := e.call("POST", "/chat/dms", alice, ws, map[string]any{"user_ids": []string{bob.ID}})
		if r.Code != 200 && r.Code != 201 {
			t.Fatalf("open dm = %d %s", r.Code, r.Body)
		}
		var dm channelBody
		r.json(t, &dm)
		msg := e.send(alice, ws, dm.ID, "just us")
		got := runChatRule(t, rules, eventFor(t, e, msg.ID))
		if len(got) != 1 || got[0].UserID != bob.ID {
			t.Fatalf("notifications for %v, want only Bob", recipients(got))
		}
		if got[0].Kind != "chat.dm" || got[0].Title != "Alice in DM" {
			t.Errorf("kind/title = %q/%q", got[0].Kind, got[0].Title)
		}
	})

	t.Run("thread reply skips a participant who left", func(t *testing.T) {
		root := e.send(bob, ws, ch.ID, "a question")
		e.sendWith(carol, ws, ch.ID, uuid.NewString(), "an answer", root.ID)
		if r := e.call("POST", "/chat/channels/"+ch.ID+"/leave", bob, ws, nil); r.Code != 204 {
			t.Fatalf("leave = %d %s", r.Code, r.Body)
		}
		reply := e.sendWith(alice, ws, ch.ID, uuid.NewString(), "thanks", root.ID)
		got := runChatRule(t, rules, eventFor(t, e, reply.ID))
		if len(got) != 1 || got[0].UserID != carol.ID {
			t.Fatalf("notifications for %v, want only Carol (Bob left, Alice sent)", recipients(got))
		}
		if got[0].Kind != "chat.thread_reply" || got[0].Data["thread_root_id"] != root.ID {
			t.Errorf("kind/data = %q/%v", got[0].Kind, got[0].Data)
		}
	})
}

// eventFor returns the chat.message_created event of one message.
func eventFor(t *testing.T, e *env, messageID string) app.Event {
	t.Helper()
	for _, evt := range e.events.all() {
		if evt.MessageID == messageID {
			return evt
		}
	}
	t.Fatalf("no chat.message_created for message %s", messageID)
	return nil
}

// runChatRule invokes chat's registered rule, as notify does.
func runChatRule(t *testing.T, rules *app.NotificationRules, evt app.Event) []app.Notification {
	t.Helper()
	registered := rules.Rules()["chat.message_created"]
	if len(registered) != 1 {
		t.Fatalf("chat registered %d rules, want 1", len(registered))
	}
	got, err := registered[0](t.Context(), evt)
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	return got
}

func recipients(list []app.Notification) []string {
	out := make([]string, len(list))
	for i, n := range list {
		out[i] = n.UserID
	}
	return out
}
