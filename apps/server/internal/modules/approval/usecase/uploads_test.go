package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

func TestRequestUploadRecordsTheFileAndPresigns(t *testing.T) {
	r := newRig(t)
	up, err := r.svc.RequestUpload(context.Background(), p(alice), usecase.UploadInput{FileName: `C:\x\medical cert.pdf`, Size: 4096, MimeType: "Application/PDF"})
	if err != nil {
		t.Fatal(err)
	}
	key := "approval/" + ws + "/" + up.AttachmentID
	if len(r.store.puts) != 1 || r.store.puts[0].Key != key || r.store.puts[0].Size != 4096 || r.store.puts[0].ContentType != "application/pdf" || r.store.puts[0].TTL != usecase.URLTTL {
		t.Errorf("presign calls = %+v", r.store.puts)
	}
	if up.Request.URL == "" || up.Request.Method != "PUT" {
		t.Errorf("request = %+v", up.Request)
	}
	a := r.repo.attachments[up.AttachmentID]
	if a.UploaderID != alice || a.WorkspaceID != ws || a.RequestID != "" || a.FileName != "medical cert.pdf" || a.ObjectKey != key || a.Size != 4096 {
		t.Errorf("attachment = %+v", a)
	}
}

func TestRequestUploadValidates(t *testing.T) {
	r := newRig(t)
	for name, in := range map[string]usecase.UploadInput{
		"empty size": {FileName: "a.pdf", Size: 0, MimeType: "application/pdf"},
		"too big":    {FileName: "a.pdf", Size: domain.MaxUploadBytes + 1, MimeType: "application/pdf"},
		"no name":    {FileName: " ", Size: 1, MimeType: "application/pdf"},
		"bad type":   {FileName: "a.pdf", Size: 1, MimeType: "pdf"},
	} {
		if _, err := r.svc.RequestUpload(context.Background(), p(alice), in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
	if len(r.repo.attachments) != 0 || len(r.store.puts) != 0 {
		t.Error("a refused upload left a row or a URL")
	}
}

func TestRequestUploadWithoutStorageFails(t *testing.T) {
	r := newRig(t)
	r.svc = newServiceWithoutStorage(r)
	if _, err := r.svc.RequestUpload(context.Background(), p(alice), usecase.UploadInput{FileName: "a.pdf", Size: 1, MimeType: "application/pdf"}); err == nil {
		t.Fatal("expected an error")
	}
	if len(r.repo.attachments) != 0 {
		t.Error("a row was stored for an upload that cannot happen")
	}
}

func TestAttachmentURL(t *testing.T) {
	r := newRig(t)
	a := r.uploaded(t, alice, 100)

	// Before submit only the uploader can see the file.
	if _, err := r.svc.AttachmentURL(context.Background(), p(alice), a); err != nil {
		t.Errorf("uploader before submit: %v", err)
	}
	if _, err := r.svc.AttachmentURL(context.Background(), p(bob), a); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("approver before submit = %v, want ErrNotFound", err)
	}

	in := submitIn(bob)
	in.AttachmentIDs = []string{a}
	if _, err := r.svc.Submit(context.Background(), p(alice), in); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{alice, bob} {
		req, err := r.svc.AttachmentURL(context.Background(), p(who), a)
		if err != nil || req.URL == "" || req.Method != "GET" {
			t.Errorf("%s: %+v, %v", who, req, err)
		}
	}
	if got := r.store.getName[len(r.store.getName)-1]; got != "cert.pdf" {
		t.Errorf("download name = %q", got)
	}
	if got := r.store.gets[len(r.store.gets)-1]; got != "approval/"+ws+"/"+a {
		t.Errorf("key = %q", got)
	}
	for _, who := range []string{carol, manager} {
		if _, err := r.svc.AttachmentURL(context.Background(), p(who), a); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", who, err)
		}
	}
	if _, err := r.svc.AttachmentURL(context.Background(), app.Principal{UserID: alice, WorkspaceID: otherWS}, a); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("other workspace = %v, want ErrNotFound", err)
	}
	if _, err := r.svc.AttachmentURL(context.Background(), p(alice), "nope"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("bad ID = %v, want ErrNotFound", err)
	}
}
