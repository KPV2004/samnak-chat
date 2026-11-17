package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

func pending() domain.Request {
	return domain.Request{
		ID: "r1", RequesterID: "alice", ApproverID: "bob", Status: domain.StatusPending,
		UpdatedAt: time.UnixMilli(1_700_000_000_123).UTC(),
	}
}

func TestCheckDecide(t *testing.T) {
	r := pending()
	if err := r.CheckDecide("bob"); err != nil {
		t.Errorf("approver on a pending request: %v", err)
	}
	for _, who := range []string{"alice", "carol", ""} {
		if err := r.CheckDecide(who); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("CheckDecide(%q) = %v, want ErrForbidden", who, err)
		}
	}
	for _, st := range []domain.Status{domain.StatusApproved, domain.StatusRejected, domain.StatusCancelled} {
		r.Status = st
		if err := r.CheckDecide("bob"); !errors.Is(err, app.ErrConflict) {
			t.Errorf("CheckDecide on %s = %v, want ErrConflict", st, err)
		}
	}
	// A third user is refused before the status is looked at.
	if err := r.CheckDecide("carol"); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("CheckDecide by a third user on a decided request = %v, want ErrForbidden", err)
	}
}

func TestCheckCancel(t *testing.T) {
	r := pending()
	if err := r.CheckCancel("alice"); err != nil {
		t.Errorf("requester on a pending request: %v", err)
	}
	for _, who := range []string{"bob", "carol"} {
		if err := r.CheckCancel(who); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("CheckCancel(%q) = %v, want ErrForbidden", who, err)
		}
	}
	r.Status = domain.StatusApproved
	if err := r.CheckCancel("alice"); !errors.Is(err, app.ErrConflict) {
		t.Errorf("CheckCancel on approved = %v, want ErrConflict", err)
	}
}

func TestCanSee(t *testing.T) {
	r := pending()
	for who, want := range map[string]bool{"alice": true, "bob": true, "carol": false, "": false} {
		if got := r.CanSee(who); got != want {
			t.Errorf("CanSee(%q) = %v, want %v", who, got, want)
		}
	}
}

func TestVersionIsUpdatedAtInUnixMillis(t *testing.T) {
	if got := pending().Version(); got != 1_700_000_000_123 {
		t.Errorf("Version = %d", got)
	}
}

func TestNormalizeReason(t *testing.T) {
	if got, err := domain.NormalizeReason("  too busy "); err != nil || got != "too busy" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, in := range []string{"", "  \n\t", strings.Repeat("a", 1001)} {
		if _, err := domain.NormalizeReason(in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeReason(%d chars) = %v, want ErrInvalid", len(in), err)
		}
	}
	if _, err := domain.NormalizeReason(strings.Repeat("a", 1000)); err != nil {
		t.Errorf("1000 characters should pass: %v", err)
	}
}

func TestApproverRequiredIsInvalid(t *testing.T) {
	if !errors.Is(domain.ErrApproverRequired, app.ErrInvalid) {
		t.Error("ErrApproverRequired must map to 400 through app.ErrInvalid")
	}
}

func TestObjectKeyStartsWithModuleName(t *testing.T) {
	if got := domain.ObjectKey("w1", "a1"); got != "approval/w1/a1" {
		t.Errorf("ObjectKey = %q", got)
	}
}

func TestUploadChecks(t *testing.T) {
	if name, err := domain.NormalizeFileName(`C:\docs\medical cert.pdf`); err != nil || name != "medical cert.pdf" {
		t.Errorf("NormalizeFileName = %q, %v", name, err)
	}
	for _, in := range []string{"", "..", "a\x00b"} {
		if _, err := domain.NormalizeFileName(in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeFileName(%q) = %v, want ErrInvalid", in, err)
		}
	}
	if mt, err := domain.NormalizeMimeType("Application/PDF; charset=x"); err != nil || mt != "application/pdf" {
		t.Errorf("NormalizeMimeType = %q, %v", mt, err)
	}
	if _, err := domain.NormalizeMimeType("pdf"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("NormalizeMimeType(pdf) = %v", err)
	}
	for size, ok := range map[int64]bool{0: false, 1: true, domain.MaxUploadBytes: true, domain.MaxUploadBytes + 1: false} {
		if err := domain.ValidateUploadSize(size); (err == nil) != ok {
			t.Errorf("ValidateUploadSize(%d) = %v", size, err)
		}
	}
}
