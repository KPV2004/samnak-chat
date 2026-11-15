package domain

import "time"

// Device is one login of one user on one client. Its ID is the session.
type Device struct {
	ID         string
	UserID     string
	Name       string
	Platform   string
	LastSeenAt time.Time
	CreatedAt  time.Time
}

// RefreshToken is the server side of a refresh token. Only the SHA-256 of the
// token is kept.
type RefreshToken struct {
	ID        string
	DeviceID  string
	TokenHash []byte
	ExpiresAt time.Time
	RevokedAt *time.Time
}
