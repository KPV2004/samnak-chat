package realtime_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const loadConns = 1000 // NFR-03

// loadClients connects n authenticated clients, one user each, with bounded
// parallelism. Cleanup closes them.
func loadClients(tb testing.TB, srv *httptest.Server, n int) ([]*websocket.Conn, []string) {
	tb.Helper()
	clients := make([]*websocket.Conn, n)
	users := make([]string, n)
	errs := make(chan error, n)
	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			c, _, err := websocket.Dial(ctx, wsURL(srv), nil)
			if err != nil {
				errs <- fmt.Errorf("dial %d: %w", i, err)
				return
			}
			if err := c.Write(ctx, websocket.MessageText, fmt.Appendf(nil, `{"type":"auth","data":{"access_token":"tok-%d"}}`, i)); err != nil {
				errs <- fmt.Errorf("auth %d: %w", i, err)
				return
			}
			if _, _, err := c.Read(ctx); err != nil { // ready
				errs <- fmt.Errorf("ready %d: %w", i, err)
				return
			}
			clients[i], users[i] = c, fmt.Sprintf("load-user-%d", i)
		})
	}
	wg.Wait()
	close(errs)
	tb.Cleanup(func() {
		for _, c := range clients {
			if c != nil {
				_ = c.CloseNow()
			}
		}
	})
	for err := range errs {
		tb.Fatal(err)
	}
	return clients, users
}

func loadAuth(n int) fakeAuth {
	auth := make(fakeAuth, n)
	for i := range n {
		auth[fmt.Sprintf("tok-%d", i)] = app.Identity{UserID: fmt.Sprintf("load-user-%d", i), DeviceID: fmt.Sprintf("load-dev-%d", i)}
	}
	return auth
}

// TestThousandConcurrentConnections holds 1,000 connections on one hub
// (NFR-03), broadcasts to all of them and checks the delivery latency against
// NFR-02 (p95 under 300 ms).
func TestThousandConcurrentConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in -short mode")
	}
	rdb := startRedis(t)
	hub, srv := newHubWith(t, loadAuth(loadConns), rdb)
	clients, users := loadClients(t, srv, loadConns)
	if got := hub.Connections(); got != loadConns {
		t.Fatalf("connections = %d, want %d", got, loadConns)
	}

	// Everyone is online, according to Redis presence.
	online, err := hub.Online(context.Background(), users)
	if err != nil || len(online) != loadConns {
		t.Fatalf("online = %d (err %v), want %d", len(online), err, loadConns)
	}

	latencies := make([]time.Duration, loadConns)
	errs := make(chan error, loadConns)
	var wg sync.WaitGroup
	var start time.Time
	ready := make(chan struct{})
	for i, c := range clients {
		wg.Go(func() {
			<-ready
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if _, _, err := c.Read(ctx); err != nil {
				errs <- fmt.Errorf("client %d: %w", i, err)
				return
			}
			latencies[i] = time.Since(start)
		})
	}

	env, _ := app.NewEnvelope("message.created", map[string]string{"workspace_id": "w1"})
	start = time.Now()
	close(ready)
	if err := hub.SendToUsers(context.Background(), users, env); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[loadConns*95/100]
	t.Logf("broadcast to %d connections: p50 %v, p95 %v, max %v", loadConns, latencies[loadConns/2], p95, latencies[loadConns-1])
	if p95 > 300*time.Millisecond {
		t.Fatalf("p95 delivery latency %v exceeds 300 ms (NFR-02)", p95)
	}
}

// BenchmarkSendToUsers measures one fan-out to 1,000 connected users.
func BenchmarkSendToUsers(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark in -short mode")
	}
	hub, srv := newHub(b, loadAuth(loadConns))
	clients, users := loadClients(b, srv, loadConns)
	for _, c := range clients {
		go func() { // drain so queues never fill
			for {
				if _, _, err := c.Read(context.Background()); err != nil {
					return
				}
			}
		}()
	}
	env, _ := app.NewEnvelope("message.created", map[string]string{"workspace_id": "w1"})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := hub.SendToUsers(context.Background(), users, env); err != nil {
			b.Fatal(err)
		}
	}
}
