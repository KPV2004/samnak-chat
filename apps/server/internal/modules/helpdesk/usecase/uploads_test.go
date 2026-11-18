package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

func upload(t *testing.T, e *env, who, name string, size int64) usecase.Upload {
	t.Helper()
	up, err := e.svc.RequestUpload(context.Background(), e.p[who], usecase.UploadInput{FileName: name, Size: size, MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	return up
}

func TestUploadPresignsAFileOfTheHelpdeskAndKeepsItUnlinked(t *testing.T) {
	e := newEnv(t)
	up := upload(t, e, "bob", `C:\docs\photo.pdf`, 1024)
	if up.AttachmentID == "" || up.Request.Method != "PUT" || !strings.Contains(up.Request.URL, "helpdesk/"+ws+"/"+up.AttachmentID) {
		t.Fatalf("upload = %+v", up)
	}
	a, err := e.repo.Attachment(context.Background(), ws, up.AttachmentID)
	if err != nil {
		t.Fatal(err)
	}
	if a.FileName != "photo.pdf" || a.TicketID != "" || a.UploaderID != bob || a.FromMessage {
		t.Errorf("stored attachment = %+v", a)
	}
}

func TestUploadRefusesBadNamesTypesAndSizes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, in := range map[string]usecase.UploadInput{
		"blank name": {FileName: " ", Size: 1, MimeType: "text/plain"},
		"bad type":   {FileName: "a", Size: 1, MimeType: "pdf"},
		"empty":      {FileName: "a", Size: 0, MimeType: "text/plain"},
		"too big":    {FileName: "a", Size: 50<<20 + 1, MimeType: "text/plain"},
	} {
		if _, err := e.svc.RequestUpload(ctx, e.p["bob"], in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestTicketTakesItsUploadedFilesOnceAndOnlyWhenTheyAreThere(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	ctx := context.Background()
	up := upload(t, e, "carol", "log.txt", 10)

	// carol is the uploader, so the file is hers to attach; until its bytes
	// are in storage the ticket is refused.
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID}}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("file not uploaded yet: err = %v, want ErrInvalid", err)
	}
	e.store.put("helpdesk/"+ws+"/"+up.AttachmentID, 99)
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID}}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("size mismatch: err = %v, want ErrInvalid", err)
	}
	e.store.put("helpdesk/"+ws+"/"+up.AttachmentID, 10)
	tk, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(ctx, e.p["carol"], tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Attachments) != 1 || d.Attachments[0].ID != up.AttachmentID {
		t.Fatalf("attachments = %+v", d.Attachments)
	}
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "again", AttachmentIDs: []string{up.AttachmentID}}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("reusing a file: err = %v, want ErrInvalid", err)
	}
}

func TestSomeoneElsesFileIsNotFound(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	up := upload(t, e, "bob", "a.txt", 3)
	e.store.put("helpdesk/"+ws+"/"+up.AttachmentID, 3)
	if _, err := e.create(t, "carol", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID}}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestTooManyOrRepeatedFileIDsAreRefused(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	up := upload(t, e, "bob", "a.txt", 3)
	ids := make([]string, domain.MaxAttachments+1)
	for i := range ids {
		ids[i] = newID()
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: ids}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("too many: err = %v, want ErrInvalid", err)
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID, up.AttachmentID}}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("repeated: err = %v, want ErrInvalid", err)
	}
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{"nope"}}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("not a UUID: err = %v, want ErrInvalid", err)
	}
}

func TestDownloadsFollowTheTicketsVisibility(t *testing.T) {
	e := newEnv(t)
	_, c := e.workSetup(t) // carol is an agent of the queue
	ctx := context.Background()
	up := upload(t, e, "bob", "a.txt", 3)
	if _, err := e.svc.AttachmentURL(ctx, e.p["carol"], up.AttachmentID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("another person's unsent file: err = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.AttachmentURL(ctx, e.p["bob"], up.AttachmentID); err != nil {
		t.Errorf("uploader's own unsent file: %v", err)
	}
	e.store.put("helpdesk/"+ws+"/"+up.AttachmentID, 3)
	if _, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "x", AttachmentIDs: []string{up.AttachmentID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AttachmentURL(ctx, e.p["carol"], up.AttachmentID); err != nil {
		t.Errorf("agent of the queue: %v", err)
	}
	if _, err := e.svc.AttachmentURL(ctx, e.p["outside"], up.AttachmentID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger: err = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.AttachmentURL(ctx, e.p["bob"], newID()); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("missing file: err = %v, want ErrNotFound", err)
	}
}

func TestAMessageSendsItsFilesAsChatsOwnKeys(t *testing.T) {
	e := newEnv(t)
	_, c := e.setup(t)
	ctx := context.Background()
	msg := newID()
	chatKey := "chat/" + ws + "/file-1"
	e.msgs[msg] = app.MessageSnapshot{
		ID: msg, ChannelID: newID(), SenderID: bob, Body: "See attached",
		Attachments: []app.MessageAttachment{{FileName: "shot.png", MimeType: "image/png", Size: 42, ObjectKey: chatKey}},
	}
	tk, err := e.create(t, "bob", usecase.CreateInput{CategoryID: c.ID, Title: "x", SourceMessageID: msg})
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(ctx, e.p["bob"], tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Attachments) != 1 || !d.Attachments[0].FromMessage || d.Attachments[0].ObjectKey != chatKey {
		t.Fatalf("attachments = %+v; the message's file keeps chat's key", d.Attachments)
	}
	if _, err := e.svc.AttachmentURL(ctx, e.p["bob"], d.Attachments[0].ID); err != nil {
		t.Errorf("download of a message file: %v", err)
	}
}
