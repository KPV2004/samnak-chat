package app

import "errors"

// Standard errors. httpx maps them to 404, 403, 409 and 400.
// Wrap them with fmt.Errorf("...: %w", app.ErrNotFound) to add detail.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid")
)
