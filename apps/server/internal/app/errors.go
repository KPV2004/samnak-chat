package app

import "errors"

// Standard errors. httpx maps them to 400, 401, 403, 404, 409 and 429.
// Wrap them with fmt.Errorf("...: %w", app.ErrNotFound) to add detail.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid")
	// ErrUnauthorized means the caller is not authenticated.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrRateLimited means the caller exceeded a rate limit.
	ErrRateLimited = errors.New("rate limited")
)
