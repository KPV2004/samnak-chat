package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

func TestSyncCardPostsTheCardOnceAsTheRequester(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request

	if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.cards.posts) != 1 || len(r.cards.updates) != 0 {
		t.Fatalf("posts = %d, updates = %d", len(r.cards.posts), len(r.cards.updates))
	}
	post := r.cards.posts[0]
	if post.WorkspaceID != ws || post.RequesterID != alice || post.ApproverID != bob {
		t.Errorf("post = %+v", post)
	}
	c := post.Payload
	if c.RequestID != req.ID || c.Type != "leave" || c.Status != "pending" || c.Summary != "2026-12-01 to 2026-12-03" ||
		c.RequesterID != alice || c.RequesterName != "Alice" || c.ApproverID != bob || c.ApproverName != "Bob" ||
		c.Reason != "" || c.DecidedAt != nil || c.Version != req.UpdatedAt.UnixMilli() {
		t.Errorf("payload = %+v", c)
	}
	if c.Fields["leave_type"] != "annual" || c.Fields["reason"] != "trip" {
		t.Errorf("fields = %v", c.Fields)
	}
	if post.Fallback != "Approval request: 2026-12-01 to 2026-12-03 (pending)" {
		t.Errorf("fallback = %q", post.Fallback)
	}
	stored := r.repo.request(t, req.ID)
	if stored.MessageID == "" || stored.ChannelID == "" {
		t.Errorf("the card was not stored on the request: %+v", stored)
	}
}

func TestSyncCardTwiceUpdatesInsteadOfPostingAgain(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	for range 2 {
		if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cards.posts) != 1 || len(r.cards.updates) != 1 {
		t.Fatalf("posts = %d, updates = %d, want 1 and 1 (a redelivered event)", len(r.cards.posts), len(r.cards.updates))
	}
	if r.cards.updatedID[0] != r.repo.request(t, req.ID).MessageID {
		t.Errorf("updated message %s", r.cards.updatedID[0])
	}
}

func TestSyncCardShowsEveryDecision(t *testing.T) {
	cases := []struct {
		name   string
		decide func(r *rig, id string) error
		status string
		reason string
	}{
		{"approve", func(r *rig, id string) error { _, err := r.svc.Approve(context.Background(), p(bob), id); return err }, "approved", ""},
		{"reject", func(r *rig, id string) error {
			_, err := r.svc.Reject(context.Background(), p(bob), id, "no cover")
			return err
		}, "rejected", "no cover"},
		{"cancel", func(r *rig, id string) error { _, err := r.svc.Cancel(context.Background(), p(alice), id); return err }, "cancelled", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			req := r.submit(t).Request
			if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
				t.Fatal(err)
			}
			r.advance(time.Second)
			if err := c.decide(r, req.ID); err != nil {
				t.Fatal(err)
			}
			if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
				t.Fatal(err)
			}
			if len(r.cards.updates) != 1 {
				t.Fatalf("updates = %d", len(r.cards.updates))
			}
			u := r.cards.updates[0]
			if u.Status != c.status || u.Reason != c.reason || u.DecidedAt == nil {
				t.Errorf("payload = %+v", u)
			}
			if u.Version <= r.cards.posts[0].Payload.Version {
				t.Errorf("version %d did not grow past %d", u.Version, r.cards.posts[0].Payload.Version)
			}
		})
	}
}

func TestSyncCardAfterADecisionPostsWhenTheFirstEventWasLost(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	if _, err := r.svc.Approve(context.Background(), p(bob), req.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.cards.posts) != 1 || r.cards.posts[0].Payload.Status != "approved" {
		t.Fatalf("posts = %+v, want the card posted with the current status", r.cards.posts)
	}
}

func TestSyncCardFailureLeavesNoCardSoARedeliveryPosts(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	boom := errors.New("chat is down")
	r.cards.postErr = boom
	if err := r.svc.SyncCard(context.Background(), ws, req.ID); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if r.repo.request(t, req.ID).MessageID != "" {
		t.Error("a message ID was stored for a card that was never posted")
	}
	r.cards.postErr = nil
	if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.cards.posts) != 1 {
		t.Errorf("posts = %d", len(r.cards.posts))
	}
}

func TestSyncCardWithoutChatDoesNothing(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	svc := usecase.New(usecase.Deps{Tx: &memTx{repo: r.repo, bus: r.bus}, Repo: r.repo, Bus: r.bus, Log: nil})
	if err := svc.SyncCard(context.Background(), ws, req.ID); err != nil {
		t.Fatalf("error = %v, want nil: a server without cards still has approvals", err)
	}
}

func TestSyncCardLosingTheRaceUpdatesTheWinnersCard(t *testing.T) {
	r := newRig(t)
	req := r.submit(t).Request
	// Another delivery stored its card between our read and our SetCard.
	r.svc = newServiceWithRepo(r, &lateCardRepo{fakeRepo: r.repo, id: req.ID})
	if err := r.svc.SyncCard(context.Background(), ws, req.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.cards.updates) != 1 || r.cards.updatedID[0] != "0193f0f0-0000-7000-8000-0000000000e1" {
		t.Errorf("updates = %v on %v, want an update of the stored card", len(r.cards.updates), r.cards.updatedID)
	}
}

// lateCardRepo stores a competing card just before SetCard runs.
type lateCardRepo struct {
	*fakeRepo
	id string
}

func (l *lateCardRepo) SetCard(ctx context.Context, workspaceID, id, channelID, messageID string) (bool, error) {
	if _, err := l.fakeRepo.SetCard(ctx, workspaceID, id, "0193f0f0-0000-7000-8000-0000000000c1", "0193f0f0-0000-7000-8000-0000000000e1"); err != nil {
		return false, err
	}
	return l.fakeRepo.SetCard(ctx, workspaceID, id, channelID, messageID)
}

func TestSyncCardForAMissingRequestIsAnError(t *testing.T) {
	r := newRig(t)
	if err := r.svc.SyncCard(context.Background(), ws, "0193f0f0-0000-7000-8000-0000000000ff"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCardFallbackTextForGeneralRequests(t *testing.T) {
	r := newRig(t)
	d, err := r.svc.Submit(context.Background(), p(alice), usecase.SubmitInput{
		Type: "general", Payload: []byte(`{"subject":"New laptop","details":"d","amount":"30000"}`), ApproverID: bob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SyncCard(context.Background(), ws, d.Request.ID); err != nil {
		t.Fatal(err)
	}
	post := r.cards.posts[0]
	if post.Fallback != "Approval request: New laptop (pending)" || post.Payload.Summary != "New laptop" || post.Payload.Fields["amount"] != "30000.00" {
		t.Errorf("post = %+v", post)
	}
	if strings.Contains(post.Fallback, "\n") {
		t.Error("the fallback must be one line")
	}
}
