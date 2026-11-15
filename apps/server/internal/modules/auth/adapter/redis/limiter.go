// Package redis has the Redis adapters of the auth module. The limiter lets
// every server instance share one failed-login count (NFR-04).
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const keyPrefix = "auth:login-fail:"

// LoginLimiter counts failures per key in a fixed window that starts at the
// first failure. It implements usecase.LoginLimiter.
//
// Fail runs INCR and PEXPIRE NX in one MULTI/EXEC pipeline. PEXPIRE NX sets
// the expiry only when the key has none, so the first failure starts the
// window, later ones do not move it, and a key is never left without an
// expiry. It needs Redis 7.0 or later.
type LoginLimiter struct {
	client goredis.Cmdable
	limit  int64
	window time.Duration
}

// NewLoginLimiter blocks a key after limit failures within window.
func NewLoginLimiter(client goredis.Cmdable, limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{client: client, limit: int64(limit), window: window}
}

func (l *LoginLimiter) Blocked(ctx context.Context, key string) (bool, error) {
	n, err := l.client.Get(ctx, keyPrefix+key).Int64()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("auth: login limiter: %w", err)
	}
	return n >= l.limit, nil
}

func (l *LoginLimiter) Fail(ctx context.Context, key string) error {
	k := keyPrefix + key
	_, err := l.client.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		p.Incr(ctx, k)
		// go-redis has no PExpireNX helper, and ExpireNX rounds to seconds.
		p.Do(ctx, "PEXPIRE", k, int64((l.window+time.Millisecond-1)/time.Millisecond), "NX")
		return nil
	})
	if err != nil {
		return fmt.Errorf("auth: login limiter: %w", err)
	}
	return nil
}
