package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/config"
)

// limitOn is an API-wide limit that is on, with the per-IP count set by the
// test. The per-user count is high so the IP limit is the one being tested.
func limitOn(perIP int) config.RateLimit {
	return config.RateLimit{Enabled: true, PerUser: 1000, PerIP: perIP}
}

// call sends one request from remote and returns the response.
func call(h http.Handler, method, path, remote string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// moduleRoutes is one GET route per module that needs a signed-in user. Without
// a token it answers 401, which shows the request reached the module's route.
var moduleRoutes = map[string]string{
	"auth":      "/api/v1/auth/me",
	"workspace": "/api/v1/workspace/members",
	"chat":      "/api/v1/chat/channels",
	"notify":    "/api/v1/notify/unread-count",
	"task":      "/api/v1/task/items",
	"approval":  "/api/v1/approval/requests",
	"helpdesk":  "/api/v1/helpdesk/tickets",
}

func TestAPIRateLimitAnswers429OnEveryModule(t *testing.T) {
	mods := newModules()
	names := make([]string, 0, len(mods))
	for _, m := range mods {
		names = append(names, m.Name())
	}
	h, err := newHandler(testLogger(), names, mods, infra{
		Platform:  map[string]any{"jwt_secret": "test-secret-test-secret-test-secret"},
		RateLimit: limitOn(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Each module gets its own client address, so one module's calls do not
	// use up another's allowance.
	for i, name := range names {
		path, ok := moduleRoutes[name]
		if !ok {
			t.Fatalf("no route listed for module %s", name)
		}
		remote := fmt.Sprintf("198.51.100.%d:4000", i+1)
		t.Run(name, func(t *testing.T) {
			for n := 1; n <= 2; n++ {
				if rec := call(h, "GET", path, remote, nil); rec.Code != http.StatusUnauthorized {
					t.Fatalf("call %d: status = %d, want 401 from the module", n, rec.Code)
				}
			}
			rec := call(h, "GET", path, remote, nil)
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429 over the limit", rec.Code)
			}
			if got := rec.Header().Get("Retry-After"); got != "60" {
				t.Fatalf("Retry-After = %q, want 60", got)
			}
		})
	}
}

// serverWith is one server instance with the task module, the fake from main_test.
func serverWith(t *testing.T, in infra) http.Handler {
	t.Helper()
	h, err := newHandler(testLogger(), []string{"task"}, []app.Module{&fakeModule{name: "task"}}, in)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestAPIRateLimitCountsAcrossInstancesThroughRedis(t *testing.T) {
	rdb := startRedis(t)
	in := infra{RateLimit: limitOn(2)}.withRedis(rdb)
	// Two server instances over one Redis share their counts (NFR-04).
	a := serverWith(t, in)
	b := serverWith(t, in)
	const remote = "192.0.2.50:4000"
	if rec := call(a, "GET", "/api/v1/task/ping", remote, nil); rec.Code != http.StatusOK {
		t.Fatalf("instance a, call 1: status = %d", rec.Code)
	}
	if rec := call(b, "GET", "/api/v1/task/ping", remote, nil); rec.Code != http.StatusOK {
		t.Fatalf("instance b, call 2: status = %d", rec.Code)
	}
	if rec := call(a, "GET", "/api/v1/task/ping", remote, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("instance a, call 3: status = %d, want 429 counted across instances", rec.Code)
	}
}

// tokenAuth is an app.Authenticator that accepts one token, as the auth module
// does for a valid access token.
type tokenAuth struct{}

func (tokenAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	if token == "good" {
		return app.Identity{UserID: "alice", DeviceID: "d1"}, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

func TestAPIRateLimitKeysSignedInUsersByTheAuthenticator(t *testing.T) {
	services := app.NewServices()
	app.Provide[app.Authenticator](services, tokenAuth{})
	h := serverWith(t, infra{
		Services:  services,
		RateLimit: config.RateLimit{Enabled: true, PerUser: 2, PerIP: 1},
	})
	const remote = "192.0.2.60:4000"
	signedIn := map[string]string{"Authorization": "Bearer good"}
	for i := 1; i <= 2; i++ {
		if rec := call(h, "GET", "/api/v1/task/ping", remote, signedIn); rec.Code != http.StatusOK {
			t.Fatalf("signed-in call %d: status = %d, want 200", i, rec.Code)
		}
	}
	if rec := call(h, "GET", "/api/v1/task/ping", remote, signedIn); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("signed-in call 3: status = %d, want 429 for the user", rec.Code)
	}
	// A signed-out call from the same address has its own, smaller count.
	if rec := call(h, "GET", "/api/v1/task/ping", remote, nil); rec.Code != http.StatusOK {
		t.Fatalf("signed-out call: status = %d, want 200", rec.Code)
	}
	if rec := call(h, "GET", "/api/v1/task/ping", remote, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second signed-out call: status = %d, want 429 for the IP", rec.Code)
	}
}

func TestAPIRateLimitLeavesProbesAndMetricsAlone(t *testing.T) {
	h := serverWith(t, infra{RateLimit: limitOn(1)})
	const remote = "192.0.2.70:4000"
	for i := 1; i <= 5; i++ {
		for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
			if rec := call(h, "GET", path, remote, nil); rec.Code != http.StatusOK {
				t.Fatalf("%s call %d: status = %d, want 200", path, i, rec.Code)
			}
		}
	}
}

func TestAPIRateLimitIsOffWhenDisabled(t *testing.T) {
	h := serverWith(t, infra{RateLimit: config.RateLimit{Enabled: false, PerUser: 1, PerIP: 1}})
	for i := 1; i <= 5; i++ {
		if rec := call(h, "GET", "/api/v1/task/ping", "192.0.2.80:4000", nil); rec.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 with the limit off", i, rec.Code)
		}
	}
}

func TestAPIRateLimitTrustsForwardedForOnlyFromTheConfiguredProxy(t *testing.T) {
	proxy := netip.MustParsePrefix("10.0.0.5/32")
	h := serverWith(t, infra{RateLimit: config.RateLimit{Enabled: true, PerUser: 100, PerIP: 1, TrustedProxies: []netip.Prefix{proxy}}})
	behindProxy := map[string]string{"X-Forwarded-For": "203.0.113.9"}
	call(h, "GET", "/api/v1/task/ping", "10.0.0.5:4000", behindProxy)
	if rec := call(h, "GET", "/api/v1/task/ping", "10.0.0.5:4001", behindProxy); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same client behind the trusted proxy: status = %d, want 429", rec.Code)
	}
	// The same header from an address that is not the proxy is ignored, so the
	// key is that address and not the forged client.
	call(h, "GET", "/api/v1/task/ping", "192.0.2.90:4000", map[string]string{"X-Forwarded-For": "203.0.113.10"})
	if rec := call(h, "GET", "/api/v1/task/ping", "192.0.2.90:4001", map[string]string{"X-Forwarded-For": "203.0.113.11"}); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged header from an untrusted peer: status = %d, want 429", rec.Code)
	}
}
