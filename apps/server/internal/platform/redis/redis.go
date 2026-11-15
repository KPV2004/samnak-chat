package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Open creates a Redis client from url and pings it.
func Open(ctx context.Context, url string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	c := goredis.NewClient(opts)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}
	return c, nil
}

// Check returns a readiness check function, for httpx.Check.Fn.
func Check(c *goredis.Client) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if err := c.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis: ping: %w", err)
		}
		return nil
	}
}

// RateLimiter limits how often a key may be used. Use cases depend on this
// interface and tests replace it with a fake.
type RateLimiter interface {
	// Allow counts one use of key and reports whether it is within limit uses
	// per window. A denied call still counts as a use of the current window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// allowScript counts a use and sets the expiry on the first one in a single
// step, so a crash between the two commands cannot leave a key without a TTL.
var allowScript = goredis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n
`)

// FixedWindow is a RateLimiter that counts uses in a fixed window that starts
// at the first use of a key.
type FixedWindow struct{ c *goredis.Client }

// NewRateLimiter returns a fixed-window RateLimiter backed by c.
func NewRateLimiter(c *goredis.Client) *FixedWindow { return &FixedWindow{c: c} }

var _ RateLimiter = (*FixedWindow)(nil)

func (l *FixedWindow) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	n, err := allowScript.Run(ctx, l.c, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("redis: rate limit %q: %w", key, err)
	}
	return n <= int64(limit), nil
}
