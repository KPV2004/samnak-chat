package realtime_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
)

// redisURL is the shared redis:7-alpine container for this package, or "" when
// Docker is unavailable or -short is set.
var redisURL string

func TestMain(m *testing.M) {
	flag.Parse()
	var stop func()
	if !testing.Short() && dockerAvailable() {
		ctx := context.Background()
		c, err := tcredis.Run(ctx, "redis:7-alpine")
		if err == nil {
			redisURL, err = c.ConnectionString(ctx)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "redis container unavailable, Redis tests will skip: %v\n", err)
			redisURL = ""
		}
		stop = func() { _ = testcontainers.TerminateContainer(c) }
	}
	code := m.Run()
	if stop != nil {
		stop()
	}
	os.Exit(code)
}

func dockerAvailable() bool {
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return true
	}
	return os.Getenv("DOCKER_HOST") != ""
}

// startRedis returns a client on the shared container.
func startRedis(t *testing.T) *goredis.Client {
	t.Helper()
	if redisURL == "" {
		t.Skip("skipping integration test: no Redis (-short or Docker unavailable)")
	}
	client, err := redis.Open(context.Background(), redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// uniqueAuth gives each test its own users, so tests sharing one Redis never
// see each other's presence keys or events.
func uniqueAuth(t *testing.T) (auth fakeAuth, a1, a2, b app.Identity) {
	t.Helper()
	n := t.Name()
	a1 = app.Identity{UserID: "alice-" + n, DeviceID: "alice-d1-" + n}
	a2 = app.Identity{UserID: "alice-" + n, DeviceID: "alice-d2-" + n}
	b = app.Identity{UserID: "bob-" + n, DeviceID: "bob-d1-" + n}
	return fakeAuth{"a1": a1, "a2": a2, "b": b}, a1, a2, b
}

func TestTwoHubsDeliverToEachOthersClients(t *testing.T) {
	rdb := startRedis(t)
	auth, a1, _, b := uniqueAuth(t)
	hub1, srv1 := newHubWith(t, auth, rdb)
	hub2, srv2 := newHubWith(t, auth, rdb)
	ca := login(t, srv1, "a1") // alice on instance 1
	cb := login(t, srv2, "b")  // bob on instance 2

	env, _ := app.NewEnvelope("message.created", map[string]string{"workspace_id": "w1", "n": "1"})
	if err := hub1.SendToUsers(context.Background(), []string{b.UserID}, env); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, cb); got.Type != "message.created" || string(got.Data) != `{"n":"1","workspace_id":"w1"}` {
		t.Fatalf("bob got %q %s", got.Type, got.Data)
	}

	env2, _ := app.NewEnvelope("channel.updated", map[string]string{"workspace_id": "w1"})
	if err := hub2.SendToUsers(context.Background(), []string{a1.UserID}, env2); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, ca); got.Type != "channel.updated" {
		t.Fatalf("alice got %q", got.Type)
	}
}

func TestSendDeliversExactlyOncePerConnectionAcrossHubs(t *testing.T) {
	rdb := startRedis(t)
	auth, a1, _, _ := uniqueAuth(t)
	hub1, srv1 := newHubWith(t, auth, rdb)
	_, srv2 := newHubWith(t, auth, rdb)
	c1 := login(t, srv1, "a1") // the same user on both instances
	c2 := login(t, srv2, "a2")

	env, _ := app.NewEnvelope("tick", nil)
	if err := hub1.SendToUsers(context.Background(), []string{a1.UserID}, env); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, c1); got.Type != "tick" {
		t.Fatalf("instance 1 got %q", got.Type)
	}
	if got := recv(t, c2); got.Type != "tick" {
		t.Fatalf("instance 2 got %q", got.Type)
	}
	// No duplicates: the next frame on each is the reply to our own ping.
	ping(t, c1)
	ping(t, c2)
}

func TestDisconnectDeviceReachesOtherHubs(t *testing.T) {
	rdb := startRedis(t)
	auth, a1, _, _ := uniqueAuth(t)
	hub1, _ := newHubWith(t, auth, rdb)
	_, srv2 := newHubWith(t, auth, rdb)
	ca := login(t, srv2, "a1")
	cb := login(t, srv2, "b")

	if err := hub1.DisconnectDevice(context.Background(), a1.DeviceID); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, ca, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("close code = %d, want 4401", got)
	}
	ping(t, cb) // another user on that instance is untouched
}

// TestDisconnectUserReachesOtherHubs covers AUTH-08 for a revoke of every
// device: the event carries only a user, and the hub on another instance must
// close all of that user's connections.
func TestDisconnectUserReachesOtherHubs(t *testing.T) {
	rdb := startRedis(t)
	auth, a1, _, _ := uniqueAuth(t)
	hub1, _ := newHubWith(t, auth, rdb)
	_, srv2 := newHubWith(t, auth, rdb)
	ca1 := login(t, srv2, "a1")
	ca2 := login(t, srv2, "a2")
	cb := login(t, srv2, "b")

	if err := hub1.DisconnectUser(context.Background(), a1.UserID); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, ca1, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("device 1 close code = %d, want 4401", got)
	}
	if got := expectClosed(t, ca2, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("device 2 close code = %d, want 4401", got)
	}
	ping(t, cb) // another user on that instance is untouched
}

func TestDisconnectUserClosesEveryDeviceOnTheSameHub(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	c1 := login(t, srv, "tok-alice")  // alice on device 1
	c2 := login(t, srv, "tok-alice2") // alice on device 2
	other := login(t, srv, "tok-bob")

	if err := hub.DisconnectUser(context.Background(), alice.UserID); err != nil {
		t.Fatal(err)
	}
	if got := expectClosed(t, c1, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("first connection close code = %d, want 4401", got)
	}
	if got := expectClosed(t, c2, 5*time.Second); got != codeUnauthorized {
		t.Fatalf("second connection close code = %d, want 4401", got)
	}
	ping(t, other)
}

func TestPresenceKeyAndOnline(t *testing.T) {
	rdb := startRedis(t)
	auth, a1, _, b := uniqueAuth(t)
	hub1, srv1 := newHubWith(t, auth, rdb)
	hub2, _ := newHubWith(t, auth, rdb)
	ctx := context.Background()

	online, err := hub1.Online(ctx, []string{a1.UserID, b.UserID})
	if err != nil || len(online) != 0 {
		t.Fatalf("before connecting: online = %v, err = %v", online, err)
	}

	c := login(t, srv1, "a1")
	key := "rt:online:" + a1.UserID
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > 60*time.Second {
		t.Fatalf("presence ttl = %v (err %v), want within (0, 60s]", ttl, err)
	}

	// Any instance sees it, in the order asked.
	online, err = hub2.Online(ctx, []string{b.UserID, a1.UserID, "nobody"})
	if err != nil || len(online) != 1 || online[0] != a1.UserID {
		t.Fatalf("online = %v, err = %v, want [%s]", online, err, a1.UserID)
	}

	// A ping refreshes the TTL.
	if err := rdb.Expire(ctx, key, 5*time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	ping(t, c)
	if ttl, _ = rdb.TTL(ctx, key).Result(); ttl < 30*time.Second {
		t.Fatalf("ttl after ping = %v, want refreshed to about 60s", ttl)
	}

	// Without a refresh the key expires, and the user is offline.
	if err := rdb.Expire(ctx, key, time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "presence to expire", func() bool {
		on, err := hub2.Online(ctx, []string{a1.UserID})
		return err == nil && len(on) == 0
	})
}

func TestOnlineWithoutRedisUsesLocalConnections(t *testing.T) {
	hub, srv := newHub(t, testAuth())
	_ = login(t, srv, "tok-alice")
	got, err := hub.Online(context.Background(), []string{bob.UserID, alice.UserID})
	if err != nil || len(got) != 1 || got[0] != alice.UserID {
		t.Fatalf("online = %v, err = %v", got, err)
	}
}
