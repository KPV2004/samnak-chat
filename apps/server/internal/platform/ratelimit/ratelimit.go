package ratelimit

import (
	"context"
	"errors"
	"time"
)

// Limiter counts calls per key. Both implementations use a fixed window: the
// first call for a key starts a window of the given length, and calls inside
// it are counted. When the window ends, the count starts again at zero.
//
// A fixed window is cheap and easy to reason about. Its known weakness is that
// a caller can make up to twice the limit across a window boundary. That is
// acceptable for failed-login limits (AUTH-06); use a sliding window if a
// feature needs a hard bound.
type Limiter interface {
	// Allow records one call for key and reports whether it is within limit
	// calls per window. Denied calls are counted but never extend the window.
	// It returns false together with an error when it cannot decide, so the
	// caller chooses whether to fail open or closed.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

var errInvalidArgs = errors.New("ratelimit: limit and window must be positive")

func validate(limit int, window time.Duration) error {
	if limit <= 0 || window <= 0 {
		return errInvalidArgs
	}
	return nil
}
