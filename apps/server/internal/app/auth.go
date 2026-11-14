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
