package chat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

func (e *env) messageReader() app.MessageReader {
	e.t.Helper()
	r, ok := app.Lookup[app.MessageReader](e.svc)
	if !ok {
		e.t.Fatal("chat does not provide app.MessageReader")
	}
	return r
}

func TestMessageReaderReturnsTextWithFiles(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "private", "files", bob.ID)
	photo := e.upload(alice, ws, ch.ID, "photo.PNG", "image/png", 2048, true)
	doc := e.upload(alice, ws, ch.ID, "spec.pdf", "application/pdf", 4096, true)
	r := e.sendFiles(alice, ws, ch.ID, uuid.NewString(), "see the files", photo.AttachmentID, doc.AttachmentID)
	if r.Code != 201 {
		t.Fatalf("send = %d %s", r.Code, r.Body)
	}
	var m messageBody
	r.json(t, &m)

	got, err := e.messageReader().ReadMessage(context.Background(), ws, m.ID, bob.ID)
	if err != nil {
		t.Fatalf("ReadMessage = %v", err)
	}
	if got.ID != m.ID || got.ChannelID != ch.ID || got.SenderID != alice.ID || got.Kind != "text" || got.Body != "see the files" {
		t.Errorf("message = %+v", got)
	}
	if got.CreatedAt.IsZero() || got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at = %v, want a UTC time", got.CreatedAt)
	}
	want := map[string]app.MessageAttachment{
		"chat/" + ws + "/" + photo.AttachmentID: {FileName: "photo.PNG", MimeType: "image/png", Size: 2048, ObjectKey: "chat/" + ws + "/" + photo.AttachmentID},
		"chat/" + ws + "/" + doc.AttachmentID:   {FileName: "spec.pdf", MimeType: "application/pdf", Size: 4096, ObjectKey: "chat/" + ws + "/" + doc.AttachmentID},
	}
	if len(got.Attachments) != len(want) {
		t.Fatalf("attachments = %+v, want %d files", got.Attachments, len(want))
	}
	for _, a := range got.Attachments {
		if w, ok := want[a.ObjectKey]; !ok || a != w {
			t.Errorf("attachment = %+v, want %+v", a, w)
		}
	}
}

func TestMessageReaderReturnsCardMessages(t *testing.T) {
	e := newEnv(t)
	e.registerDemo()
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "public", "polls")
	id, err := e.postCard(ws, ch.ID, alice.ID, demoCard{Status: "open"})
	if err != nil {
		t.Fatalf("post card = %v", err)
	}

	got, err := e.messageReader().ReadMessage(context.Background(), ws, id, bob.ID)
	if err != nil {
		t.Fatalf("ReadMessage = %v", err)
	}
	if got.Kind != "demo.poll" || got.SenderID != alice.ID || got.ChannelID != ch.ID {
		t.Errorf("card message = %+v", got)
	}
}

func TestMessageReaderFollowsChannelReadRules(t *testing.T) {
	e := newEnv(t)
	reader, access := e.messageReader(), e.access()
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	guest, gone, outsider := e.newUser("guest"), e.newUser("gone"), e.newUser("outsider")
	e.join(ws, guest, app.RoleGuest)
	e.join(ws, gone, app.RoleMember)
	admin := e.newUser("admin")
	e.join(ws, admin, app.RoleAdmin)
	dave := e.member(other, "dave")

	public := e.createChannel(alice, ws, "public", "open")
	private := e.createChannel(alice, ws, "private", "secret", bob.ID)
	guests := e.createChannel(alice, ws, "private", "guests", guest.ID)
	team := e.createChannel(alice, ws, "private", "team", bob.ID, gone.ID)
	archived := e.createChannel(alice, ws, "public", "old")

	msgPublic := e.send(alice, ws, public.ID, "hello")
	msgPrivate := e.send(alice, ws, private.ID, "secret")
	msgGuests := e.send(alice, ws, guests.ID, "for guests")
	msgTeam := e.send(alice, ws, team.ID, "team only")
	msgArchived := e.send(alice, ws, archived.ID, "old news")
	msgDeleted := e.send(alice, ws, private.ID, "oops")
	if r := e.call("DELETE", "/chat/messages/"+msgDeleted.ID, alice, ws, nil); r.Code != 204 {
		t.Fatalf("delete = %d %s", r.Code, r.Body)
	}
	if r := e.call("PATCH", "/chat/channels/"+archived.ID, admin, ws, map[string]any{"archived": true}); r.Code != 200 {
		t.Fatalf("archive = %d %s", r.Code, r.Body)
	}
	e.members.remove(ws, gone.ID)

	unknown := uuid.NewString()
	for _, tc := range []struct {
		what string
		ws   string
		msg  string
		user string
		ok   bool
	}{
		{"member, public, not joined", ws, msgPublic.ID, carol.ID, true},
		{"guest, public, not joined", ws, msgPublic.ID, guest.ID, false},
		{"guest, private they are in", ws, msgGuests.ID, guest.ID, true},
		{"member of private", ws, msgPrivate.ID, bob.ID, true},
		{"non-member of private", ws, msgPrivate.ID, carol.ID, false},
		{"admin who is not in private", ws, msgPrivate.ID, admin.ID, false},
		{"archived public, not joined", ws, msgArchived.ID, carol.ID, true},
		{"deleted message, member", ws, msgDeleted.ID, bob.ID, false},
		{"removed member of team", ws, msgTeam.ID, gone.ID, false},
		{"user outside the workspace", ws, msgPublic.ID, outsider.ID, false},
		{"user of another workspace", ws, msgPublic.ID, dave.ID, false},
		{"message of another workspace", other, msgPublic.ID, carol.ID, false},
		{"unknown message", ws, unknown, carol.ID, false},
		{"malformed message ID", ws, "nope", carol.ID, false},
		{"malformed user ID", ws, msgPublic.ID, "nope", false},
		{"malformed workspace ID", "nope", msgPublic.ID, carol.ID, false},
	} {
		got, err := reader.ReadMessage(context.Background(), tc.ws, tc.msg, tc.user)
		if tc.ok {
			if err != nil || got.ID != tc.msg {
				t.Errorf("%s: ReadMessage = %+v, %v; want the message", tc.what, got, err)
			}
			continue
		}
		if !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s: ReadMessage = %+v, %v; want ErrNotFound", tc.what, got, err)
		}
	}

	// The same rule as ChannelAccess: a message is readable exactly when its channel is.
	for _, m := range []struct {
		name string
		id   string
		ch   string
	}{
		{"public", msgPublic.ID, public.ID},
		{"private", msgPrivate.ID, private.ID},
		{"guests", msgGuests.ID, guests.ID},
		{"team", msgTeam.ID, team.ID},
		{"archived", msgArchived.ID, archived.ID},
	} {
		for _, u := range []user{alice, bob, carol, guest, admin, gone, outsider} {
			want, err := access.CanRead(context.Background(), ws, m.ch, u.ID)
			if err != nil {
				t.Fatalf("CanRead = %v", err)
			}
			_, err = reader.ReadMessage(context.Background(), ws, m.id, u.ID)
			if got := err == nil; got != want {
				t.Errorf("%s as %s: ReadMessage ok = %v (%v), CanRead = %v", m.name, u.Name, got, err, want)
			}
		}
	}
}
