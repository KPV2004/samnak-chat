// Package auth owns accounts, devices and tokens (SRS section 5).
// This file and events.go are the only parts other modules may import.
package auth

import "context"

// UserSummary is the public view of a user that other modules may show.
type UserSummary struct {
	ID          string
	Email       string
	DisplayName string
	AvatarURL   string
}

// Directory looks up users by ID. auth provides it through app.Services;
// consumers must handle Lookup returning false.
type Directory interface {
	// Users returns the users that exist among ids, in no particular order.
	// Unknown IDs are skipped, not an error.
	Users(ctx context.Context, ids []string) ([]UserSummary, error)
}
