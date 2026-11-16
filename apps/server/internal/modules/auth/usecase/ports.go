// Package usecase holds the auth flows and the ports they need.
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
)

// ProfilePatch carries the fields PATCH /auth/me may change. A nil field is
// left as it is.
type ProfilePatch struct {
	DisplayName *string
	AvatarURL   *string
	Locale      *string
	Timezone    *string
}

// Repository stores users, devices and refresh tokens. Every method takes the
// transaction from ctx when there is one. Methods that look up one row return
// an error wrapping app.ErrNotFound when it does not exist; CreateUser returns
// one wrapping app.ErrConflict when the email is taken.
type Repository interface {
	CreateUser(ctx context.Context, u domain.User) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	UserByID(ctx context.Context, id string) (domain.User, error)
	UsersByIDs(ctx context.Context, ids []string) ([]domain.User, error)
	UpdateProfile(ctx context.Context, id string, p ProfilePatch, now time.Time) (domain.User, error)

	CreateDevice(ctx context.Context, d domain.Device) (domain.Device, error)
	// ActiveDevice returns not found for a revoked device.
	ActiveDevice(ctx context.Context, id string) (domain.Device, error)
	TouchDevice(ctx context.Context, id string, now time.Time) error
	// ActiveDevices lists a user's devices that are not revoked, latest first.
	ActiveDevices(ctx context.Context, userID string) ([]domain.Device, error)
	// RevokeDevice revokes a device of userID. Another user's device and an
	// already revoked one are both not found.
	RevokeDevice(ctx context.Context, userID, deviceID string, now time.Time) error
	RevokeDeviceByID(ctx context.Context, deviceID string, now time.Time) error
	RevokeUserDevices(ctx context.Context, userID string, now time.Time) error

	CreateRefreshToken(ctx context.Context, t domain.RefreshToken) error
	// RefreshTokenForUpdate locks the row until the transaction ends, so two
	// concurrent refreshes with one token cannot both succeed.
	RefreshTokenForUpdate(ctx context.Context, hash []byte) (domain.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string, now time.Time) error
	RevokeDeviceRefreshTokens(ctx context.Context, deviceID string, now time.Time) error
	RevokeUserRefreshTokens(ctx context.Context, userID string, now time.Time) error
}

// Events announces session changes. Methods are called after the transaction
// commits, and never for a change that did nothing.
type Events interface {
	// DeviceRevoked is called once when a device's sessions were revoked. An
	// empty deviceID means every device of the user was revoked (AUTH-08).
	DeviceRevoked(ctx context.Context, userID, deviceID string)
}

// LoginLimiter counts failed logins (AUTH-06). Keys are "email:<address>" and
// "ip:<address>". The limit and window belong to the implementation.
type LoginLimiter interface {
	// Blocked reports whether key has used up its failures in the window.
	Blocked(ctx context.Context, key string) (bool, error)
	// Fail records one failed login for key.
	Fail(ctx context.Context, key string) error
}

// Claims are what an access token proves.
type Claims struct {
	UserID   string
	DeviceID string
}

// AccessTokens signs and verifies access tokens.
type AccessTokens interface {
	// Issue returns a signed token that expires ttl after now.
	Issue(c Claims, now time.Time, ttl time.Duration) (string, error)
	// Parse verifies signature, algorithm and expiry at now. Any failure wraps
	// app.ErrUnauthorized.
	Parse(token string, now time.Time) (Claims, error)
}
