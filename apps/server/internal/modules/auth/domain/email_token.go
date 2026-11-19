package domain

import "time"

// EmailPurpose says what an emailed link does (AUTH-09). A link works for its
// purpose only.
type EmailPurpose string

const (
	// EmailPurposeVerify confirms the address of the account.
	EmailPurposeVerify EmailPurpose = "verify"
	// EmailPurposeReset sets a new password.
	EmailPurposeReset EmailPurpose = "reset"
)

// EmailToken is the server side of an emailed link. Only the SHA-256 of the
// token is kept.
type EmailToken struct {
	ID        string
	UserID    string
	Purpose   EmailPurpose
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
}
