package app

import "context"

type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleManager Role = "manager"
	RoleMember  Role = "member"
	RoleGuest   Role = "guest"
)

// Principal is the authenticated caller. Use cases read WorkspaceID from
// here only, never from client input.
type Principal struct {
	UserID      string
	DeviceID    string
	WorkspaceID string
	Role        Role
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Identity is who sent a request, before any workspace is involved.
type Identity struct {
	UserID   string
	DeviceID string
}

// Authenticator turns an access token into an Identity. The auth module
// provides it through Services. It returns an error wrapping ErrUnauthorized
// for a missing, malformed, expired or revoked token (including a token whose
// device was logged out).
type Authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (Identity, error)
}

// MembershipResolver returns a user's role in a workspace. The workspace
// module provides it through Services. It returns an error wrapping
// ErrForbidden when the user is not an active member, so a deactivated member
// loses access on their next request (WSP-08). It must not cache.
type MembershipResolver interface {
	Role(ctx context.Context, workspaceID, userID string) (Role, error)
}
