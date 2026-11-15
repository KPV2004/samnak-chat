package realtime_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/realtime"
)

func TestSlowClientIsDisconnectedAndDoesNotBlockOthers(t *testing.T) {
	hub, srv := newHub(t, testAuth(), realtime.WithSendQueue(8))
	slow := login(t, srv, "tok-alice") // never reads after this
	fast := login(t, srv, "tok-bob")
	_ = slow

	got := make(chan int, 1)
	go func() {
		n := 0
		for {
			env, err := recvErr(fast, 10*time.Second)
			if err != nil || env.Type == "end" {
				got <- n
				return
			}
			n++
		}
	}()

	big, err := app.NewEnvelope("blob", map[string]string{"pad": strings.Repeat("x", 128*1024)})
	if err != nil {
		t.Fatal(err)
	}
	sent := 0
	for ; sent < 5000 && hub.Connections() == 2; sent++ {
		start := time.Now()
		if err := hub.SendToUsers(context.Background(), []string{alice.UserID, bob.UserID}, big); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("SendToUsers blocked for %v on a slow client", d)
		}
		time.Sleep(time.Millisecond) // let the fast client keep up
	}
	if hub.Connections() != 1 {
		t.Fatalf("slow client was not disconnected after %d frames", sent)
	}

	end, _ := app.NewEnvelope("end", nil)
	if err := hub.SendToUsers(context.Background(), []string{bob.UserID}, end); err != nil {
		t.Fatal(err)
	}
	if n := <-got; n != sent {
		t.Fatalf("fast client received %d frames, want %d", n, sent)
	}
}

func TestCloseSaysGoodbyeAndWaitsForConnections(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	a := login(t, srv, "tok-alice")
	b := login(t, srv, "tok-bob")

	// Clients answer the close frame while they are reading, as real ones do.
	type result struct {
		code websocket.StatusCode
		err  error
	}
	codes := make(map[string]chan result)
	for name, c := range map[string]*websocket.Conn{"alice": a, "bob": b} {
		ch := make(chan result, 1)
		codes[name] = ch
		go func() {
			code, err := closeCode(c, 5*time.Second)
			ch <- result{code, err}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := hub.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for name, ch := range codes {
		got := <-ch
		if got.err != nil || got.code != websocket.StatusGoingAway {
			t.Fatalf("%s: close code = %d (%v), want 1001", name, got.code, got.err)
		}
	}
	if n := hub.Connections(); n != 0 {
		t.Fatalf("connections after Close = %d, want 0", n)
	}
	if err := hub.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestCloseHonoursContextWhenAClientDoesNotAnswer(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	_ = login(t, srv, "tok-alice") // a client that never reads the close frame

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// The client library only answers a close frame while reading, so Close
	// either finishes in time or reports the expired context; it never hangs.
	done := make(chan error, 1)
	go func() { done <- hub.Close(ctx) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close ignored its context")
	}
}

func TestNoNewConnectionsAfterClose(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	if err := hub.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err == nil {
		// The upgrade may succeed; the hub must then close it straight away.
		defer func() { _ = c.CloseNow() }()
		if got := expectClosed(t, c, 5*time.Second); got != websocket.StatusGoingAway {
			t.Fatalf("close code = %d, want 1001", got)
		}
		return
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
