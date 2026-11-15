package ratelimit

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Redis is a Limiter backed by Redis, so every server instance shares the
// same counters (NFR-04).
//
// Each call runs INCR and PEXPIRE NX in one MULTI/EXEC pipeline. PEXPIRE NX
// sets the expiry only when the key has none, so the first call starts the
// window, later calls (allowed or denied) do not move it, and a key can never
// be left without an expiry. It needs Redis 7.0 or later. PEXPIRE is used
// because go-redis's ExpireNX rounds the window to whole seconds; the window
// is rounded up to a whole millisecond.
type Redis struct {
	client goredis.Cmdable
	prefix string
}

// NewRedis returns a Limiter that stores counters under prefix+key.
func NewRedis(client goredis.Cmdable, prefix string) *Redis {
	return &Redis{client: client, prefix: prefix}
}

// Allow implements Limiter.
func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if err := validate(limit, window); err != nil {
		return false, err
	}
	k := r.prefix + key
	var incr *goredis.IntCmd
	_, err := r.client.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		incr = p.Incr(ctx, k)
		// go-redis has no PExpireNX helper, so send the command itself.
		p.Do(ctx, "PEXPIRE", k, int64((window+time.Millisecond-1)/time.Millisecond), "NX")
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("ratelimit: redis: %w", err)
	}
	return incr.Val() <= int64(limit), nil
}
