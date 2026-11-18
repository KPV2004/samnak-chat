package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

const (
	itChannel   = "0193f0f0-0000-7000-8000-0000000000c1"
	chatChannel = "0193f0f0-0000-7000-8000-0000000000c2"
)

// queueWithChannel makes an IT queue whose agents' card goes to itChannel,
// staffed by carol, and returns it with its Laptop category.
func (e *env) queueWithChannel(t *testing.T) (domain.Queue, domain.Category) {
	t.Helper()
	ctx := context.Background()
	q, err := e.svc.CreateQueue(ctx, e.p["alice"], domain.QueueInput{Name: "IT", Prefix: "IT", ChannelID: itChannel})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetAgents(ctx, e.p["alice"], q.ID, []string{carol}); err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	return q, c
}

// postedIn returns the card posted to channel, if any.
func (e *env) postedIn(channel string) (app.Card, bool) {
	for _, c := range e.cards.posted() {
		if c.ChannelID == channel {
			return c, true
		}
	}
	return app.Card{}, false
}

// payloadOf returns the payload a card was posted with.
func payloadOf(t *testing.T, c app.Card) usecase.CardPayload {
	t.Helper()
	p, ok := c.Payload.(usecase.CardPayload)
	if !ok {
		t.Fatalf("payload is %T", c.Payload)
	}
	return p
}

// lastPayload is the payload of the latest update.
func (e *env) lastPayload(t *testing.T) usecase.CardPayload {
	t.Helper()
	u, ok := e.cards.lastUpdate()
	if !ok {
		t.Fatal("no card update")
	}
	return u.Payload
}

func TestAChatTicketPostsItsTwoCardsOnceAndUpdatesThem(t *testing.T) {
	e := newEnv(t)
	q, cat := e.queueWithChannel(t)
	ctx := context.Background()
	tk, err := e.svc.Create(ctx, e.p["bob"], usecase.CreateInput{CategoryID: cat.ID, Title: "Printer jams", SourceChannelID: chatChannel})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ { // the second run is a redelivered event
		if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
			t.Fatal(err)
		}
	}

	if n := len(e.cards.posted()); n != 2 {
		t.Fatalf("posted %d cards, want 2 (requester and agents)", n)
	}
	for _, ch := range []string{chatChannel, itChannel} {
		c, ok := e.postedIn(ch)
		if !ok {
			t.Fatalf("no card posted in %s", ch)
		}
		if c.Kind != usecase.CardKind || c.SenderID != bob || c.WorkspaceID != ws {
			t.Errorf("card in %s: kind %q sender %q workspace %q; want the ticket kind, sent as the requester", ch, c.Kind, c.SenderID, c.WorkspaceID)
		}
		if c.FallbackText != "Ticket IT-0001: Printer jams (new)" {
			t.Errorf("fallback = %q", c.FallbackText)
		}
	}
	if p := payloadOf(t, mustPosted(t, e, itChannel)); p.QueueName != q.Name {
		t.Errorf("queue name = %q, want %q", p.QueueName, q.Name)
	}

	// The second run only refreshed the two stored cards.
	if n := len(e.cards.updates); n != 2 {
		t.Errorf("updates = %d, want 2 (one per stored card)", n)
	}
	stored, err := e.repo.Ticket(ctx, ws, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.MessageID == "" || stored.QueueMessageID == "" {
		t.Errorf("stored card IDs = %q and %q; both must be set", stored.MessageID, stored.QueueMessageID)
	}
}

// mustPosted returns the card posted to channel, failing the test when there is none.
func mustPosted(t *testing.T, e *env, channel string) app.Card {
	t.Helper()
	c, ok := e.postedIn(channel)
	if !ok {
		t.Fatalf("no card posted in %s", channel)
	}
	return c
}

func TestAFormTicketInAQueueWithoutAChannelHasNoCards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plain := e.createQueue(t, "alice", "HR", "HR")
	cat, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: plain.ID, Name: "Leave"})
	if err != nil {
		t.Fatal(err)
	}
	tk, err := e.svc.Create(ctx, e.p["bob"], usecase.CreateInput{CategoryID: cat.ID, Title: "Leave form"})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(e.cards.posted()); n != 0 {
		t.Errorf("posted %d cards, want none", n)
	}
}

func TestTheCardPayloadFollowsTheTicket(t *testing.T) {
	e := newEnv(t)
	q, cat := e.queueWithChannel(t)
	ctx := context.Background()
	tk, err := e.svc.Create(ctx, e.p["bob"], usecase.CreateInput{CategoryID: cat.ID, Title: "Printer jams"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
		t.Fatal(err)
	}

	// Unassigned, so the queue's agents may claim it.
	p := payloadOf(t, mustPosted(t, e, itChannel))
	if p.TicketID != tk.ID || p.Key != "IT-0001" || p.Title != "Printer jams" || p.Status != "new" || p.Priority != "normal" {
		t.Errorf("payload = %+v", p)
	}
	if p.QueueName != "IT" || p.RequesterID != bob || p.RequesterName != "Bob" || p.AssigneeID != nil || p.AssigneeName != "" {
		t.Errorf("names and assignee = %+v", p)
	}
	if p.CanClaimQueueID == nil || *p.CanClaimQueueID != q.ID {
		t.Errorf("can_claim_queue_id = %v, want %s", p.CanClaimQueueID, q.ID)
	}
	if p.Version != t0.UnixMilli() {
		t.Errorf("version = %d, want the creation time in ms %d", p.Version, t0.UnixMilli())
	}

	// The claim changes the stored card through an update, not a second post.
	e.now = t0.Add(time.Minute)
	if _, err := e.svc.Assign(ctx, e.p["carol"], tk.ID, carol); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(e.cards.posted()); n != 1 {
		t.Errorf("posted %d cards after the claim, want still 1", n)
	}
	p = e.lastPayload(t)
	if p.Status != "open" || p.AssigneeID == nil || *p.AssigneeID != carol || p.AssigneeName != "Carol" {
		t.Errorf("after the claim: payload = %+v", p)
	}
	if p.CanClaimQueueID != nil {
		t.Errorf("an assigned ticket still offers Claim: %v", *p.CanClaimQueueID)
	}
	if p.Version != e.now.UnixMilli() {
		t.Errorf("version = %d, want %d", p.Version, e.now.UnixMilli())
	}
}

func TestALostRaceUpdatesTheCardThatWon(t *testing.T) {
	e := newEnv(t)
	_, cat := e.queueWithChannel(t)
	ctx := context.Background()
	tk, err := e.svc.Create(ctx, e.p["bob"], usecase.CreateInput{CategoryID: cat.ID, Title: "Printer jams", SourceChannelID: chatChannel})
	if err != nil {
		t.Fatal(err)
	}
	// Another delivery stores its requester card while this one is posting.
	e.cards.onPost = func(c app.Card) {
		if c.ChannelID == chatChannel {
			if _, err := e.repo.SetRequesterCard(ctx, ws, tk.ID, "stored-by-other"); err != nil {
				t.Errorf("store: %v", err)
			}
		}
	}

	if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := e.repo.Ticket(ctx, ws, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.MessageID != "stored-by-other" {
		t.Errorf("requester card = %q, want the one that was stored first", stored.MessageID)
	}
	refreshed := false
	for _, u := range e.cards.updates {
		refreshed = refreshed || u.MessageID == "stored-by-other"
	}
	if !refreshed {
		t.Errorf("the stored card was not refreshed: updates = %+v", e.cards.updates)
	}
}

func TestACardFailureIsReturned(t *testing.T) {
	e := newEnv(t)
	_, cat := e.queueWithChannel(t)
	ctx := context.Background()
	tk, err := e.svc.Create(ctx, e.p["bob"], usecase.CreateInput{CategoryID: cat.ID, Title: "Printer jams"})
	if err != nil {
		t.Fatal(err)
	}
	e.cards.err = errors.New("chat is down")
	if err := e.svc.SyncCard(ctx, ws, tk.ID); err == nil {
		t.Fatal("SyncCard = nil, want the chat error so the outbox can log it")
	}
}
