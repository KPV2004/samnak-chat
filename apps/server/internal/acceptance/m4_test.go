package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
)

// m4Config is cmd/server's wiring for MODULES_ENABLED=notify,task,approval on
// the core modules: the real rules registry, the job queue and the outbox bus.
func m4Config() instanceConfig {
	return instanceConfig{
		modules:       []string{"auth", "workspace", "chat", "notify", "task", "approval"},
		notifications: app.NewNotificationRules(),
		jobs:          true,
		outbox:        true,
	}
}

// TestSRSM4Acceptance proves the SRS 16.3 M4 outcomes on the server cmd/server
// builds with task and approval enabled. A task made from a message gets a card
// that follows its status over WebSocket, and a leave request is decided from
// its card in the DM. Every change reaches the sockets through the outbox relay.
func TestSRSM4Acceptance(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	in := newInstanceWith(t, pool, rdb, m4Config())
	c := client{t: t, url: in.url}

	alice := c.signUp("alice") // requester, task creator and workspace owner
	bob := c.signUp("bob")     // task assignee and channel member
	boss := c.signUp("boss")   // manager of alice's group, so her approver
	carol := c.signUp("carol") // workspace member who is neither in the channel nor an approver
	workspaceID := c.createWorkspace(alice)
	for _, u := range []user{bob, boss, carol} {
		c.addMember(alice, workspaceID, u)
	}
	group := c.createGroup(alice, workspaceID, "Team")
	c.setGroupMembers(alice, workspaceID, group, []map[string]any{
		{"user_id": alice.id, "is_manager": false},
		{"user_id": boss.id, "is_manager": true},
	})
	channelID := c.createChannel(alice, workspaceID, unique("m4-room"), []string{bob.id})

	aliceConn := in.connect(t, alice.access)
	bobConn := in.connect(t, bob.access)
	bossConn := in.connect(t, boss.access)
	carolConn := in.connect(t, carol.access)

	var taskID, taskCardID string

	t.Run("a task created from a message gets a task.item card", func(t *testing.T) {
		msg := c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(),
			"Please prepare the Q3 slides"), 201)
		task := c.want(c.do("POST", "/task/items", call{token: alice.access, workspace: workspaceID, body: map[string]any{
			"channel_id": channelID, "title": "Prepare the Q3 slides", "assignee_id": bob.id,
			"source_message_id": msg.str("id"),
		}}), 201)
		taskID = task.str("id")
		if task.str("source_message_id") != msg.str("id") || task.str("status") != "todo" {
			t.Fatalf("task = %v, want status todo from message %s", task.Body, msg.str("id"))
		}

		// The handler of task.created posts the card after the commit (PLT-10).
		card := recvCard(t, bobConn, "task.item", 2*time.Second)
		if card.ChannelID != channelID || card.Message.Payload["task_id"] != taskID ||
			card.Message.Payload["title"] != "Prepare the Q3 slides" || card.Message.Payload["status"] != "todo" ||
			card.Message.Payload["assignee_name"] != "bob" {
			t.Fatalf("card = %+v, want the new task in #%s", card, channelID)
		}
		taskCardID = card.Message.ID
		if sent := recvCard(t, aliceConn, "task.item", 2*time.Second); sent.Message.ID != taskCardID {
			t.Fatalf("the creator's card = %s, want %s", sent.Message.ID, taskCardID)
		}
	})

	t.Run("a status change from the task API sends card.updated to channel members", func(t *testing.T) {
		c.want(c.do("PATCH", "/task/items/"+taskID, call{token: alice.access, workspace: workspaceID,
			body: map[string]any{"status": "doing"}}), 200)
		// SRS 16.3: measured from the HTTP response to the member's frame.
		changed := time.Now()
		upd := recvCardUpdate(t, bobConn, taskCardID, "doing", time.Second)
		latency := time.Since(changed)
		if latency >= time.Second {
			t.Fatalf("card.updated took %s after the PATCH returned, want under 1s", latency)
		}
		t.Logf("card.updated arrived in %s after the PATCH returned", latency)
		if upd.ChannelID != channelID || upd.Payload["title"] != "Prepare the Q3 slides" {
			t.Fatalf("card.updated = %+v", upd)
		}
		recvCardUpdate(t, aliceConn, taskCardID, "doing", time.Second)

		// carol is in the workspace but not in the channel, so she hears nothing.
		for _, f := range drainToPong(t, carolConn) {
			if f.Type == "card.updated" || f.Type == "message.created" {
				t.Fatalf("a non-member received %s: %s", f.Type, f.Data)
			}
		}
	})

	t.Run("a leave request gets a card in the DM and the manager's approval updates it within a second", func(t *testing.T) {
		req := c.want(c.do("POST", "/approval/requests", call{token: alice.access, workspace: workspaceID, body: map[string]any{
			"type": "leave",
			"payload": map[string]any{
				"leave_type": "annual", "start_date": "2026-12-01", "end_date": "2026-12-03", "reason": "family trip",
			},
		}}), 201)
		requestID := req.str("id")
		// No approver named: the manager of alice's group is chosen (APR-02).
		if req.str("approver_id") != boss.id || req.str("status") != "pending" {
			t.Fatalf("request = %v, want pending with approver boss", req.Body)
		}

		// The card goes to the requester-approver DM and both people see it (APR-03).
		dm := c.openDM(alice, workspaceID, boss.id)
		leaveCard := recvCard(t, bossConn, "approval.request", 2*time.Second)
		if leaveCard.ChannelID != dm || leaveCard.Message.Payload["status"] != "pending" ||
			leaveCard.Message.Payload["requester_name"] != "alice" || leaveCard.Message.Payload["approver_name"] != "boss" {
			t.Fatalf("card = %+v, want a pending request in DM %s", leaveCard, dm)
		}
		if seen := recvCard(t, aliceConn, "approval.request", 2*time.Second); seen.Message.ID != leaveCard.Message.ID {
			t.Fatalf("the requester's card = %s, want %s", seen.Message.ID, leaveCard.Message.ID)
		}

		// Only the approver decides (APR-04): a member who is not the approver,
		// and the requester, both get 403, and the request stays pending.
		c.want(c.do("POST", "/approval/requests/"+requestID+"/approve", call{token: carol.access, workspace: workspaceID}), 403)
		c.want(c.do("POST", "/approval/requests/"+requestID+"/approve", call{token: alice.access, workspace: workspaceID}), 403)
		if got := c.want(c.do("GET", "/approval/requests/"+requestID, call{token: alice.access, workspace: workspaceID}), 200); got.str("status") != "pending" {
			t.Fatalf("status after the refused approvals = %s, want pending", got.str("status"))
		}

		c.want(c.do("POST", "/approval/requests/"+requestID+"/approve", call{token: boss.access, workspace: workspaceID}), 200)
		// SRS 16.3: the requester's card changes within a second of the approval.
		approved := time.Now()
		upd := recvCardUpdate(t, aliceConn, leaveCard.Message.ID, "approved", time.Second)
		latency := time.Since(approved)
		if latency >= time.Second {
			t.Fatalf("the requester's card updated %s after the approval, want under 1s", latency)
		}
		t.Logf("the requester's card updated in %s after the approval returned", latency)
		if upd.Payload["decided_at"] == nil {
			t.Fatalf("approved card has no decided_at: %v", upd.Payload)
		}
		recvCardUpdate(t, bossConn, leaveCard.Message.ID, "approved", time.Second)

		for _, f := range drainToPong(t, carolConn) {
			if f.Type == "card.updated" || f.Type == "message.created" {
				t.Fatalf("a non-member received %s: %s", f.Type, f.Data)
			}
		}
	})
}

// probeEvent is an event no module declares, so the tests below see only what
// they publish themselves.
type probeEvent struct {
	ID string `json:"id"`
}

func (probeEvent) EventName() string { return "acceptance.probe" }

// TestPLT10Outbox proves the outbox that cmd/server uses: an event published in
// a transaction that rolls back is never delivered, and events committed while
// the relay was down are delivered once a relay starts.
func TestPLT10Outbox(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)

	t.Run("an event from a rolled-back transaction is never delivered", func(t *testing.T) {
		in := newInstanceWith(t, pool, rdb, instanceConfig{outbox: true})
		delivered := make(chan string, 10)
		in.bus.Declare(probeEvent{})
		in.bus.Subscribe(probeEvent{}.EventName(), probeHandler(delivered))

		rolledBack := errors.New("roll back")
		rolledID := unique("rolled-back")
		err := in.tx.Do(context.Background(), func(ctx context.Context) error {
			if err := in.bus.Publish(ctx, probeEvent{ID: rolledID}); err != nil {
				return err
			}
			return rolledBack
		})
		if !errors.Is(err, rolledBack) {
			t.Fatalf("transaction error = %v, want the rollback error", err)
		}
		if rows, _ := outboxRows(t, pool, rolledID); rows != 0 {
			t.Fatalf("outbox rows for the rolled-back event = %d, want 0", rows)
		}

		// A committed event after it shows the relay is running. Nothing else is
		// ever delivered, because the rolled-back row never existed.
		committedID := unique("committed")
		commitProbe(t, in.tx, in.bus, committedID)
		if got := awaitProbes(t, delivered, 1, 5*time.Second); got[0] != committedID {
			t.Fatalf("delivered %v, want only %s", got, committedID)
		}
		select {
		case id := <-delivered:
			t.Fatalf("an extra event was delivered: %s", id)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("a relay restart delivers the events committed while it was down", func(t *testing.T) {
		logger := testLogger()
		ctx := context.Background()

		// The first relay runs and stops, as a process does before it goes down.
		down := eventbus.NewOutbox(pool, logger)
		down.Declare(probeEvent{})
		if err := down.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if err := down.Stop(ctx); err != nil {
			t.Fatal(err)
		}

		// Two events commit while no relay is listening.
		ids := []string{unique("pending-1"), unique("pending-2")}
		tx := database.NewTxManager(pool)
		for _, id := range ids {
			commitProbe(t, tx, down, id)
		}
		for _, id := range ids {
			if rows, published := outboxRows(t, pool, id); rows != 1 || published != 0 {
				t.Fatalf("outbox rows for %s = %d, published %d; want 1 unpublished", id, rows, published)
			}
		}

		// The restarted process declares and subscribes, then starts its relay.
		up := eventbus.NewOutbox(pool, logger)
		up.Declare(probeEvent{})
		delivered := make(chan string, 10)
		up.Subscribe(probeEvent{}.EventName(), probeHandler(delivered))
		if err := up.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_ = up.Stop(stopCtx)
		})

		if got := awaitProbes(t, delivered, len(ids), 5*time.Second); got[0] != ids[0] || got[1] != ids[1] {
			t.Fatalf("delivered %v, want %v in commit order", got, ids)
		}
		for _, id := range ids {
			if rows, published := outboxRows(t, pool, id); rows != 1 || published != 1 {
				t.Fatalf("outbox rows for %s = %d, published %d; want 1 published", id, rows, published)
			}
		}
	})
}

// probeHandler sends each delivered probe ID to delivered.
func probeHandler(delivered chan<- string) app.Handler {
	return func(_ context.Context, evt app.Event) error {
		p, ok := evt.(probeEvent)
		if !ok {
			return errors.New("not a probe event")
		}
		delivered <- p.ID
		return nil
	}
}

// awaitProbes receives n delivered probe IDs in delivery order, or fails after
// within.
func awaitProbes(t *testing.T, delivered <-chan string, n int, within time.Duration) []string {
	t.Helper()
	var got []string
	deadline := time.After(within)
	for len(got) < n {
		select {
		case id := <-delivered:
			got = append(got, id)
		case <-deadline:
			t.Fatalf("delivered %v, want %d events within %s", got, n, within)
		}
	}
	return got
}

// commitProbe publishes a probe event in its own transaction, as a use case does.
func commitProbe(t *testing.T, tx app.TxManager, bus *eventbus.Bus, id string) {
	t.Helper()
	err := tx.Do(context.Background(), func(ctx context.Context) error {
		return bus.Publish(ctx, probeEvent{ID: id})
	})
	if err != nil {
		t.Fatalf("commit %s: %v", id, err)
	}
}

// outboxRows counts the outbox rows that carry probe id, and how many are
// published.
func outboxRows(t *testing.T, pool *pgxpool.Pool, id string) (rows, published int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(published_at) FROM platform_outbox_events WHERE payload->>'id' = $1`,
		id).Scan(&rows, &published); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return rows, published
}

// messageCreated is a message.created frame. Only the fields the card tests
// read are declared.
type messageCreated struct {
	ChannelID string `json:"channel_id"`
	Message   struct {
		ID      string         `json:"id"`
		Kind    string         `json:"kind"`
		Payload map[string]any `json:"payload"`
	} `json:"message"`
}

// cardUpdated is a card.updated frame.
type cardUpdated struct {
	ChannelID string         `json:"channel_id"`
	MessageID string         `json:"message_id"`
	Payload   map[string]any `json:"payload"`
}

// recvCard reads until a message.created of the card kind arrives.
func recvCard(t *testing.T, c *websocket.Conn, kind string, within time.Duration) messageCreated {
	t.Helper()
	var got messageCreated
	recvWhere(t, c, "message.created", within, func(env app.Envelope) bool {
		got = decodeFrame[messageCreated](t, env)
		return got.Message.Kind == kind
	})
	return got
}

// recvCardUpdate reads until a card.updated for messageID with the status arrives.
func recvCardUpdate(t *testing.T, c *websocket.Conn, messageID, status string, within time.Duration) cardUpdated {
	t.Helper()
	var got cardUpdated
	recvWhere(t, c, "card.updated", within, func(env app.Envelope) bool {
		got = decodeFrame[cardUpdated](t, env)
		return got.MessageID == messageID && got.Payload["status"] == status
	})
	return got
}

// recvWhere reads frames of type typ until one satisfies match and returns it.
// Other frames are skipped; it fails when none arrives within the timeout.
func recvWhere(t *testing.T, c *websocket.Conn, typ string, within time.Duration, match func(app.Envelope) bool) app.Envelope {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			t.Fatalf("no %s frame matching the test within %s", typ, within)
		}
		env := recv(t, c, left)
		if env.Type == typ && match(env) {
			return env
		}
	}
}

// decodeFrame reads a frame's data into T.
func decodeFrame[T any](t *testing.T, env app.Envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("%s data %s: %v", env.Type, env.Data, err)
	}
	return v
}

// createGroup makes a workspace group and returns its ID (WSP-09).
func (c client) createGroup(owner user, workspaceID, name string) string {
	c.t.Helper()
	r := c.want(c.do("POST", "/workspace/groups", call{
		token: owner.access, workspace: workspaceID, body: map[string]any{"name": name},
	}), 201)
	return r.str("id")
}

// setGroupMembers replaces a group's members; is_manager makes one a manager.
func (c client) setGroupMembers(owner user, workspaceID, groupID string, members []map[string]any) {
	c.t.Helper()
	c.want(c.do("PUT", "/workspace/groups/"+groupID+"/members", call{
		token: owner.access, workspace: workspaceID, body: map[string]any{"members": members},
	}), 204)
}
