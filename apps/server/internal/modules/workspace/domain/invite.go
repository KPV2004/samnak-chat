package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// InviteTTL is how long an invite stays valid (WSP-02).
const InviteTTL = 7 * 24 * time.Hour

// Invite lets someone join a workspace with a role. Email is empty for a
// link invite that any signed-in user may accept once.
type Invite struct {
	ID          string
	WorkspaceID string
	Email       string
	Role        app.Role
	CreatedBy   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
	UsedAt      *time.Time
}

// ValidateInviteRole allows admin, member and guest. An invite never grants
// owner (or manager, which is assigned after joining).
func ValidateInviteRole(r app.Role) error {
	switch r {
	case app.RoleAdmin, app.RoleMember, app.RoleGuest:
		return nil
	}
	return fmt.Errorf("invite role must be admin, member or guest: %w", app.ErrInvalid)
}

// NormalizeInviteEmail trims and lower-cases a plain address such as
// "a@b.com". An empty input stays empty (link invite).
func NormalizeInviteEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", fmt.Errorf("email is not valid: %w", app.ErrInvalid)
	}
	return strings.ToLower(s), nil
}

// NewToken returns a random invite token: 32 bytes, base64url without
// padding. Only HashToken of it is stored.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err)) // never happens on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the hex SHA-256 of a raw token. The token is already 256
// bits of randomness, so a plain digest is enough.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Usable reports whether the invite is neither revoked, used nor expired.
func (i Invite) Usable(now time.Time) bool {
	return i.RevokedAt == nil && i.UsedAt == nil && now.Before(i.ExpiresAt)
}

// CheckAccept decides whether the user with the given email may accept the
// invite at now. An unusable invite is ErrNotFound, so a probe cannot tell
// expired from unknown; an email invite for someone else is ErrForbidden.
func (i Invite) CheckAccept(now time.Time, email string) error {
	if !i.Usable(now) {
		return fmt.Errorf("invite: %w", app.ErrNotFound)
	}
	if i.Email != "" && !strings.EqualFold(i.Email, strings.TrimSpace(email)) {
		return fmt.Errorf("this invite is for another email address: %w", app.ErrForbidden)
	}
	return nil
}
