package acceptance

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
)

const (
	// codeUnauthorized is the application close code for "authenticate again".
	codeUnauthorized = websocket.StatusCode(4401)
	password         = "correct-horse-battery-staple"
)

var seq int64

func unique(key string) string {
	return fmt.Sprintf("%s-%d-%d", key, time.Now().UnixNano(), atomic.AddInt64(&seq, 1))
}

type user struct {
	id, email, access, refresh, deviceID string
}

// signUp registers and logs in a new account.
func (c client) signUp(name string) user {
	c.t.Helper()
	email := unique(name) + "@example.com"
	reg := c.want(c.do("POST", "/auth/register", call{body: map[string]any{
		"email": email, "password": password, "display_name": name,
	}}), 201)
	login := c.want(c.do("POST", "/auth/login", call{body: map[string]any{
		"email": email, "password": password, "device_name": name + "'s laptop", "platform": "web",
	}}), 200)
	return user{
		id: reg.str("id"), email: email, access: login.str("access_token"),
		refresh: login.str("refresh_token"), deviceID: login.str("device_id"),
	}
}

// login opens another device for an existing account.
func (c client) login(u user, device string) user {
	c.t.Helper()
	r := c.want(c.do("POST", "/auth/login", call{body: map[string]any{
		"email": u.email, "password": password, "device_name": device, "platform": "web",
	}}), 200)
	return user{
		id: u.id, email: u.email, access: r.str("access_token"),
		refresh: r.str("refresh_token"), deviceID: r.str("device_id"),
	}
}

func (c client) createWorkspace(u user) string {
	c.t.Helper()
	r := c.want(c.do("POST", "/workspace", call{token: u.access, body: map[string]any{
		"name": "Acme", "slug": unique("acme"),
	}}), 201)
	return r.str("id")
}

// addMember invites a user to the workspace and accepts the invite for them.
func (c client) addMember(owner user, workspaceID string, member user) {
	c.t.Helper()
	invite := c.want(c.do("POST", "/workspace/invites", call{
		token: owner.access, workspace: workspaceID,
		body: map[string]any{"email": member.email, "role": "member"},
	}), 201)
	c.want(c.do("POST", "/workspace/invites/accept", call{
		token: member.access, body: map[string]any{"token": invite.str("token")},
	}), 200)
}

func (c client) openDM(u user, workspaceID, otherUserID string) string {
	c.t.Helper()
	r := c.do("POST", "/chat/dms", call{
		token: u.access, workspace: workspaceID, body: map[string]any{"user_ids": []string{otherUserID}},
	})
	if r.Status != 201 && r.Status != 200 {
		c.t.Fatalf("open DM: status = %d: %v", r.Status, r.Body)
	}
	return r.str("id")
}

func (c client) send(u user, workspaceID, channelID, clientMsgID, body string) reply {
	c.t.Helper()
	return c.do("POST", "/chat/channels/"+channelID+"/messages", call{
		token: u.access, workspace: workspaceID,
		body: map[string]any{"client_msg_id": clientMsgID, "body": body},
	})
}

// messagesAfter is the catch-up query a client runs after reconnecting.
func (c client) messagesAfter(u user, workspaceID, channelID string, afterSeq int64) []map[string]any {
	c.t.Helper()
	r := c.want(c.do("GET", fmt.Sprintf("/chat/channels/%s/messages?after_seq=%d", channelID, afterSeq),
		call{token: u.access, workspace: workspaceID}), 200)
	return r.items()
}

type messageView struct {
	ID, ClientMsgID, Body, SenderID string
	Seq                             int64
}

func decodeItem(t *testing.T, m map[string]any) messageView {
	t.Helper()
	seq, _ := m["seq"].(float64)
	str := func(key string) string { s, _ := m[key].(string); return s }
	return messageView{ID: str("id"), ClientMsgID: str("client_msg_id"), Body: str("body"), SenderID: str("sender_id"), Seq: int64(seq)}
}

// messageEvent is the data of a message.created frame.
type messageEvent struct {
	WorkspaceID string      `json:"workspace_id"`
	ChannelID   string      `json:"channel_id"`
	Message     messageWire `json:"message"`
}

type messageWire struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channel_id"`
	Seq         int64  `json:"seq"`
	SenderID    string `json:"sender_id"`
	ClientMsgID string `json:"client_msg_id"`
	Body        string `json:"body"`
}

func decodeEvent(t *testing.T, env app.Envelope) messageEvent {
	t.Helper()
	var e messageEvent
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatalf("message.created data %s: %v", env.Data, err)
	}
	return e
}

func replySeq(t *testing.T, r reply) int64 {
	t.Helper()
	seq, ok := r.Body["seq"].(float64)
	if !ok {
		t.Fatalf("response has no seq: %v", r.Body)
	}
	return int64(seq)
}

// expiredToken signs an access token that expired an hour ago, with the secret
// this test server uses.
func expiredToken(t *testing.T, userID, deviceID string) string {
	t.Helper()
	now := time.Now()
	tok := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": userID, "did": deviceID, "jti": "expired-test-token",
		"iat": now.Add(-2 * time.Hour).Unix(), "exp": now.Add(-time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// wantEvents asserts exactly the events seen since the last drain.
func (in *instance) wantEvents(t *testing.T, want ...auth.DeviceRevoked) {
	t.Helper()
	got := in.disconnectEvents()
	if len(got) != len(want) {
		t.Fatalf("auth.device_revoked events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("auth.device_revoked events = %v, want %v", got, want)
		}
	}
}

// TestSRSM2Acceptance proves the SRS 16.3 M2 outcomes against the real
// registry, hub, Postgres and Redis, plus AUTH-08 (revoked devices are cut off).
func TestSRSM2Acceptance(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	one := newInstance(t, pool, rdb)
	two := newInstance(t, pool, rdb)
	c := client{t: t, url: one.url}

	alice := c.signUp("alice")
	bob := c.signUp("bob")
	workspaceID := c.createWorkspace(alice)
	c.addMember(alice, workspaceID, bob)
	channelID := c.openDM(alice, workspaceID, bob.id)

	var lastSeq int64

	t.Run("two users see each other's message within one second", func(t *testing.T) {
		aliceConn := one.connect(t, alice.access)
		bobConn := one.connect(t, bob.access)

		clientMsgID := uuid.Must(uuid.NewV7()).String()
		sent := c.want(c.send(alice, workspaceID, channelID, clientMsgID, "hello bob"), 201)

		// SRS 16.3: measured from the HTTP response to the recipient's frame.
		posted := time.Now()
		env := recvType(t, bobConn, "message.created", time.Second)
		latency := time.Since(posted)
		if latency >= time.Second {
			t.Fatalf("recipient frame took %s after the POST returned, want under 1s", latency)
		}
		evt := decodeEvent(t, env)
		if evt.ChannelID != channelID || evt.Message.ID != sent.str("id") ||
			evt.Message.ClientMsgID != clientMsgID || evt.Message.Body != "hello bob" {
			t.Fatalf("frame = %+v, want the message that was posted (%v)", evt, sent.Body)
		}
		t.Logf("recipient got message.created in %s after the POST returned", latency)

		// The sender's own browser sees it as well.
		own := decodeEvent(t, recvType(t, aliceConn, "message.created", time.Second))
		if own.Message.ID != sent.str("id") {
			t.Fatalf("sender frame message = %s, want %s", own.Message.ID, sent.str("id"))
		}
		// Exactly one delivery per recipient.
		for _, f := range drainToPong(t, bobConn) {
			if f.Type == "message.created" {
				t.Fatalf("a duplicate message.created arrived: %s", f.Data)
			}
		}

		lastSeq = replySeq(t, sent)
	})

	t.Run("reconnecting with after_seq recovers every message in order", func(t *testing.T) {
		// Bob's socket closed with the previous subtest: the network dropped.
		first := c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(), "while you were away 1"), 201)
		second := c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(), "while you were away 2"), 201)

		// He reconnects on the other instance, as a restarted client would.
		bobConn := two.connect(t, bob.access)
		items := c.messagesAfter(bob, workspaceID, channelID, lastSeq)
		if len(items) != 2 {
			t.Fatalf("catch-up returned %d messages, want 2: %v", len(items), items)
		}
		got := []messageView{decodeItem(t, items[0]), decodeItem(t, items[1])}
		if got[0].ID != first.str("id") || got[1].ID != second.str("id") {
			t.Fatalf("catch-up = %+v, want the two messages sent while offline", got)
		}
		if got[0].Seq != replySeq(t, first) || got[1].Seq != replySeq(t, second) || got[0].Seq >= got[1].Seq {
			t.Fatalf("catch-up seqs = %d, %d, want ascending and dense", got[0].Seq, got[1].Seq)
		}
		// Each message exactly once, and the socket replays nothing.
		if frames := drainToPong(t, bobConn); len(frames) != 0 {
			t.Fatalf("reconnect replayed %d frames, want none", len(frames))
		}
		lastSeq = got[1].Seq
	})

	t.Run("resending client_msg_id creates one message and one event", func(t *testing.T) {
		bobConn := one.connect(t, bob.access)
		clientMsgID := uuid.Must(uuid.NewV7()).String()

		first := c.want(c.send(alice, workspaceID, channelID, clientMsgID, "sent once"), 201)
		evt := decodeEvent(t, recvType(t, bobConn, "message.created", time.Second))
		if evt.Message.ID != first.str("id") || evt.Message.ClientMsgID != clientMsgID {
			t.Fatalf("frame = %+v, want the stored message %s", evt.Message, first.str("id"))
		}
		if frames := drainToPong(t, bobConn); len(frames) != 0 {
			t.Fatalf("the first send delivered %d extra frames", len(frames))
		}

		again := c.want(c.send(alice, workspaceID, channelID, clientMsgID, "sent once"), 200)
		if again.str("id") != first.str("id") {
			t.Fatalf("retry created message %s, want the original %s", again.str("id"), first.str("id"))
		}
		if frames := drainToPong(t, bobConn); len(frames) != 0 {
			t.Fatalf("the retry published %d frames, want none", len(frames))
		}

		count := 0
		for _, it := range c.messagesAfter(bob, workspaceID, channelID, lastSeq) {
			if it["client_msg_id"] == clientMsgID {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("channel holds %d messages with client_msg_id %s, want 1", count, clientMsgID)
		}
		lastSeq = replySeq(t, first)
	})

	t.Run("an expired access token is rejected everywhere", func(t *testing.T) {
		token := expiredToken(t, bob.id, bob.deviceID)
		for _, tc := range []struct{ name, path string }{
			{"auth", "/auth/me"},
			{"chat", "/chat/channels"},
			{"workspace", "/workspace/members"},
		} {
			r := c.do("GET", tc.path, call{token: token, workspace: workspaceID})
			if r.Status != 401 || r.errCode() != "unauthorized" {
				t.Errorf("%s with an expired token: status = %d %v, want 401 unauthorized", tc.name, r.Status, r.Body)
			}
		}
		conn := one.dial(t)
		write(t, conn, "auth", map[string]any{"access_token": token})
		if code := expectClose(t, conn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("auth frame close code = %d, want 4401", code)
		}
	})

	t.Run("revoking a device closes its socket with 4401", func(t *testing.T) {
		one.disconnectEvents()
		phone := c.login(bob, "Phone")
		conn := one.connect(t, phone.access)

		c.want(c.do("DELETE", "/auth/devices/"+phone.deviceID, call{token: bob.access}), 204)
		if code := expectClose(t, conn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("close code = %d, want 4401", code)
		}
		one.wantEvents(t, auth.DeviceRevoked{UserID: bob.id, DeviceID: phone.deviceID})

		// Replaying the revoke changes nothing and publishes nothing.
		if r := c.do("DELETE", "/auth/devices/"+phone.deviceID, call{token: bob.access}); r.Status != 404 {
			t.Fatalf("second revoke: status = %d: %v", r.Status, r.Body)
		}
		one.wantEvents(t)

		// The revoked token fails over HTTP and at the auth frame.
		if r := c.do("GET", "/auth/me", call{token: phone.access}); r.Status != 401 {
			t.Fatalf("revoked token on /auth/me: status = %d", r.Status)
		}
		next := one.dial(t)
		write(t, next, "auth", map[string]any{"access_token": phone.access})
		if code := expectClose(t, next, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("next auth frame close code = %d, want 4401", code)
		}
	})

	t.Run("logging out every device closes sockets on another instance", func(t *testing.T) {
		one.disconnectEvents()
		peerConn := two.connect(t, bob.access)
		localConn := one.connect(t, bob.access)

		c.want(c.do("POST", "/auth/logout", call{token: bob.access, body: map[string]any{"all_devices": true}}), 204)
		if code := expectClose(t, peerConn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("peer instance close code = %d, want 4401", code)
		}
		if code := expectClose(t, localConn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("local instance close code = %d, want 4401", code)
		}
		// One event for the user, with no device: every device went.
		one.wantEvents(t, auth.DeviceRevoked{UserID: bob.id})

		// The token is dead, so a replay publishes nothing.
		if r := c.do("POST", "/auth/logout", call{token: bob.access, body: map[string]any{"all_devices": true}}); r.Status != 401 {
			t.Fatalf("second logout: status = %d: %v", r.Status, r.Body)
		}
		one.wantEvents(t)

		next := one.dial(t)
		write(t, next, "auth", map[string]any{"access_token": bob.access})
		if code := expectClose(t, next, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("next auth frame close code = %d, want 4401", code)
		}
	})

	t.Run("a token of a device revoked by refresh reuse is rejected", func(t *testing.T) {
		one.disconnectEvents()
		tablet := c.login(bob, "Tablet")
		tabletConn := one.connect(t, tablet.access)
		c.want(c.do("POST", "/auth/refresh", call{body: map[string]any{"refresh_token": tablet.refresh}}), 200)

		// Replaying the spent token revokes the whole device (AUTH-03). That is
		// a revocation like any other, so its socket closes at once (AUTH-08).
		if r := c.do("POST", "/auth/refresh", call{body: map[string]any{"refresh_token": tablet.refresh}}); r.Status != 401 {
			t.Fatalf("reused refresh token: status = %d: %v", r.Status, r.Body)
		}
		if code := expectClose(t, tabletConn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("close code = %d, want 4401", code)
		}
		one.wantEvents(t, auth.DeviceRevoked{UserID: bob.id, DeviceID: tablet.deviceID})
		for _, tc := range []struct{ name, path string }{
			{"auth", "/auth/me"},
			{"chat", "/chat/channels"},
			{"workspace", "/workspace/members"},
		} {
			r := c.do("GET", tc.path, call{token: tablet.access, workspace: workspaceID})
			if r.Status != 401 {
				t.Errorf("%s with the access token of a revoked device: status = %d, want 401", tc.name, r.Status)
			}
		}
		conn := one.dial(t)
		write(t, conn, "auth", map[string]any{"access_token": tablet.access})
		if code := expectClose(t, conn, 5*time.Second); code != codeUnauthorized {
			t.Fatalf("auth frame close code = %d, want 4401", code)
		}
	})
}
