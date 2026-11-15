package realtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/realtime"
)

const codeUnauthorized = websocket.StatusCode(4401)

var (
	alice  = app.Identity{UserID: "user-alice", DeviceID: "dev-alice-1"}
	alice2 = app.Identity{UserID: "user-alice", DeviceID: "dev-alice-2"}
	bob    = app.Identity{UserID: "user-bob", DeviceID: "dev-bob-1"}
)

func testAuth() fakeAuth {
	return fakeAuth{"tok-alice": alice, "tok-alice2": alice2, "tok-bob": bob}
}

func TestHandshakeReady(t *testing.T) {
	_, srv := newHub(t, testAuth())
	c := dial(t, srv)
	send(t, c, "auth", map[string]string{"access_token": "tok-alice"})

	env := recv(t, c)
	if env.Type != "ready" {
		t.Fatalf("type = %q, want ready", env.Type)
	}
	var data struct {
		UserID   string `json:"user_id"`
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.UserID != alice.UserID || data.DeviceID != alice.DeviceID {
		t.Fatalf("ready data = %+v, want %+v", data, alice)
	}
}

func TestHandshakeFailuresCloseWith4401(t *testing.T) {
	tests := []struct {
		name string
		send func(t *testing.T, c *websocket.Conn)
	}{
		{"bad token", func(t *testing.T, c *websocket.Conn) {
			send(t, c, "auth", map[string]string{"access_token": "nope"})
		}},
		{"empty token", func(t *testing.T, c *websocket.Conn) {
			send(t, c, "auth", map[string]string{"access_token": ""})
		}},
		{"first message is not auth", func(t *testing.T, c *websocket.Conn) {
			send(t, c, "ping", nil)
		}},
		{"not json", func(t *testing.T, c *websocket.Conn) {
			if err := c.Write(context.Background(), websocket.MessageText, []byte("hello")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, srv := newHub(t, testAuth())
			c := dial(t, srv)
			tt.send(t, c)
			if got := expectClosed(t, c, 5*time.Second); got != codeUnauthorized {
				t.Fatalf("close code = %d, want 4401", got)
			}
		})
	}
}

func TestHandshakeTimeoutCloses4401(t *testing.T) {
	_, srv := newHub(t, testAuth(), realtime.WithAuthTimeout(100*time.Millisecond))
	c := dial(t, srv)
	if got := expectClosed(t, c, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("close code = %d, want 4401", got)
	}
}

func TestHandshakeWithoutAuthenticatorCloses4401(t *testing.T) {
	_, srv := newHub(t, nil)
	c := dial(t, srv)
	send(t, c, "auth", map[string]string{"access_token": "tok-alice"})
	if got := expectClosed(t, c, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("close code = %d, want 4401", got)
	}
}

func TestHandshakeAuthenticatorFailureCloses1011(t *testing.T) {
	_, srv := newHub(t, brokenAuth{})
	c := dial(t, srv)
	send(t, c, "auth", map[string]string{"access_token": "tok-alice"})
	if got := expectClosed(t, c, 5*time.Second); got != websocket.StatusInternalError {
		t.Fatalf("close code = %d, want 1011", got)
	}
}

func TestAuthenticatorIsLookedUpAtHandshakeTime(t *testing.T) {
	services := app.NewServices()
	hub := realtime.NewHub(testLogger(t), services, nil)
	t.Cleanup(func() { _ = hub.Close(context.Background()) })
	// The authenticator is provided after the hub is built.
	app.Provide[app.Authenticator](services, testAuth())

	srv := serve(t, hub)
	c := dial(t, srv)
	send(t, c, "auth", map[string]string{"access_token": "tok-bob"})
	if env := recv(t, c); env.Type != "ready" {
		t.Fatalf("type = %q, want ready", env.Type)
	}
}

func TestPingPong(t *testing.T) {
	_, srv := newHub(t, testAuth())
	c := login(t, srv, "tok-alice")
	ping(t, c)
	ping(t, c)
}

func TestIdleConnectionIsClosed(t *testing.T) {
	_, srv := newHub(t, testAuth(), realtime.WithIdleTimeout(200*time.Millisecond))
	c := login(t, srv, "tok-alice")
	if got := expectClosed(t, c, 5*time.Second); got != websocket.StatusPolicyViolation {
		t.Fatalf("close code = %d, want 1008", got)
	}
}

func TestPingKeepsConnectionAlive(t *testing.T) {
	_, srv := newHub(t, testAuth(), realtime.WithIdleTimeout(300*time.Millisecond))
	c := login(t, srv, "tok-alice")
	for range 8 { // 8 x 100 ms is well past the idle timeout
		time.Sleep(100 * time.Millisecond)
		ping(t, c)
	}
}

func TestInboundFrameOver64KiBClosesWith1009(t *testing.T) {
	_, srv := newHub(t, testAuth())
	c := login(t, srv, "tok-alice")
	big := `{"type":"typing","data":"` + strings.Repeat("x", 64*1024) + `"}`
	if err := c.Write(context.Background(), websocket.MessageText, []byte(big)); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, c, 5*time.Second); got != websocket.StatusMessageTooBig {
		t.Fatalf("close code = %d, want 1009", got)
	}
}

func TestFrameJustUnder64KiBIsAccepted(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	got := make(chan int, 1)
	hub.Handle("blob", func(_ context.Context, _ app.Identity, data json.RawMessage) error {
		got <- len(data)
		return nil
	})
	c := login(t, srv, "tok-alice")
	const prefix, suffix = `{"type":"blob","data":"`, `"}`
	body := strings.Repeat("x", 64*1024-len(prefix)-len(suffix))
	if err := c.Write(context.Background(), websocket.MessageText, []byte(prefix+body+suffix)); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-got:
		if n != len(body)+2 {
			t.Fatalf("handler data length = %d, want %d", n, len(body)+2)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler was not called")
	}
}

func TestBinaryFrameClosesWith1003(t *testing.T) {
	_, srv := newHub(t, testAuth())
	c := login(t, srv, "tok-alice")
	if err := c.Write(context.Background(), websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, c, 5*time.Second); got != websocket.StatusUnsupportedData {
		t.Fatalf("close code = %d, want 1003", got)
	}
}

func TestHandleDispatchesRegisteredTypes(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	type call struct {
		from app.Identity
		data string
	}
	calls := make(chan call, 1)
	hub.Handle("typing", func(_ context.Context, from app.Identity, data json.RawMessage) error {
		calls <- call{from, string(data)}
		return nil
	})
	c := login(t, srv, "tok-alice")
	send(t, c, "typing", map[string]string{"channel_id": "c1"})

	select {
	case got := <-calls:
		if got.from != alice || got.data != `{"channel_id":"c1"}` {
			t.Fatalf("handler got %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler was not called")
	}
}

func TestUnknownMalformedAndFailingMessagesAreRejectedWithoutClosing(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	hub.Handle("fails", func(context.Context, app.Identity, json.RawMessage) error {
		return errors.New("db is down")
	})
	hub.Handle("invalid", func(context.Context, app.Identity, json.RawMessage) error {
		return app.ErrInvalid
	})
	hub.Handle("panics", func(context.Context, app.Identity, json.RawMessage) error {
		panic("boom")
	})
	c := login(t, srv, "tok-alice")

	wantError := func(what, code string) {
		t.Helper()
		env := recv(t, c)
		if env.Type != "error" {
			t.Fatalf("%s: type = %q, want error", what, env.Type)
		}
		var data struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(env.Data, &data); err != nil || data.Code != code {
			t.Fatalf("%s: data = %s (%v), want code %q", what, env.Data, err, code)
		}
		ping(t, c) // the connection is still open
	}

	send(t, c, "no.such.type", nil)
	wantError("unknown type", "unknown_type")

	if err := c.Write(context.Background(), websocket.MessageText, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	wantError("malformed json", "invalid")

	send(t, c, "auth", map[string]string{"access_token": "tok-alice"})
	wantError("auth after ready", "invalid")

	send(t, c, "invalid", nil)
	wantError("handler ErrInvalid", "invalid")

	send(t, c, "fails", nil)
	wantError("handler error", "internal")

	send(t, c, "panics", nil)
	wantError("handler panic", "internal")
}

func TestHandleRejectsReservedAndDuplicateTypes(t *testing.T) {
	hub, _ := newHub(t, testAuth())
	noop := func(context.Context, app.Identity, json.RawMessage) error { return nil }
	for _, typ := range []string{"auth", "ping"} {
		mustPanic(t, typ, func() { hub.Handle(typ, noop) })
	}
	hub.Handle("typing", noop)
	mustPanic(t, "duplicate", func() { hub.Handle("typing", noop) })
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected a panic", what)
		}
	}()
	fn()
}

func TestSendToUsersReachesEveryConnectionOfTheTargetsOnly(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	a1 := login(t, srv, "tok-alice")
	a2 := login(t, srv, "tok-alice2")
	b := login(t, srv, "tok-bob")

	env, err := app.NewEnvelope("message.created", map[string]string{"workspace_id": "w1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.SendToUsers(context.Background(), []string{alice.UserID, "user-nobody"}, env); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*websocket.Conn{"alice device 1": a1, "alice device 2": a2} {
		got := recv(t, c)
		if got.Type != "message.created" || string(got.Data) != `{"workspace_id":"w1"}` {
			t.Fatalf("%s got %q %s", name, got.Type, got.Data)
		}
	}
	// bob was not a target: his next frame is the pong for his own ping.
	ping(t, b)
}

func TestSendToUsersValidatesInput(t *testing.T) {
	hub, _ := newHub(t, testAuth())
	if err := hub.SendToUsers(context.Background(), []string{"u"}, app.Envelope{}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("empty type: err = %v, want ErrInvalid", err)
	}
	env, _ := app.NewEnvelope("x", nil)
	if err := hub.SendToUsers(context.Background(), nil, env); err != nil {
		t.Fatalf("no recipients: err = %v, want nil", err)
	}
}

func TestDisconnectDeviceClosesOnlyThatDevice(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	a1 := login(t, srv, "tok-alice")
	a2 := login(t, srv, "tok-alice2")

	if err := hub.DisconnectDevice(context.Background(), alice.DeviceID); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, a1, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("close code = %d, want 4401", got)
	}
	ping(t, a2) // the other device is untouched
}

func TestConnectionsAreCountedAndReleased(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	if n := hub.Connections(); n != 0 {
		t.Fatalf("connections = %d, want 0", n)
	}
	a := login(t, srv, "tok-alice")
	_ = login(t, srv, "tok-bob")
	eventually(t, "2 connections", func() bool { return hub.Connections() == 2 })

	if err := a.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, "1 connection", func() bool { return hub.Connections() == 1 })

	// A released user no longer receives events and does not leak memory.
	env, _ := app.NewEnvelope("x", nil)
	if err := hub.SendToUsers(context.Background(), []string{alice.UserID}, env); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentSendsAreSafe(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	c := login(t, srv, "tok-alice")
	env, _ := app.NewEnvelope("tick", nil)

	const senders, perSender = 8, 20
	var wg sync.WaitGroup
	for range senders {
		wg.Go(func() {
			for range perSender {
				_ = hub.SendToUsers(context.Background(), []string{alice.UserID}, env)
			}
		})
	}
	wg.Wait()
	for i := range senders * perSender {
		if got := recv(t, c); got.Type != "tick" {
			t.Fatalf("frame %d type = %q, want tick", i, got.Type)
		}
	}
}
