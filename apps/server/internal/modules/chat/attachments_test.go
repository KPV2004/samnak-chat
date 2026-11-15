package chat_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type uploadBody struct {
	AttachmentID string `json:"attachment_id"`
	Upload       struct {
		URL       string            `json:"url"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		ExpiresAt time.Time         `json:"expires_at"`
	} `json:"upload"`
}

// upload asks for an upload and, when put is true, stores the bytes.
func (e *env) upload(u user, ws, channelID, name, mime string, size int64, put bool) uploadBody {
	e.t.Helper()
	r := e.call("POST", "/chat/uploads", u, ws, map[string]any{"channel_id": channelID, "file_name": name, "size": size, "mime_type": mime})
	if r.Code != 201 {
		e.t.Fatalf("upload = %d %s", r.Code, r.Body)
	}
	var up uploadBody
	r.json(e.t, &up)
	if put {
		e.storage.put(fmt.Sprintf("chat/%s/%s", ws, up.AttachmentID), size)
	}
	return up
}

func (e *env) sendFiles(u user, ws, channelID, clientID, text string, attachmentIDs ...string) resp {
	e.t.Helper()
	return e.call("POST", "/chat/channels/"+channelID+"/messages", u, ws,
		map[string]any{"client_msg_id": clientID, "body": text, "attachment_ids": attachmentIDs})
}

func TestUploadPresignsKeyTypeAndSize(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	ch := e.createChannel(alice, ws, "public", "files")

	up := e.upload(alice, ws, ch.ID, `C:\Users\alice\รายงาน.pdf`, "application/pdf", 1234, false)
	if up.Upload.Method != "PUT" || !strings.Contains(up.Upload.URL, "chat/"+ws+"/"+up.AttachmentID) {
		t.Errorf("upload = %+v", up)
	}
	if up.Upload.Headers["Content-Type"] != "application/pdf" || up.Upload.Headers["Content-Length"] != "1234" {
		t.Errorf("headers = %v", up.Upload.Headers)
	}
	if len(e.storage.puts) != 1 {
		t.Fatalf("puts = %v", e.storage.puts)
	}
	got := e.storage.puts[0]
	if got.Key != "chat/"+ws+"/"+up.AttachmentID || got.ContentType != "application/pdf" || got.Size != 1234 || got.TTL != 15*time.Minute {
		t.Errorf("presign call = %+v", got)
	}
	if _, err := uuid.Parse(up.AttachmentID); err != nil {
		t.Errorf("attachment_id %q is not a UUID", up.AttachmentID)
	}
}

func TestUploadValidation(t *testing.T) {
	e := newEnv(t)
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	mallory := e.member(other, "mallory")
	pub := e.createChannel(alice, ws, "public", "open")
	priv := e.createChannel(alice, ws, "private", "secret")
	archived := e.createChannel(alice, ws, "public", "old")
	if r := e.call("PATCH", "/chat/channels/"+archived.ID, alice, ws, map[string]any{"archived": true}); r.Code != 200 {
		t.Fatal(r.Code)
	}
	req := func(u user, w string, channelID string, size int64, mime, name string) int {
		return e.call("POST", "/chat/uploads", u, w, map[string]any{"channel_id": channelID, "file_name": name, "size": size, "mime_type": mime}).Code
	}
	for _, c := range []struct {
		name string
		code int
		got  int
	}{
		{"50 MB is the limit", 201, req(alice, ws, pub.ID, 50<<20, "video/mp4", "a.mp4")},
		{"over 50 MB", 400, req(alice, ws, pub.ID, 50<<20+1, "video/mp4", "a.mp4")},
		{"zero bytes", 400, req(alice, ws, pub.ID, 0, "text/plain", "a.txt")},
		{"negative size", 400, req(alice, ws, pub.ID, -5, "text/plain", "a.txt")},
		{"bad mime type", 400, req(alice, ws, pub.ID, 10, "pdf", "a.pdf")},
		{"empty name", 400, req(alice, ws, pub.ID, 10, "text/plain", " ")},
		{"channel not joined", 403, req(bob, ws, pub.ID, 10, "text/plain", "a.txt")},
		{"private channel of others", 404, req(bob, ws, priv.ID, 10, "text/plain", "a.txt")},
		{"other workspace", 404, req(mallory, other, pub.ID, 10, "text/plain", "a.txt")},
		{"archived channel", 409, req(alice, ws, archived.ID, 10, "text/plain", "a.txt")},
		{"unknown channel", 404, req(alice, ws, uuid.NewString(), 10, "text/plain", "a.txt")},
	} {
		if c.got != c.code {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.code)
		}
	}
}

func TestSendWithAttachments(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	ch := e.createChannel(alice, ws, "private", "files", bob.ID)
	photo := e.upload(alice, ws, ch.ID, "photo.PNG", "image/png", 2048, true)
	doc := e.upload(alice, ws, ch.ID, "spec.pdf", "application/pdf", 4096, true)
	e.rt.reset()

	clientID := uuid.NewString()
	r := e.sendFiles(alice, ws, ch.ID, clientID, "", photo.AttachmentID, doc.AttachmentID)
	if r.Code != 201 {
		t.Fatalf("send = %d %s", r.Code, r.Body)
	}
	var m messageBody
	r.json(t, &m)
	if m.Body != "" || len(m.Attachments) != 2 {
		t.Fatalf("message = %s", r.Body)
	}
	want := map[string]attachmentBody{
		photo.AttachmentID: {ID: photo.AttachmentID, FileName: "photo.PNG", Size: 2048, MimeType: "image/png", IsImage: true},
		doc.AttachmentID:   {ID: doc.AttachmentID, FileName: "spec.pdf", Size: 4096, MimeType: "application/pdf", IsImage: false},
	}
	for _, a := range m.Attachments {
		if want[a.ID] != a {
			t.Errorf("attachment = %+v, want %+v", a, want[a.ID])
		}
	}

	// The same files come back when listing, and in the event.
	page := e.messages(bob, ws, ch.ID, "")
	if len(page.Items) != 1 || len(page.Items[0].Attachments) != 2 {
		t.Fatalf("list = %+v", page)
	}
	created := e.rt.of("message.created")
	if len(created) != 1 {
		t.Fatalf("message.created deliveries = %d", len(created))
	}
	var ev struct {
		Message messageBody `json:"message"`
	}
	if err := json.Unmarshal(created[0].Data, &ev); err != nil || len(ev.Message.Attachments) != 2 {
		t.Errorf("event = %s (%v)", created[0].Data, err)
	}

	// Sending again with the same client_msg_id returns the same message.
	r = e.sendFiles(alice, ws, ch.ID, clientID, "", photo.AttachmentID, doc.AttachmentID)
	var again messageBody
	r.json(t, &again)
	if r.Code != 200 || again.ID != m.ID || len(again.Attachments) != 2 {
		t.Errorf("resend = %d %s", r.Code, r.Body)
	}

	// Text and a file together.
	extra := e.upload(alice, ws, ch.ID, "x.txt", "text/plain", 5, true)
	r = e.sendFiles(alice, ws, ch.ID, uuid.NewString(), "see attached", extra.AttachmentID)
	var withText messageBody
	r.json(t, &withText)
	if r.Code != 201 || withText.Body != "see attached" || len(withText.Attachments) != 1 {
		t.Errorf("text and file = %d %s", r.Code, r.Body)
	}
}

func TestSendAttachmentRules(t *testing.T) {
	e := newEnv(t)
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob := e.member(ws, "alice"), e.member(ws, "bob")
	mallory := e.member(other, "mallory")
	one := e.createChannel(alice, ws, "public", "one")
	two := e.createChannel(alice, ws, "public", "two")
	if r := e.call("POST", "/chat/channels/"+one.ID+"/join", bob, ws, nil); r.Code != 204 {
		t.Fatal(r.Code)
	}
	send := func(u user, w, channelID string, ids ...string) resp {
		return e.sendFiles(u, w, channelID, uuid.NewString(), "", ids...)
	}

	notUploaded := e.upload(alice, ws, one.ID, "a.txt", "text/plain", 10, false)
	if r := send(alice, ws, one.ID, notUploaded.AttachmentID); r.Code != 400 {
		t.Errorf("file never uploaded = %d %s, want 400", r.Code, r.Body)
	}
	// The stored object has another size than the one that was announced.
	wrongSize := e.upload(alice, ws, one.ID, "b.txt", "text/plain", 10, false)
	e.storage.put("chat/"+ws+"/"+wrongSize.AttachmentID, 11)
	if r := send(alice, ws, one.ID, wrongSize.AttachmentID); r.Code != 400 {
		t.Errorf("wrong size = %d %s, want 400", r.Code, r.Body)
	}

	mine := e.upload(alice, ws, one.ID, "c.txt", "text/plain", 10, true)
	if r := send(bob, ws, one.ID, mine.AttachmentID); r.Code != 404 {
		t.Errorf("someone else's file = %d %s, want 404", r.Code, r.Body)
	}
	if r := send(mallory, other, one.ID, mine.AttachmentID); r.Code != 404 {
		t.Errorf("other workspace = %d %s, want 404", r.Code, r.Body)
	}
	if r := send(alice, ws, two.ID, mine.AttachmentID); r.Code != 400 {
		t.Errorf("file from another channel = %d %s, want 400", r.Code, r.Body)
	}
	if r := send(alice, ws, one.ID, uuid.NewString()); r.Code != 404 {
		t.Errorf("unknown file = %d %s, want 404", r.Code, r.Body)
	}
	if r := send(alice, ws, one.ID, "not-a-uuid"); r.Code != 400 {
		t.Errorf("malformed id = %d %s, want 400", r.Code, r.Body)
	}
	if r := send(alice, ws, one.ID, mine.AttachmentID, mine.AttachmentID); r.Code != 400 {
		t.Errorf("same file twice = %d %s, want 400", r.Code, r.Body)
	}
	if r := e.sendFiles(alice, ws, one.ID, uuid.NewString(), ""); r.Code != 400 {
		t.Errorf("empty body and no files = %d %s, want 400", r.Code, r.Body)
	}

	// Nothing above used the file; now it can be sent once.
	if r := send(alice, ws, one.ID, mine.AttachmentID); r.Code != 201 {
		t.Fatalf("send = %d %s", r.Code, r.Body)
	}
	if r := send(alice, ws, one.ID, mine.AttachmentID); r.Code != 400 {
		t.Errorf("file used twice = %d %s, want 400", r.Code, r.Body)
	}

	// At most 10 files.
	var ids []string
	for i := range 11 {
		ids = append(ids, e.upload(alice, ws, two.ID, fmt.Sprintf("f%d.txt", i), "text/plain", 1, true).AttachmentID)
	}
	if r := send(alice, ws, two.ID, ids...); r.Code != 400 {
		t.Errorf("11 files = %d %s, want 400", r.Code, r.Body)
	}
	if r := send(alice, ws, two.ID, ids[:10]...); r.Code != 201 {
		t.Errorf("10 files = %d %s, want 201", r.Code, r.Body)
	}
}

func TestDownloadAttachment(t *testing.T) {
	e := newEnv(t)
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	mallory := e.member(other, "mallory")
	ch := e.createChannel(alice, ws, "private", "secret", bob.ID)
	up := e.upload(alice, ws, ch.ID, "รายงาน.pdf", "application/pdf", 100, true)
	get := func(u user, w string) resp { return e.call("GET", "/chat/attachments/"+up.AttachmentID, u, w, nil) }

	// Before it is sent, only the uploader may fetch it.
	if r := get(alice, ws); r.Code != 200 {
		t.Errorf("uploader before send = %d %s", r.Code, r.Body)
	}
	if r := get(bob, ws); r.Code != 404 {
		t.Errorf("member before send = %d %s, want 404", r.Code, r.Body)
	}
	m := e.sendFiles(alice, ws, ch.ID, uuid.NewString(), "", up.AttachmentID)
	var msg messageBody
	m.json(t, &msg)

	r := get(bob, ws)
	var link struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	r.json(t, &link)
	if r.Code != 200 || !strings.Contains(link.URL, "chat/"+ws+"/"+up.AttachmentID) || time.Until(link.ExpiresAt) < 14*time.Minute {
		t.Fatalf("member = %d %s", r.Code, r.Body)
	}
	last := e.storage.gets[len(e.storage.gets)-1]
	if last.FileName != "รายงาน.pdf" || last.TTL != 15*time.Minute {
		t.Errorf("presign get = %+v", last)
	}

	// NFR-06 and channel visibility: everyone else gets 404.
	if r := get(carol, ws); r.Code != 404 {
		t.Errorf("not in the private channel = %d %s, want 404", r.Code, r.Body)
	}
	if r := get(mallory, other); r.Code != 404 {
		t.Errorf("other workspace = %d %s, want 404", r.Code, r.Body)
	}
	if r := e.call("GET", "/chat/attachments/"+uuid.NewString(), alice, ws, nil); r.Code != 404 {
		t.Errorf("unknown = %d, want 404", r.Code)
	}
	if r := e.call("GET", "/chat/attachments/nope", alice, ws, nil); r.Code != 404 {
		t.Errorf("malformed = %d, want 404", r.Code)
	}

	// A deleted message takes its files with it.
	if r := e.call("DELETE", "/chat/messages/"+msg.ID, alice, ws, nil); r.Code != 204 {
		t.Fatal(r.Code)
	}
	if r := get(bob, ws); r.Code != 404 {
		t.Errorf("after delete = %d %s, want 404", r.Code, r.Body)
	}
	if got := e.messages(bob, ws, ch.ID, "").Items; len(got) != 1 || len(got[0].Attachments) != 0 {
		t.Errorf("deleted message still lists files: %+v", got)
	}
}
