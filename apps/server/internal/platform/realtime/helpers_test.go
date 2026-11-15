package realtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/realtime"
)

// fakeAuth maps access tokens to identities.
type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, token string) (app.Identity, error) {
	id, ok := f[token]
	if !ok {
		return app.Identity{}, fmt.Errorf("bad token: %w", app.ErrUnauthorized)
	}
	return id, nil
}

// brokenAuth fails with an error that is not ErrUnauthorized.
type brokenAuth struct{}

func (brokenAuth) Authenticate(context.Context, string) (app.Identity, error) {
	return app.Identity{}, io.ErrUnexpectedEOF
}

func testLogger(t testing.TB) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newHubWith starts a hub behind a real router and httptest server. Cleanup
// closes the hub first, then the server.
func newHubWith(t testing.TB, auth app.Authenticator, rdb *goredis.Client, opts ...realtime.Option) (*realtime.Hub, *httptest.Server) {
	t.Helper()
	services := app.NewServices()
	if auth != nil {
		app.Provide(services, auth)
	}
	logger := testLogger(t)
	hub := realtime.NewHub(logger, services, rdb, opts...)
	r := httpx.NewRouter(logger)
	hub.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := hub.Close(ctx); err != nil {
			t.Errorf("hub close: %v", err)
		}
	})
	return hub, srv
}

func newHub(t testing.TB, auth app.Authenticator, opts ...realtime.Option) (*realtime.Hub, *httptest.Server) {
	t.Helper()
	return newHubWith(t, auth, nil, opts...)
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws"
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.SetReadLimit(1 << 20)
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, typ string, data any) {
	t.Helper()
	env, err := app.NewEnvelope(typ, data)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		env.Data = nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write %s: %v", typ, err)
	}
}

// recv reads one envelope, failing the test if none arrives in time.
func recv(t *testing.T, c *websocket.Conn) app.Envelope {
	t.Helper()
	env, err := recvErr(c, 5*time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return env
}

func recvErr(c *websocket.Conn, timeout time.Duration) (app.Envelope, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		return app.Envelope{}, err
	}
	var env app.Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return app.Envelope{}, err
	}
	return env, nil
}

// closeCode waits for the server to close the connection and returns the
// close code. It is safe to call from any goroutine.
func closeCode(c *websocket.Conn, timeout time.Duration) (websocket.StatusCode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		_, _, err := c.Read(ctx)
		if err == nil {
			continue // drain frames that were queued before the close
		}
		code := websocket.CloseStatus(err)
		if code == -1 {
			return 0, fmt.Errorf("connection ended without a close frame: %w", err)
		}
		return code, nil
	}
}

func expectClosed(t *testing.T, c *websocket.Conn, timeout time.Duration) websocket.StatusCode {
	t.Helper()
	code, err := closeCode(c, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// login dials, authenticates with token and waits for ready.
func login(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	c := dial(t, srv)
	send(t, c, "auth", map[string]string{"access_token": token})
	env := recv(t, c)
	if env.Type != "ready" {
		t.Fatalf("first frame = %q, want ready", env.Type)
	}
	return c
}

func ping(t *testing.T, c *websocket.Conn) {
	t.Helper()
	send(t, c, "ping", nil)
	if env := recv(t, c); env.Type != "pong" {
		t.Fatalf("reply to ping = %q, want pong", env.Type)
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// serve mounts hub on a fresh router and httptest server.
func serve(t *testing.T, hub *realtime.Hub) *httptest.Server {
	t.Helper()
	r := httpx.NewRouter(testLogger(t))
	hub.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}
