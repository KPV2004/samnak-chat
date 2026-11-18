package acceptance

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify"
)

// ticketCardKind is the chat card kind of a helpdesk ticket (HD-08).
const ticketCardKind = "helpdesk.ticket"

// m5Config is cmd/server's wiring for MODULES_ENABLED=notify,helpdesk: task is
// off (HD-19), with the real rules registry, the job queue and the outbox bus.
func m5Config() instanceConfig {
	return instanceConfig{
		modules:       []string{"auth", "workspace", "chat", "notify", "helpdesk"},
		notifications: app.NewNotificationRules(),
		jobs:          true,
		outbox:        true,
	}
}

// TestSRSM5Acceptance proves the SRS 16.3 M5 outcomes on the server with
// helpdesk on and task off: a ticket made from a chat message gets its number
// and both cards, an agent walks every path of the status table, a move the
// table does not allow is 409 and the wrong person is 403, and the requester
// never sees an internal comment.
func TestSRSM5Acceptance(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	in := newInstanceWith(t, pool, rdb, m5Config())
	c := client{t: t, url: in.url}

	admin := c.signUp("alice") // workspace owner: manages the queue and sees every ticket
	rita := c.signUp("rita")   // requester
	ada := c.signUp("ada")     // agent of the IT queue; claims the tickets below
	ben := c.signUp("ben")     // agent of the IT queue; never the assignee
	carol := c.signUp("carol") // workspace member who is neither an agent nor a requester
	ws := c.createWorkspace(admin)
	for _, u := range []user{rita, ada, ben, carol} {
		c.addMember(admin, ws, u)
	}
	support := c.createChannel(admin, ws, unique("m5-support"), []string{rita.id})
	agents := c.createChannel(admin, ws, unique("m5-it-agents"), []string{ada.id, ben.id})
	queue := c.createQueue(admin, ws, "IT", "IT", agents)
	category := c.createCategory(admin, ws, queue, "Hardware")
	c.setQueueAgents(admin, ws, queue, []string{ada.id, ben.id})

	ritaConn := in.connect(t, rita.access)
	adaConn := in.connect(t, ada.access)
	benConn := in.connect(t, ben.access)
	carolConn := in.connect(t, carol.access)

	var (
		t1ID, t1ReqCard, t1AgentCard string
		t2ID, t2AgentCard            string
		t3ID                         string
	)

	t.Run("with task off, helpdesk is on and serves its queues", func(t *testing.T) {
		c := client{t: t, url: in.url}
		meta := c.want(c.do("GET", "/meta", call{}), 200)
		raw, _ := meta.Body["modules"].([]any)
		got := make([]string, 0, len(raw))
		for _, m := range raw {
			name, _ := m.(string)
			got = append(got, name)
		}
		slices.Sort(got)
		if want := []string{"auth", "chat", "helpdesk", "notify", "workspace"}; !slices.Equal(got, want) {
			t.Fatalf("/meta modules = %v, want %v", got, want)
		}
		if code := c.status("GET", "/task/items", call{token: rita.access, workspace: ws}); code != 404 {
			t.Fatalf("GET /task/items = %d, want 404 with task off", code)
		}
		queues := c.want(c.do("GET", "/helpdesk/queues", call{token: rita.access, workspace: ws}), 200).items()
		if len(queues) != 1 || queues[0]["prefix"] != "IT" {
			t.Fatalf("queues = %v, want the IT queue", queues)
		}
	})

	t.Run("a ticket made from a chat message gets its number and both cards", func(t *testing.T) {
		c := client{t: t, url: in.url}
		msg := c.want(c.send(rita, ws, support, uuid.Must(uuid.NewV7()).String(), "The printer on floor 3 is jammed"), 201)
		tk := c.want(c.do("POST", "/helpdesk/tickets", call{token: rita.access, workspace: ws, body: map[string]any{
			"category_id": category, "title": "Printer jammed on floor 3", "priority": "high", "source_message_id": msg.str("id"),
		}}), 201)
		t1ID = tk.str("id")
		if tk.str("key") != "IT-0001" || tk.Body["number"] != float64(1) || tk.str("status") != "new" {
			t.Fatalf("ticket = %v, want IT-0001 with number 1 and status new", tk.Body)
		}
		if tk.str("description") != "The printer on floor 3 is jammed" || tk.str("source_channel_id") != support || tk.str("source_message_id") != msg.str("id") {
			t.Fatalf("ticket = %v, want the message body as description, from its channel and message", tk.Body)
		}
		if tk.str("queue_id") != queue || tk.str("requester_id") != rita.id || tk.Body["assignee_id"] != nil {
			t.Fatalf("ticket = %v, want queue %s, requester rita and no assignee", tk.Body, queue)
		}

		// The requester's card is in the channel the message came from.
		req := recvTicketCard(t, ritaConn, "IT-0001", 2*time.Second)
		if req.ChannelID != support {
			t.Fatalf("requester card is in channel %s, want %s", req.ChannelID, support)
		}
		p := req.Message.Payload
		if p["ticket_id"] != t1ID || p["status"] != "new" || p["priority"] != "high" || p["queue_name"] != "IT" ||
			p["requester_name"] != "rita" || p["assignee_id"] != nil || p["can_claim_queue_id"] != queue || p["version"] != updatedAt(t, tk) {
			t.Fatalf("requester card payload = %v", p)
		}

		// The queue's card is in the agents' channel, with the same payload, and
		// the other agent sees it too.
		agent := recvTicketCard(t, adaConn, "IT-0001", 2*time.Second)
		if agent.ChannelID != agents || agent.Message.ID == req.Message.ID || !reflect.DeepEqual(agent.Message.Payload, p) {
			t.Fatalf("agents' card = %+v, want a new card in %s with the requester's payload", agent, agents)
		}
		if other := recvTicketCard(t, benConn, "IT-0001", 2*time.Second); other.Message.ID != agent.Message.ID {
			t.Fatalf("ben's card = %s, want %s", other.Message.ID, agent.Message.ID)
		}
		t1ReqCard, t1AgentCard = req.Message.ID, agent.Message.ID

		// Both are stored messages, and the requester's is sent as the requester.
		card := itemByID(c.messagesAfter(rita, ws, support, 0), t1ReqCard)
		if card == nil || card["kind"] != ticketCardKind || card["sender_id"] != rita.id {
			t.Fatalf("the requester's card in the channel history = %v", card)
		}
	})

	t.Run("an agent claims the ticket and walks every path of the state table", func(t *testing.T) {
		c := client{t: t, url: in.url}
		// Claiming is new -> open (HD-06): an agent takes the ticket for themselves.
		claim := c.want(c.do("POST", "/helpdesk/tickets/"+t1ID+"/assign", call{token: ada.access, workspace: ws,
			body: map[string]any{"assignee_id": ada.id}}), 200)
		if claim.str("status") != "open" || claim.str("assignee_id") != ada.id {
			t.Fatalf("claimed ticket = %v, want open with assignee ada", claim.Body)
		}
		awaitCards(t, claim, cardWatch{ritaConn, t1ReqCard}, cardWatch{adaConn, t1AgentCard})

		move := func(u user, to string) reply {
			r := c.setStatus(u, ws, t1ID, to)
			awaitCards(t, r, cardWatch{ritaConn, t1ReqCard}, cardWatch{adaConn, t1AgentCard})
			return r
		}
		// open -> in_progress -> waiting -> in_progress -> resolved
		move(ada, "in_progress")
		move(ada, "waiting")
		// Waiting for the requester tells the requester (HD-20).
		recvNotification(t, ritaConn, "helpdesk.status_changed", "is waiting for you", 2*time.Second)
		move(ada, "in_progress")
		resolved := move(ada, "resolved")
		if resolved.Body["resolved_at"] == nil {
			t.Fatalf("resolved ticket has no resolved_at: %v", resolved.Body)
		}
		recvNotification(t, ritaConn, "helpdesk.status_changed", "was resolved", 2*time.Second)
		// resolved -> open is the reopen: the requester, within seven days. It
		// clears resolved_at and keeps the assignee.
		reopened := move(rita, "open")
		if reopened.Body["resolved_at"] != nil || reopened.str("assignee_id") != ada.id {
			t.Fatalf("reopened ticket = %v, want no resolved_at and assignee ada", reopened.Body)
		}
		// open -> in_progress -> resolved again, then resolved -> closed by the requester.
		move(ada, "in_progress")
		move(ada, "resolved")
		closed := move(rita, "closed")
		if closed.str("status") != "closed" || closed.Body["closed_at"] == nil {
			t.Fatalf("closed ticket = %v, want status closed with closed_at", closed.Body)
		}

		// The history lines hold the path in order (HD-18). The claim is the
		// first one: new -> open.
		view := c.want(c.do("GET", "/helpdesk/tickets/"+t1ID, call{token: admin.access, workspace: ws}), 200)
		var path []string
		for _, h := range view.listOf("history") {
			if h["action"] == "status_changed" {
				from, _ := h["from"].(string)
				to, _ := h["to"].(string)
				path = append(path, from+">"+to)
			}
		}
		want := []string{
			"new>open", "open>in_progress", "in_progress>waiting", "waiting>in_progress", "in_progress>resolved",
			"resolved>open", "open>in_progress", "in_progress>resolved", "resolved>closed",
		}
		if !slices.Equal(path, want) {
			t.Fatalf("status path = %v, want %v", path, want)
		}
	})

	t.Run("an admin assigns an agent, and an admin closes a resolved ticket", func(t *testing.T) {
		c := client{t: t, url: in.url}
		form := c.want(c.do("POST", "/helpdesk/tickets", call{token: rita.access, workspace: ws, body: map[string]any{
			"category_id": category, "title": "VPN account for a new hire",
		}}), 201)
		t2ID = form.str("id")
		if form.str("key") != "IT-0002" || form.Body["source_message_id"] != nil {
			t.Fatalf("form ticket = %v, want IT-0002 with no source message", form.Body)
		}
		// A form ticket has no requester card; the queue's card still goes to the agents.
		agent := recvTicketCard(t, adaConn, "IT-0002", 2*time.Second)
		if agent.ChannelID != agents {
			t.Fatalf("agents' card is in channel %s, want %s", agent.ChannelID, agents)
		}
		t2AgentCard = agent.Message.ID

		// new -> open by an admin assigning an agent of the queue (HD-06).
		assigned := c.want(c.do("POST", "/helpdesk/tickets/"+t2ID+"/assign", call{token: admin.access, workspace: ws,
			body: map[string]any{"assignee_id": ada.id}}), 200)
		if assigned.str("status") != "open" || assigned.str("assignee_id") != ada.id {
			t.Fatalf("assigned ticket = %v, want open with assignee ada", assigned.Body)
		}
		awaitCards(t, assigned, cardWatch{adaConn, t2AgentCard})
		for _, f := range drainToPong(t, ritaConn) {
			if strings.Contains(string(f.Data), "IT-0002") {
				t.Fatalf("the requester received a frame about the form ticket: %s %s", f.Type, f.Data)
			}
		}

		step := func(u user, to string) reply {
			r := c.setStatus(u, ws, t2ID, to)
			awaitCards(t, r, cardWatch{adaConn, t2AgentCard})
			return r
		}
		step(ada, "in_progress")
		step(ada, "resolved")
		closed := step(admin, "closed")
		if closed.str("status") != "closed" || closed.Body["closed_at"] == nil {
			t.Fatalf("closed ticket = %v, want status closed with closed_at", closed.Body)
		}
	})

	t.Run("a move the table does not allow is 409 and the wrong person is 403", func(t *testing.T) {
		c := client{t: t, url: in.url}
		msg := c.want(c.send(rita, ws, support, uuid.Must(uuid.NewV7()).String(), "Could I get a charger for the projector?"), 201)
		tk := c.want(c.do("POST", "/helpdesk/tickets", call{token: rita.access, workspace: ws, body: map[string]any{
			"category_id": category, "title": "Projector charger", "source_message_id": msg.str("id"),
		}}), 201)
		t3ID = tk.str("id")
		if tk.str("key") != "IT-0003" {
			t.Fatalf("ticket = %v, want IT-0003", tk.Body)
		}
		moveTo := func(u user, to string) call {
			return call{token: u.access, workspace: ws, body: map[string]any{"status": to}}
		}
		claimCall := func(u user, assignee string) call {
			return call{token: u.access, workspace: ws, body: map[string]any{"assignee_id": assignee}}
		}
		assignURL, statusURL := "/helpdesk/tickets/"+t3ID+"/assign", "/helpdesk/tickets/"+t3ID+"/status"

		// Only an agent of the queue takes a ticket, and only for themselves. Carol
		// cannot see the ticket at all, so she gets not found before the rule applies.
		c.want(c.do("POST", assignURL, claimCall(carol, carol.id)), 404)
		c.want(c.do("POST", assignURL, claimCall(rita, rita.id)), 403)
		c.want(c.do("POST", assignURL, claimCall(ben, ada.id)), 403)
		// new -> in_progress is not in the table: 409, whoever asks.
		c.want(c.do("POST", statusURL, moveTo(ada, "in_progress")), 409)
		c.want(c.do("POST", statusURL, moveTo(rita, "resolved")), 409)

		c.setClaim(ada, ws, t3ID, ada.id)
		// Ada holds the ticket. Another agent cannot take it, and only the
		// assignee moves it.
		c.want(c.do("POST", assignURL, claimCall(ben, ben.id)), 403)
		c.want(c.do("POST", statusURL, moveTo(rita, "in_progress")), 403)
		c.want(c.do("POST", statusURL, moveTo(ben, "in_progress")), 403)
		// open -> resolved is not in the table, so it is 409 even for the assignee.
		c.want(c.do("POST", statusURL, moveTo(ada, "resolved")), 409)
		// A member who cannot see the ticket gets not found.
		c.want(c.do("POST", statusURL, moveTo(carol, "in_progress")), 404)

		c.setStatus(ada, ws, t3ID, "in_progress")
	})

	t.Run("the requester never receives internal comments", func(t *testing.T) {
		c := client{t: t, url: in.url}
		// A public comment reaches the requester as a notification (HD-20).
		public := c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/comments", call{token: ada.access, workspace: ws,
			body: map[string]any{"body": "We are checking the projector"}}), 201)
		awaitRelayed(t, pool, public.str("id"))
		recvNotification(t, ritaConn, "helpdesk.comment", "New comment on IT-0003", 2*time.Second)
		drainToPong(t, ritaConn)

		// The relay marks an event published only after its handlers return, so
		// once it is published the requester's card has been updated and every
		// frame for the requester is already queued.
		const secret = "Internal only: the projector bulb is out of stock"
		internal := c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/comments", call{token: ada.access, workspace: ws,
			body: map[string]any{"body": secret, "internal": true}}), 201)
		awaitRelayed(t, pool, internal.str("id"))
		sawCard := false
		for _, f := range drainToPong(t, ritaConn) {
			if strings.Contains(string(f.Data), secret) {
				t.Fatalf("the requester received the internal comment in a %s frame: %s", f.Type, f.Data)
			}
			sawCard = sawCard || f.Type == "card.updated"
		}
		if !sawCard {
			t.Fatalf("the requester's card was not updated for the internal comment, so the check above proves nothing")
		}
		for _, f := range drainToPong(t, adaConn) {
			if strings.Contains(string(f.Data), secret) {
				t.Fatalf("an agent's frame carries the internal comment in a card: %s %s", f.Type, f.Data)
			}
		}

		// API: the requester sees the public comment and no internal one or line.
		view := c.want(c.do("GET", "/helpdesk/tickets/"+t3ID, call{token: rita.access, workspace: ws}), 200)
		comments := view.listOf("comments")
		if len(comments) != 1 || comments[0]["id"] != public.str("id") || comments[0]["internal"] != false {
			t.Fatalf("the requester's comments = %v, want only the public one", comments)
		}
		for _, h := range view.listOf("history") {
			if h["internal"] == true {
				t.Fatalf("the requester's history has an internal line: %v", h)
			}
		}
		// The agent sees both, so the filter is what hides the note from the requester.
		if agentView := c.want(c.do("GET", "/helpdesk/tickets/"+t3ID, call{token: ada.access, workspace: ws}), 200); len(agentView.listOf("comments")) != 2 {
			t.Fatalf("the agent's comments = %v, want both", agentView.listOf("comments"))
		}
		// The requester's channel, where the card lives, holds no trace of the note.
		raw, _ := json.Marshal(c.messagesAfter(rita, ws, support, 0))
		if strings.Contains(string(raw), secret) {
			t.Fatalf("the requester's channel holds the internal comment: %s", raw)
		}
		// A requester cannot write an internal comment (HD-07).
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/comments", call{token: rita.access, workspace: ws,
			body: map[string]any{"body": "Can I see the bulb?", "internal": true}}), 403)

		// Carol is in neither the requester's channel nor the agents'.
		for _, f := range drainToPong(t, carolConn) {
			if f.Type == "card.updated" || f.Type == "message.created" || strings.Contains(string(f.Data), secret) {
				t.Fatalf("a member outside the queue received %s: %s", f.Type, f.Data)
			}
		}
	})

	t.Run("the requester closes a resolved ticket and a closed one cannot reopen", func(t *testing.T) {
		c := client{t: t, url: in.url}
		// in_progress -> resolved and in_progress -> waiting belong to the assignee.
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/status", call{token: rita.access, workspace: ws,
			body: map[string]any{"status": "resolved"}}), 403)
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/status", call{token: ben.access, workspace: ws,
			body: map[string]any{"status": "waiting"}}), 403)

		c.setStatus(ada, ws, t3ID, "resolved")
		// resolved -> closed belongs to the requester or an admin, not the assignee.
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/status", call{token: ada.access, workspace: ws,
			body: map[string]any{"status": "closed"}}), 403)
		// resolved -> in_progress is not in the table.
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/status", call{token: rita.access, workspace: ws,
			body: map[string]any{"status": "in_progress"}}), 409)

		closed := c.setStatus(rita, ws, t3ID, "closed")
		if closed.Body["closed_at"] == nil {
			t.Fatalf("closed ticket has no closed_at: %v", closed.Body)
		}
		// closed -> open is not in the table, and a closed ticket cannot be assigned.
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/status", call{token: rita.access, workspace: ws,
			body: map[string]any{"status": "open"}}), 409)
		c.want(c.do("POST", "/helpdesk/tickets/"+t3ID+"/assign", call{token: ada.access, workspace: ws,
			body: map[string]any{"assignee_id": ada.id}}), 409)
	})
}

// cardWatch is a card a test follows: its message in the socket of one user.
type cardWatch struct {
	conn      *websocket.Conn
	messageID string
}

// awaitCards waits until each watched card shows the state the ticket reply
// returned.
func awaitCards(t *testing.T, ticket reply, watches ...cardWatch) {
	t.Helper()
	for _, w := range watches {
		awaitTicketCard(t, w.conn, w.messageID, ticket)
	}
}

// awaitTicketCard reads until the card messageID shows the status of ticket
// and the version of its updated_at (HD-08), so an earlier update of the same
// status does not satisfy the wait.
func awaitTicketCard(t *testing.T, c *websocket.Conn, messageID string, ticket reply) cardUpdated {
	t.Helper()
	var got cardUpdated
	recvWhere(t, c, "card.updated", 2*time.Second, func(env app.Envelope) bool {
		got = decodeFrame[cardUpdated](t, env)
		return got.MessageID == messageID && got.Payload["status"] == ticket.str("status") &&
			got.Payload["version"] == updatedAt(t, ticket)
	})
	return got
}

// recvTicketCard reads until the helpdesk card of the ticket with key arrives.
func recvTicketCard(t *testing.T, c *websocket.Conn, key string, within time.Duration) messageCreated {
	t.Helper()
	var got messageCreated
	recvWhere(t, c, "message.created", within, func(env app.Envelope) bool {
		got = decodeFrame[messageCreated](t, env)
		return got.Message.Kind == ticketCardKind && got.Message.Payload["key"] == key
	})
	return got
}

// recvNotification reads until a notification of kind arrives whose title
// contains title.
func recvNotification(t *testing.T, c *websocket.Conn, kind, title string, within time.Duration) notify.CreatedEvent {
	t.Helper()
	var got notify.CreatedEvent
	recvWhere(t, c, notify.EventCreated, within, func(env app.Envelope) bool {
		got = decodeCreated(t, env)
		return got.Notification.Kind == kind && strings.Contains(got.Notification.Title, title)
	})
	return got
}

// awaitRelayed waits until the outbox relay has delivered the comment's
// helpdesk.comment_added event.
func awaitRelayed(t *testing.T, pool *pgxpool.Pool, commentID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var published bool
		err := pool.QueryRow(context.Background(),
			`SELECT published_at IS NOT NULL FROM platform_outbox_events WHERE name = $1 AND payload->>'comment_id' = $2`,
			helpdesk.CommentAdded{}.EventName(), commentID).Scan(&published)
		if err == nil && published {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the relay did not deliver comment %s within 5s (last error: %v)", commentID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// updatedAt is the ticket's updated_at in Unix milliseconds, which is the
// version of its card.
func updatedAt(t *testing.T, r reply) float64 {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, r.str("updated_at"))
	if err != nil {
		t.Fatalf("updated_at %q: %v", r.str("updated_at"), err)
	}
	return float64(at.UnixMilli())
}

// listOf returns the objects of the array under key.
func (r reply) listOf(key string) []map[string]any {
	raw, _ := r.Body[key].([]any)
	out := make([]map[string]any, len(raw))
	for i, it := range raw {
		out[i], _ = it.(map[string]any)
	}
	return out
}

// itemByID returns the message with id from a channel page, or nil.
func itemByID(items []map[string]any, id string) map[string]any {
	for _, it := range items {
		if it["id"] == id {
			return it
		}
	}
	return nil
}

// createQueue makes a helpdesk queue with its agents' channel and returns its
// ID (HD-01).
func (c client) createQueue(owner user, workspaceID, name, prefix, channelID string) string {
	c.t.Helper()
	r := c.want(c.do("POST", "/helpdesk/queues", call{token: owner.access, workspace: workspaceID,
		body: map[string]any{"name": name, "prefix": prefix, "channel_id": channelID}}), 201)
	return r.str("id")
}

// createCategory makes the default category of a queue and returns its ID
// (HD-02).
func (c client) createCategory(owner user, workspaceID, queueID, name string) string {
	c.t.Helper()
	r := c.want(c.do("POST", "/helpdesk/categories", call{token: owner.access, workspace: workspaceID,
		body: map[string]any{"queue_id": queueID, "name": name, "is_default": true}}), 201)
	return r.str("id")
}

// setQueueAgents replaces the agents of a queue (HD-01).
func (c client) setQueueAgents(owner user, workspaceID, queueID string, userIDs []string) {
	c.t.Helper()
	c.want(c.do("PUT", "/helpdesk/queues/"+queueID+"/agents", call{token: owner.access, workspace: workspaceID,
		body: map[string]any{"user_ids": userIDs}}), 204)
}

// setStatus moves a ticket to status as u and returns the ticket.
func (c client) setStatus(u user, workspaceID, ticketID, status string) reply {
	c.t.Helper()
	return c.want(c.do("POST", "/helpdesk/tickets/"+ticketID+"/status", call{token: u.access, workspace: workspaceID,
		body: map[string]any{"status": status}}), 200)
}

// setClaim makes assignee the assignee of a ticket, as u (HD-06).
func (c client) setClaim(u user, workspaceID, ticketID, assignee string) reply {
	c.t.Helper()
	return c.want(c.do("POST", "/helpdesk/tickets/"+ticketID+"/assign", call{token: u.access, workspace: workspaceID,
		body: map[string]any{"assignee_id": assignee}}), 200)
}
