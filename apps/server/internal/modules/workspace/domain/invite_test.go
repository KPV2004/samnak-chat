package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

func TestInviteRoleCannotBeOwner(t *testing.T) {
	for _, r := range []app.Role{app.RoleAdmin, app.RoleMember, app.RoleGuest} {
		if err := domain.ValidateInviteRole(r); err != nil {
			t.Errorf("ValidateInviteRole(%s) = %v", r, err)
		}
	}
	for _, r := range []app.Role{app.RoleOwner, app.RoleManager, "", "boss"} {
		if err := domain.ValidateInviteRole(r); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("ValidateInviteRole(%q) = %v, want ErrInvalid", r, err)
		}
	}
}

func TestNormalizeInviteEmail(t *testing.T) {
	got, err := domain.NormalizeInviteEmail("  Somchai@Example.COM ")
	if err != nil || got != "somchai@example.com" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := domain.NormalizeInviteEmail("  "); err != nil || got != "" {
		t.Fatalf("empty email = %q, %v", got, err)
	}
	for _, s := range []string{"not-an-email", "a@", "@b.com", "Name <a@b.com>"} {
		if _, err := domain.NormalizeInviteEmail(s); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeInviteEmail(%q) = %v, want ErrInvalid", s, err)
		}
	}
}

func TestNewTokenIsRandomAndHashed(t *testing.T) {
	a, b := domain.NewToken(), domain.NewToken()
	if a == b || len(a) < 43 {
		t.Fatalf("tokens %q %q", a, b)
	}
	h1, h2 := domain.HashToken(a), domain.HashToken(a)
	if h1 == a || h1 != h2 || h1 == domain.HashToken(b) {
		t.Fatal("HashToken must be a deterministic one-way digest")
	}
}

func TestInviteCheckAccept(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	base := domain.Invite{ID: "i", WorkspaceID: "w", Role: app.RoleMember, ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(-time.Hour)}
	past, future := now.Add(-time.Second), now.Add(time.Second)

	withEmail := base
	withEmail.Email = "a@example.com"
	expired := base
	expired.ExpiresAt = now // expires exactly now: no longer usable
	revoked := base
	revoked.RevokedAt = &past
	used := base
	used.UsedAt = &past
	notYet := base
	notYet.RevokedAt = &future // a future timestamp is still revoked data; treat as revoked

	cases := []struct {
		name    string
		inv     domain.Invite
		email   string
		wantErr error
	}{
		{"link invite, anyone", base, "x@example.com", nil},
		{"email invite, same email", withEmail, "a@example.com", nil},
		{"email invite, case-insensitive", withEmail, "A@Example.com", nil},
		{"email invite, other email", withEmail, "b@example.com", app.ErrForbidden},
		{"expired", expired, "x@example.com", app.ErrNotFound},
		{"revoked", revoked, "x@example.com", app.ErrNotFound},
		{"used", used, "x@example.com", app.ErrNotFound},
		{"revoked with later timestamp", notYet, "x@example.com", app.ErrNotFound},
		{"expired beats wrong email", func() domain.Invite { e := withEmail; e.ExpiresAt = past; return e }(), "b@example.com", app.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.inv.CheckAccept(now, c.email); !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestInviteTTLIsSevenDays(t *testing.T) {
	if domain.InviteTTL != 7*24*time.Hour {
		t.Fatalf("InviteTTL = %v", domain.InviteTTL)
	}
}
