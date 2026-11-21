package ratelimit

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// Options sets the API-wide limit on /api/v1 (NFR-07).
type Options struct {
	// Logger reports limiter failures.
	Logger *slog.Logger
	// Limiter counts the calls. Use Redis so every instance shares the counts.
	Limiter Limiter
	// PerUser is how many calls a signed-in user may make per Window.
	PerUser int
	// PerIP is how many calls a signed-out client may make per Window.
	PerIP int
	// Window is the length of the count. A 429 says to retry after it.
	Window time.Duration
	// TrustedProxies are the peers whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
	// UserID turns a bearer token into the user it belongs to. It returns
	// false for a token it does not accept, and the request then counts
	// against its IP. Nil means every request counts against its IP.
	UserID func(ctx context.Context, token string) (string, bool)
}

// Middleware limits every /api/v1 request except a WebSocket upgrade. A request
// with a valid bearer token counts against its user; any other counts against
// its client IP. Over the limit it answers 429 with Retry-After.
//
// When the limiter fails, the request goes through and the error is logged.
// Redis being down already fails /readyz, so the API stays up instead.
func Middleware(o Options) func(http.Handler) http.Handler {
	if o.Limiter == nil || o.Logger == nil || o.PerUser < 1 || o.PerIP < 1 || o.Window <= 0 {
		panic("ratelimit: Middleware needs a limiter, a logger, positive limits and a positive window")
	}
	retryAfter := strconv.Itoa(int(math.Ceil(o.Window.Seconds())))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limited(r) {
				next.ServeHTTP(w, r)
				return
			}
			key, limit := o.subject(r)
			ok, err := o.Limiter.Allow(r.Context(), key, limit, o.Window)
			if err != nil {
				o.Logger.WarnContext(r.Context(), "ratelimit: check failed, letting the request through", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !ok {
				w.Header().Set("Retry-After", retryAfter)
				httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
					"error": map[string]string{"code": "rate_limited", "message": "too many requests, try again in " + retryAfter + " seconds"},
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limited reports whether the request is under /api/v1 and is not a WebSocket
// upgrade. /healthz, /readyz and /metrics stay out, so probes and scrapes
// never see a 429.
func limited(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		return false
	}
	return !strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// subject returns the key a request counts against and the limit for it.
func (o Options) subject(r *http.Request) (string, int) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if found && token != "" && o.UserID != nil {
		if id, ok := o.UserID(r.Context(), token); ok {
			return "user:" + id, o.PerUser
		}
	}
	return "ip:" + clientIP(r, o.TrustedProxies), o.PerIP
}

// clientIP is the address the request came from. X-Forwarded-For is read only
// when the connection comes from a trusted proxy. A proxy appends the address
// it received from, so the rightmost entry that is not a trusted proxy is the
// client. Entries to its left were written by the client and are ignored.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		ip = ip.Unmap()
		if !isTrusted(ip, trusted) {
			return ip.String()
		}
	}
	return peer.String()
}

func isTrusted(a netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
