package usecase

import (
	"errors"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Error is a standard error (app.ErrInvalid and friends) with a message that
// is safe to show to the caller.
type Error struct {
	kind error
	msg  string
}

func (e *Error) Error() string { return e.msg }
func (e *Error) Unwrap() error { return e.kind }

func invalid(err error) error { return &Error{kind: app.ErrInvalid, msg: err.Error()} }

// errBadCredentials is the one answer for a wrong email and a wrong password.
var errBadCredentials = &Error{kind: app.ErrUnauthorized, msg: "invalid email or password"}

var errRateLimited = &Error{kind: app.ErrRateLimited, msg: "too many failed logins, try again in a minute"}

var errEmailTaken = &Error{kind: app.ErrConflict, msg: "email is already registered"}

var errBadRefresh = &Error{kind: app.ErrUnauthorized, msg: "invalid refresh token"}

var errDeviceNotFound = &Error{kind: app.ErrNotFound, msg: "device not found"}

// errBadEmailToken answers every link that cannot be used. It does not say why.
var errBadEmailToken = &Error{kind: app.ErrInvalid, msg: "the link is invalid or has expired"}

var errResendLimited = &Error{kind: app.ErrRateLimited, msg: "too many verification emails, try again in a minute"}

func notFound(err error) bool { return errors.Is(err, app.ErrNotFound) }
