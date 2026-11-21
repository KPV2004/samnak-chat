package ratelimit_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/ratelimit"
)

// tokens maps bearer tokens to user IDs for the tests; anything else is
// rejected, as the auth module rejects an invalid token.
var tokens = map[string]string{"good-alice": "alice", "good-bob": "bob"}

func resolve(_ context.Context, token string) (string, bool) {
	id, ok := tokens[token]
	return id, ok
}

func options(perUser, perIP int) ratelimit.Options {
	return ratelimit.Options{
		Logger:  slog.New(slog.DiscardHandler),
		Limiter: ratelimit.NewMemory(),
		PerUser: perUser,
		PerIP:   perIP,
		Window:  time.Minute,
		UserID:  resolve,
	}
}

// serve sends one request through the middleware and returns the response.
func serve(t *testing.T, h http.Handler, method, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.0.2.10:5555"
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func withBearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func TestMiddlewareAnswers429WithRetryAfter(t *testing.T) {
	h := ratelimit.Middleware(options(100, 2))(http.HandlerFunc(ok))
	for i := 1; i <= 2; i++ {
		if rec := serve(t, h, "GET", "/api/v1/meta", nil); rec.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i, rec.Code)
		}
	}
	rec := serve(t, h, "GET", "/api/v1/meta", nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != "rate_limited" {
		t.Fatalf("error code = %q, want rate_limited", body.Error.Code)
	}
}

func TestMiddlewareRetryAfterRoundsUpToWholeSeconds(t *testing.T) {
	o := options(100, 1)
	o.Window = 1500 * time.Millisecond
	h := ratelimit.Middleware(o)(http.HandlerFunc(ok))
	serve(t, h, "GET", "/api/v1/meta", nil)
	if got := serve(t, h, "GET", "/api/v1/meta", nil).Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
}

func TestMiddlewareKeysSignedOutClientsByIP(t *testing.T) {
	h := ratelimit.Middleware(options(100, 1))(http.HandlerFunc(ok))
	serve(t, h, "GET", "/api/v1/meta", nil)
	if rec := serve(t, h, "GET", "/api/v1/meta", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same IP: status = %d, want 429", rec.Code)
	}
	other := func(r *http.Request) { r.RemoteAddr = "198.51.100.1:4444" }
	if rec := serve(t, h, "GET", "/api/v1/meta", other); rec.Code != http.StatusOK {
		t.Fatalf("other IP: status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareKeysSignedInUsersByUserNotIP(t *testing.T) {
	// The per-IP limit is 1, so a second signed-out call from one address
	// would be denied. Signed-in calls count against their user instead.
	h := ratelimit.Middleware(options(2, 1))(http.HandlerFunc(ok))
	for i := 1; i <= 2; i++ {
		if rec := serve(t, h, "GET", "/api/v1/meta", withBearer("good-alice")); rec.Code != http.StatusOK {
			t.Fatalf("alice call %d: status = %d, want 200", i, rec.Code)
		}
	}
	if rec := serve(t, h, "GET", "/api/v1/meta", withBearer("good-alice")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("alice over her limit: status = %d, want 429", rec.Code)
	}
	// Bob shares the address but has his own count.
	if rec := serve(t, h, "GET", "/api/v1/meta", withBearer("good-bob")); rec.Code != http.StatusOK {
		t.Fatalf("bob from the same IP: status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareCountsAnInvalidTokenAgainstTheIP(t *testing.T) {
	h := ratelimit.Middleware(options(100, 1))(http.HandlerFunc(ok))
	serve(t, h, "GET", "/api/v1/meta", withBearer("forged"))
	if rec := serve(t, h, "GET", "/api/v1/meta", withBearer("forged-2")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("invalid tokens from one IP: status = %d, want 429", rec.Code)
	}
}

func TestMiddlewareExemptsPathsOutsideAPIAndWebSocketUpgrades(t *testing.T) {
	h := ratelimit.Middleware(options(100, 1))(http.HandlerFunc(ok))
	for i := 0; i < 5; i++ {
		for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
			if rec := serve(t, h, "GET", path, nil); rec.Code != http.StatusOK {
				t.Fatalf("%s call %d: status = %d, want 200", path, i+1, rec.Code)
			}
		}
		upgrade := func(r *http.Request) { r.Header.Set("Upgrade", "websocket") }
		if rec := serve(t, h, "GET", "/api/v1/ws", upgrade); rec.Code != http.StatusOK {
			t.Fatalf("websocket upgrade call %d: status = %d, want 200", i+1, rec.Code)
		}
	}
	// The same path without the upgrade header is an API call and counts.
	serve(t, h, "GET", "/api/v1/ws", nil)
	if rec := serve(t, h, "GET", "/api/v1/ws", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("plain call to /api/v1/ws: status = %d, want 429", rec.Code)
	}
}

func TestMiddlewareIgnoresForwardedForFromAnUntrustedPeer(t *testing.T) {
	h := ratelimit.Middleware(options(100, 1))(http.HandlerFunc(ok))
	spoof := func(ip string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("X-Forwarded-For", ip) }
	}
	serve(t, h, "GET", "/api/v1/meta", spoof("203.0.113.1"))
	if rec := serve(t, h, "GET", "/api/v1/meta", spoof("203.0.113.2")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged X-Forwarded-For from an untrusted peer: status = %d, want 429", rec.Code)
	}
}

func TestMiddlewareKeysByForwardedClientBehindATrustedProxy(t *testing.T) {
	o := options(100, 1)
	o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := ratelimit.Middleware(o)(http.HandlerFunc(ok))
	fromProxy := func(xff string) func(*http.Request) {
		return func(r *http.Request) {
			r.RemoteAddr = "10.1.2.3:4000"
			r.Header.Set("X-Forwarded-For", xff)
		}
	}
	serve(t, h, "GET", "/api/v1/meta", fromProxy("203.0.113.7"))
	if rec := serve(t, h, "GET", "/api/v1/meta", fromProxy("203.0.113.7")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same client through the proxy: status = %d, want 429", rec.Code)
	}
	if rec := serve(t, h, "GET", "/api/v1/meta", fromProxy("198.51.100.9")); rec.Code != http.StatusOK {
		t.Fatalf("other client through the proxy: status = %d, want 200", rec.Code)
	}
	// The client prepends a fake address; the proxy appended the real one last,
	// so the real one is the key and the limit still applies.
	if rec := serve(t, h, "GET", "/api/v1/meta", fromProxy("192.0.2.99, 203.0.113.7")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("client prepending an address: status = %d, want 429", rec.Code)
	}
}

func TestMiddlewareSkipsTrustedProxiesInAForwardedChain(t *testing.T) {
	o := options(100, 1)
	o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := ratelimit.Middleware(o)(http.HandlerFunc(ok))
	chain := func(r *http.Request) {
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2")
	}
	serve(t, h, "GET", "/api/v1/meta", chain)
	if rec := serve(t, h, "GET", "/api/v1/meta", chain); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: the client is 203.0.113.7", rec.Code)
	}
}

func TestMiddlewareFallsBackToThePeerWhenForwardedForIsMalformed(t *testing.T) {
	o := options(100, 1)
	o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")}
	h := ratelimit.Middleware(o)(http.HandlerFunc(ok))
	garbled := func(r *http.Request) {
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", "not-an-address")
	}
	serve(t, h, "GET", "/api/v1/meta", garbled)
	// The key is the proxy itself, so the next request through it counts too.
	if rec := serve(t, h, "GET", "/api/v1/meta", garbled); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: the key is the proxy", rec.Code)
	}
}

// failing is a Limiter that cannot decide.
type failing struct{}

func (failing) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return false, errors.New("redis is down")
}

func TestMiddlewareFailsOpenWhenTheLimiterErrors(t *testing.T) {
	o := options(1, 1)
	o.Limiter = failing{}
	h := ratelimit.Middleware(o)(http.HandlerFunc(ok))
	for i := 1; i <= 3; i++ {
		if rec := serve(t, h, "GET", "/api/v1/meta", nil); rec.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 while the limiter is down", i, rec.Code)
		}
	}
}

func TestMiddlewareRejectsOptionsThatCannotLimit(t *testing.T) {
	for name, mutate := range map[string]func(*ratelimit.Options){
		"no limiter":    func(o *ratelimit.Options) { o.Limiter = nil },
		"zero per user": func(o *ratelimit.Options) { o.PerUser = 0 },
		"zero per IP":   func(o *ratelimit.Options) { o.PerIP = 0 },
		"zero window":   func(o *ratelimit.Options) { o.Window = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			o := options(1, 1)
			mutate(&o)
			defer func() {
				if recover() == nil {
					t.Fatal("Middleware did not panic")
				}
			}()
			ratelimit.Middleware(o)
		})
	}
}
