package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/jobs"
)

// TestSRSM3Acceptance proves the SRS 16.3 M3 outcomes against the real
// registry, hub, Postgres and Redis: a mention raises the mentioned user's bell
// over WebSocket, the count survives a reconnect, and read state reaches the
// user's other device. notify and the River queue are wired as cmd/server does.
func TestSRSM3Acceptance(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	// MODULES_ENABLED unset: core modules plus notify, with the real rules
	// registry and job queue from cmd/server.
	cfg := instanceConfig{
		modules:       notifyModuleNames,
		notifications: app.NewNotificationRules(),
		jobs:          true,
	}
	one := newInstanceWith(t, pool, rdb, cfg)
	two := newInstanceWith(t, pool, rdb, cfg)
	c := client{t: t, url: one.url}
	parentT := t

	alice := c.signUp("alice")
	bob := c.signUp("bob")
	workspaceID := c.createWorkspace(alice)
	c.addMember(alice, workspaceID, bob)
	channelName := unique("m3-room")
	channelID := c.createChannel(alice, workspaceID, channelName, []string{bob.id})

	// Bob has two devices on two instances, like a desktop and a phone.
	phone := c.login(bob, "Phone")
	aliceConn := one.connect(t, alice.access)
	bobDesk := two.connect(t, bob.access)
	bobPhone := one.connect(t, phone.access)

	var firstID, secondID string

	t.Run("a mention reaches the mentioned user within one second", func(t *testing.T) {
		sent := c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(),
			"please review <@"+bob.id+">"), 201)
		// SRS 16.3: measured from the HTTP response to the recipient's frame.
		posted := time.Now()
		env := recvType(t, bobDesk, notify.EventCreated, time.Second)
		latency := time.Since(posted)
		if latency >= time.Second {
			t.Fatalf("bell frame took %s after the POST returned, want under 1s", latency)
		}
		t.Logf("notification.created arrived in %s after the POST returned", latency)

		created := decodeCreated(t, env)
		if created.WorkspaceID != workspaceID {
			t.Fatalf("frame workspace = %q, want %q", created.WorkspaceID, workspaceID)
		}
		if created.UnreadCount != 1 {
			t.Fatalf("unread_count = %d, want 1", created.UnreadCount)
		}
		n := created.Notification
		if n.Kind != "chat.mention" {
			t.Fatalf("kind = %q, want chat.mention", n.Kind)
		}
		if n.Title != "alice in #"+channelName {
			t.Fatalf("title = %q, want %q", n.Title, "alice in #"+channelName)
		}
		if n.Body != "please review @bob" {
			t.Fatalf("body = %q, want %q", n.Body, "please review @bob")
		}
		if n.Link != "/chat/"+channelID {
			t.Fatalf("link = %q, want %q", n.Link, "/chat/"+channelID)
		}
		if n.Data["message_id"] != sent.str("id") || n.Data["channel_id"] != channelID {
			t.Fatalf("data = %v, want message %s in channel %s", n.Data, sent.str("id"), channelID)
		}
		if n.Data["sender_name"] != "alice" || n.Data["channel_name"] != channelName {
			t.Fatalf("data = %v, want sender_name alice and channel_name %s", n.Data, channelName)
		}
		if n.ReadAt != nil {
			t.Fatalf("read_at = %v, want null for a new notification", n.ReadAt)
		}
		firstID = n.ID

		// The user's other device gets the same notification.
		phoneEvt := decodeCreated(t, recvType(t, bobPhone, notify.EventCreated, time.Second))
		if phoneEvt.Notification.ID != firstID || phoneEvt.UnreadCount != 1 {
			t.Fatalf("phone frame = %+v, want notification %s with unread_count 1", phoneEvt, firstID)
		}

		// The sender is never notified about her own message.
		for _, f := range drainToPong(t, aliceConn) {
			if f.Type == notify.EventCreated {
				t.Fatalf("the sender received a notification: %s", f.Data)
			}
		}

		if got := unreadCount(t, c, bob, workspaceID); got != 1 {
			t.Fatalf("bob unread after the mention = %d, want 1", got)
		}
		if got := unreadCount(t, c, alice, workspaceID); got != 0 {
			t.Fatalf("alice unread after the mention = %d, want 0", got)
		}
	})

	t.Run("reconnecting finds the same unread count", func(t *testing.T) {
		// The desktop drops its socket, as a reload does.
		_ = bobDesk.CloseNow()
		// Clean up with the parent test: the socket is used by later subtests.
		bobDesk = two.connect(parentT, bob.access)
		if frames := drainToPong(t, bobDesk); len(frames) != 0 {
			t.Fatalf("reconnect replayed %d frames, want none", len(frames))
		}

		if got := unreadCount(t, c, bob, workspaceID); got != 1 {
			t.Fatalf("unread after reconnecting = %d, want 1", got)
		}
		page := c.want(c.do("GET", "/notify/notifications", call{
			token: bob.access, workspace: workspaceID,
		}), 200)
		items := page.items()
		if len(items) != 1 {
			t.Fatalf("notification list = %d items, want 1: %v", len(items), items)
		}
		if items[0]["id"] != firstID || items[0]["kind"] != "chat.mention" || items[0]["read_at"] != nil {
			t.Fatalf("notification = %v, want unread chat.mention %s", items[0], firstID)
		}
	})

	t.Run("a second mention raises the exact count", func(t *testing.T) {
		c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(),
			"and again <@"+bob.id+">"), 201)
		created := decodeCreated(t, recvType(t, bobDesk, notify.EventCreated, time.Second))
		if created.UnreadCount != 2 {
			t.Fatalf("unread_count in the frame = %d, want 2", created.UnreadCount)
		}
		secondID = created.Notification.ID
		// The second device keeps the same count.
		phoneEvt := decodeCreated(t, recvType(t, bobPhone, notify.EventCreated, time.Second))
		if phoneEvt.UnreadCount != 2 {
			t.Fatalf("phone unread_count = %d, want 2", phoneEvt.UnreadCount)
		}
		if got := unreadCount(t, c, bob, workspaceID); got != 2 {
			t.Fatalf("unread = %d, want 2", got)
		}
	})

	t.Run("marking one read reaches the other device", func(t *testing.T) {
		c.want(c.do("POST", "/notify/notifications/"+firstID+"/read", call{
			token: bob.access, workspace: workspaceID,
		}), 204)

		// The phone, the other device, clears the bell.
		read := decodeRead(t, recvType(t, bobPhone, notify.EventRead, time.Second))
		if read.WorkspaceID != workspaceID || read.All || read.UnreadCount != 1 {
			t.Fatalf("read frame = %+v, want workspace %s, all=false, unread_count 1", read, workspaceID)
		}
		if !slices.Equal(read.IDs, []string{firstID}) {
			t.Fatalf("read frame ids = %v, want [%s]", read.IDs, firstID)
		}
		// The desktop that made the request hears about it too.
		own := decodeRead(t, recvType(t, bobDesk, notify.EventRead, time.Second))
		if own.UnreadCount != 1 || !slices.Equal(own.IDs, []string{firstID}) {
			t.Fatalf("desktop read frame = %+v, want ids [%s] and unread_count 1", own, firstID)
		}
		if got := unreadCount(t, c, bob, workspaceID); got != 1 {
			t.Fatalf("unread after marking one read = %d, want 1", got)
		}
	})

	t.Run("read-all on one device reaches the other", func(t *testing.T) {
		c.want(c.do("POST", "/notify/read-all", call{
			token: phone.access, workspace: workspaceID,
		}), 204)

		read := decodeRead(t, recvType(t, bobDesk, notify.EventRead, time.Second))
		if read.WorkspaceID != workspaceID || !read.All || read.UnreadCount != 0 {
			t.Fatalf("read-all frame = %+v, want all=true and unread_count 0", read)
		}
		// ids names the rows this call marked read: the one still unread.
		if !slices.Equal(read.IDs, []string{secondID}) {
			t.Fatalf("read-all frame ids = %v, want [%s]", read.IDs, secondID)
		}
		if got := unreadCount(t, c, bob, workspaceID); got != 0 {
			t.Fatalf("unread after read-all = %d, want 0", got)
		}
		if got := unreadRows(t, pool, workspaceID, bob.id); got != 0 {
			t.Fatalf("bob's unread rows = %d, want 0", got)
		}
		if got := countNotifications(t, pool, workspaceID, bob.id); got != 2 {
			t.Fatalf("bob's notification rows = %d, want 2", got)
		}
		if got := countNotifications(t, pool, workspaceID, alice.id); got != 0 {
			t.Fatalf("alice's notification rows = %d, want 0", got)
		}
	})
}

// TestSRSM3NotifyDisabled proves NOTI-05: with MODULES_ENABLED= (notify off)
// chat still works, nothing is stored for the rules chat registered, and the
// notify API is not mounted.
func TestSRSM3NotifyDisabled(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	// The equivalent of MODULES_ENABLED=: core only. cmd/server still passes
	// the real rules registry and the job queue.
	in := newInstanceWith(t, pool, rdb, instanceConfig{
		modules:       moduleNames,
		notifications: app.NewNotificationRules(),
		jobs:          true,
	})
	c := client{t: t, url: in.url}

	alice := c.signUp("alice")
	bob := c.signUp("bob")
	workspaceID := c.createWorkspace(alice)
	c.addMember(alice, workspaceID, bob)
	channelID := c.createChannel(alice, workspaceID, unique("no-notify"), []string{bob.id})

	bobConn := in.connect(t, bob.access)

	t.Run("messages and mentions still arrive", func(t *testing.T) {
		sent := c.want(c.send(alice, workspaceID, channelID, uuid.Must(uuid.NewV7()).String(),
			"hello <@"+bob.id+">"), 201)
		posted := time.Now()
		evt := decodeEvent(t, recvType(t, bobConn, "message.created", time.Second))
		if latency := time.Since(posted); latency >= time.Second {
			t.Fatalf("message frame took %s, want under 1s", latency)
		}
		if evt.Message.ID != sent.str("id") {
			t.Fatalf("frame message = %s, want %s", evt.Message.ID, sent.str("id"))
		}
		if frames := drainToPong(t, bobConn); len(frames) != 0 {
			for _, f := range frames {
				if f.Type == notify.EventCreated {
					t.Fatalf("a notification frame arrived with notify disabled: %s", f.Data)
				}
			}
		}
	})

	t.Run("direct messages still arrive and store nothing", func(t *testing.T) {
		dmID := c.openDM(alice, workspaceID, bob.id)
		sent := c.want(c.send(alice, workspaceID, dmID, uuid.Must(uuid.NewV7()).String(), "just us"), 201)
		evt := decodeEvent(t, recvType(t, bobConn, "message.created", time.Second))
		if evt.Message.ID != sent.str("id") || evt.ChannelID != dmID {
			t.Fatalf("DM frame = %+v, want message %s in %s", evt, sent.str("id"), dmID)
		}
		if frames := drainToPong(t, bobConn); len(frames) != 0 {
			for _, f := range frames {
				if f.Type == notify.EventCreated {
					t.Fatalf("a notification frame arrived with notify disabled: %s", f.Data)
				}
			}
		}
		if got := countNotifications(t, pool, workspaceID, ""); got != 0 {
			t.Fatalf("notification rows with notify off = %d, want 0", got)
		}
	})

	t.Run("the notify API is not mounted", func(t *testing.T) {
		anyID := uuid.Must(uuid.NewV7()).String()
		for _, tc := range []struct{ name, method, path string }{
			{"unread count", "GET", "/notify/unread-count"},
			{"list", "GET", "/notify/notifications"},
			{"mark read", "POST", "/notify/notifications/" + anyID + "/read"},
			{"read all", "POST", "/notify/read-all"},
		} {
			if status := c.status(tc.method, tc.path, call{token: bob.access, workspace: workspaceID}); status != 404 {
				t.Errorf("%s: status = %d, want 404", tc.name, status)
			}
		}

		// /meta lists the modules the web hides the bell for.
		meta := c.want(c.do("GET", "/meta", call{}), 200)
		raw, _ := meta.Body["modules"].([]any)
		var names []string
		for _, m := range raw {
			if s, ok := m.(string); ok {
				names = append(names, s)
			}
		}
		if slices.Contains(names, "notify") {
			t.Fatalf("/meta modules = %v, want no notify", names)
		}
		for _, want := range []string{"auth", "workspace", "chat"} {
			if !slices.Contains(names, want) {
				t.Fatalf("/meta modules = %v, want %s", names, want)
			}
		}
	})
}

// TestSRSM3Jobs proves PLT-09 on the queue cmd/server builds: a job enqueued
// inside a transaction that rolls back never exists or runs, a committed one
// runs, and a job whose kind has no handler on this server is cancelled.
func TestSRSM3Jobs(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)

	rollbackKind := unique("acceptance.rollback")
	commitKind := unique("acceptance.commit")
	orphanKind := unique("acceptance.orphan")
	unknownKind := unique("acceptance.unregistered")

	type result struct {
		kind string
		args string
	}
	ran := make(chan result, 10)
	record := func(kind string) app.JobHandler {
		return func(_ context.Context, args json.RawMessage) error {
			ran <- result{kind: kind, args: string(args)}
			return nil
		}
	}

	// One full server instance owns the queue. The test's kinds are registered
	// where a module's Init would, and the sweep is short so the cancellation
	// is observable.
	in := newInstanceWith(t, pool, rdb, instanceConfig{
		modules:       notifyModuleNames,
		notifications: app.NewNotificationRules(),
		jobs:          true,
		sweepInterval: 100 * time.Millisecond,
		jobHandlers: func(q *jobs.Queue) {
			q.Handle(rollbackKind, record(rollbackKind), app.JobOptions{})
			q.Handle(commitKind, record(commitKind), app.JobOptions{})
		},
	})

	t.Run("a job enqueued in a rolled-back transaction never runs", func(t *testing.T) {
		rolledBack := errors.New("roll back")
		err := in.tx.Do(context.Background(), func(ctx context.Context) error {
			if err := in.jobs.Enqueue(ctx, rollbackKind, map[string]string{"state": "rolled back"}, app.EnqueueOptions{}); err != nil {
				return err
			}
			return rolledBack
		})
		if !errors.Is(err, rolledBack) {
			t.Fatalf("transaction error = %v, want the rollback error", err)
		}

		select {
		case got := <-ran:
			t.Fatalf("a handler ran for a rolled-back job: %+v", got)
		case <-time.After(500 * time.Millisecond):
		}
		if n := jobCount(t, pool, rollbackKind); n != 0 {
			t.Fatalf("river_job rows for %s = %d, want 0", rollbackKind, n)
		}
	})

	t.Run("a committed job runs with its args", func(t *testing.T) {
		err := in.tx.Do(context.Background(), func(ctx context.Context) error {
			return in.jobs.Enqueue(ctx, commitKind, map[string]string{"state": "committed"}, app.EnqueueOptions{})
		})
		if err != nil {
			t.Fatalf("commit transaction: %v", err)
		}

		select {
		case got := <-ran:
			if got.kind != commitKind {
				t.Fatalf("handler ran for %s, want %s", got.kind, commitKind)
			}
			var args map[string]string
			if err := json.Unmarshal([]byte(got.args), &args); err != nil {
				t.Fatalf("args = %s: %v", got.args, err)
			}
			if args["state"] != "committed" {
				t.Fatalf("args = %v, want state committed", args)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the committed job did not run")
		}
		if state, _ := waitJobState(t, pool, commitKind, "completed", 5*time.Second); state != "completed" {
			t.Fatalf("committed job state = %s, want completed", state)
		}
	})

	t.Run("a job of a kind with no handler is cancelled", func(t *testing.T) {
		// Another deployment inserted the job before this one lost the module.
		insertRawJob(t, pool, orphanKind)
		state, attempts := waitJobState(t, pool, orphanKind, "cancelled", 5*time.Second)
		if state != "cancelled" {
			t.Fatalf("state = %s, want cancelled", state)
		}
		if attempts != 0 {
			t.Fatalf("cancelled job attempts = %d, want 0: it must never run", attempts)
		}
	})

	t.Run("enqueuing a kind with no handler here is refused", func(t *testing.T) {
		err := in.jobs.Enqueue(context.Background(), unknownKind, map[string]string{}, app.EnqueueOptions{})
		if err == nil {
			t.Fatalf("Enqueue of %s succeeded, want an error", unknownKind)
		}
		if n := jobCount(t, pool, unknownKind); n != 0 {
			t.Fatalf("river_job rows for %s = %d, want 0", unknownKind, n)
		}
	})
}

// createChannel makes a private channel with the given members.
func (c client) createChannel(u user, workspaceID, name string, memberIDs []string) string {
	c.t.Helper()
	r := c.want(c.do("POST", "/chat/channels", call{
		token: u.access, workspace: workspaceID,
		body: map[string]any{"type": "private", "name": name, "member_ids": memberIDs},
	}), 201)
	return r.str("id")
}

// unreadCount is GET /notify/unread-count for one user.
func unreadCount(t *testing.T, c client, u user, workspaceID string) int64 {
	t.Helper()
	r := c.want(c.do("GET", "/notify/unread-count", call{token: u.access, workspace: workspaceID}), 200)
	count, ok := r.Body["count"].(float64)
	if !ok {
		t.Fatalf("unread-count response has no count: %v", r.Body)
	}
	return int64(count)
}

// countNotifications reads the notify table the way the module's owner would;
// userID "" counts every user in the workspace.
func countNotifications(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string) int {
	t.Helper()
	query := `SELECT count(*) FROM notify_notifications WHERE workspace_id = $1`
	args := []any{workspaceID}
	if userID != "" {
		query += ` AND user_id = $2`
		args = append(args, userID)
	}
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}

// unreadRows counts the rows a user still has to read in a workspace.
func unreadRows(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notify_notifications WHERE workspace_id = $1 AND user_id = $2 AND read_at IS NULL`,
		workspaceID, userID).Scan(&n); err != nil {
		t.Fatalf("count unread notifications: %v", err)
	}
	return n
}

func decodeCreated(t *testing.T, env app.Envelope) notify.CreatedEvent {
	t.Helper()
	var e notify.CreatedEvent
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatalf("notification.created data %s: %v", env.Data, err)
	}
	return e
}

func decodeRead(t *testing.T, env app.Envelope) notify.ReadEvent {
	t.Helper()
	var e notify.ReadEvent
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatalf("notification.read data %s: %v", env.Data, err)
	}
	return e
}

// rawArgs inserts a job whose kind this server does not know, as a deployment
// with that module had left behind.
type rawArgs struct {
	Name string `json:"kind"`
}

func (a rawArgs) Kind() string { return a.Name }

// insertRawJob writes a job directly with an insert-only client, since the
// server can only enqueue kinds it has a worker for.
func insertRawJob(t *testing.T, pool *pgxpool.Pool, kind string) {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: testLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insert(context.Background(), rawArgs{Name: kind}, nil); err != nil {
		t.Fatalf("insert %s: %v", kind, err)
	}
}

func jobCount(t *testing.T, pool *pgxpool.Pool, kind string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count jobs %s: %v", kind, err)
	}
	return n
}

// waitJobState waits for the newest job of kind to reach want and returns its
// state and attempt count.
func waitJobState(t *testing.T, pool *pgxpool.Pool, kind, want string, within time.Duration) (state string, attempts int) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		err := pool.QueryRow(context.Background(),
			`SELECT state::text, attempt FROM river_job WHERE kind = $1 ORDER BY id DESC LIMIT 1`, kind).
			Scan(&state, &attempts)
		if err != nil {
			t.Fatalf("read job %s: %v", kind, err)
		}
		if state == want {
			return state, attempts
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s state = %s after %s, want %s", kind, state, within, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
