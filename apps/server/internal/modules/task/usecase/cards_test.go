package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

func (e *env) sync(id string) error {
	return e.uc.SyncCard(context.Background(), ws, id)
}

func TestSyncCardPostsOnceAndStoresTheMessage(t *testing.T) {
	e := newEnv(t)
	due := clock.Add(36 * time.Hour)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Fix login", AssigneeID: bob, DueAt: &due, Priority: domain.PriorityHigh})

	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	if len(e.cards.posts) != 1 || len(e.cards.updates) != 0 {
		t.Fatalf("posts=%d updates=%d, want 1 and 0", len(e.cards.posts), len(e.cards.updates))
	}
	post := e.cards.posts[0]
	c := post.Card
	if c.WorkspaceID != ws || c.ChannelID != ch || c.SenderID != alice || c.Kind != "task.item" || c.FallbackText != "Task: Fix login" || c.ThreadRootID != "" {
		t.Fatalf("card = %+v", c)
	}
	payload, ok := c.Payload.(usecase.CardPayload)
	if !ok {
		t.Fatalf("payload is %T", c.Payload)
	}
	if payload.TaskID != task.ID || payload.Title != "Fix login" || payload.Status != "todo" || payload.Priority != "high" ||
		payload.AssigneeID == nil || *payload.AssigneeID != bob || payload.AssigneeName != "Bob" ||
		payload.DueAt == nil || !payload.DueAt.Equal(due) || payload.Version != clock.UnixMilli() {
		t.Fatalf("payload = %+v", payload)
	}
	if stored, _ := e.repo.Get(context.Background(), ws, task.ID); stored.MessageID != post.ID {
		t.Fatalf("message_id = %q, want %q", stored.MessageID, post.ID)
	}
}

func TestSyncCardRedeliveryOnlyUpdatesThePayload(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Fix login"})
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}

	e.now = clock.Add(time.Minute)
	if _, err := e.update(asAlice, task.ID, domain.Patch{Status: ptr(domain.StatusDoing)}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // the same event delivered twice
		if err := e.sync(task.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.cards.posts) != 1 {
		t.Fatalf("posts = %d, a redelivered event must not post again", len(e.cards.posts))
	}
	if len(e.cards.updates) != 2 {
		t.Fatalf("updates = %d, want 2", len(e.cards.updates))
	}
	u := e.cards.updates[1]
	p := u.Payload.(usecase.CardPayload)
	if u.WorkspaceID != ws || u.MessageID != e.cards.posts[0].ID || p.Status != "doing" || p.Version != clock.Add(time.Minute).UnixMilli() {
		t.Fatalf("update = %+v payload = %+v", u, p)
	}
	if p.AssigneeID != nil || p.AssigneeName != "" || p.DueAt != nil {
		t.Fatalf("a task without assignee or due date carries none: %+v", p)
	}
}

func TestSyncCardLosingTheRaceUpdatesTheWinnersCard(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Fix login"})
	winner := "0193f0f0-0000-7000-8000-00000000ffff"
	// Another delivery posts and stores its card while this one is posting.
	e.cards.beforePost = func() {
		stored := e.repo.tasks[task.ID]
		stored.MessageID = winner
		e.repo.tasks[task.ID] = stored
	}
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.tasks[task.ID].MessageID; got != winner {
		t.Fatalf("message_id = %q, want the winner's %q", got, winner)
	}
	if len(e.cards.updates) != 1 || e.cards.updates[0].MessageID != winner {
		t.Fatalf("updates = %+v, want the winner's card refreshed", e.cards.updates)
	}
}

func TestSyncCardErrors(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"})

	e.cards.postErr = errors.New("chat down")
	if err := e.sync(task.ID); err == nil {
		t.Fatal("want the post error so the event is retried")
	}
	if e.repo.tasks[task.ID].MessageID != "" {
		t.Fatal("a failed post stored a message id")
	}
	e.cards.postErr = nil

	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	e.cards.updateErr = errors.New("chat down")
	if err := e.sync(task.ID); err == nil {
		t.Fatal("want the update error")
	}

	if err := e.sync("0193f0f0-0000-7000-8000-00000000eeee"); err != nil {
		t.Fatalf("a task that is gone has no card to sync: %v", err)
	}
}

func TestSyncCardWithoutACardRegistryDoesNothing(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"})
	e.cards = nil
	e.uc = usecase.New(usecase.Deps{Tx: e.tx, Repo: e.repo, Channels: e.channels, Members: e.members, Events: e.events, Now: func() time.Time { return clock }})
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSyncCardWithoutADirectoryLeavesTheNameEmpty(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x", AssigneeID: bob})
	e.dir = nil
	e.build()
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	p := e.cards.posts[0].Card.Payload.(usecase.CardPayload)
	if p.AssigneeName != "" || p.AssigneeID == nil {
		t.Fatalf("payload = %+v", p)
	}
}

func TestCardVersionNeverGoesBackwards(t *testing.T) {
	e := newEnv(t)
	task := e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"})
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	first := e.cards.posts[0].Card.Payload.(usecase.CardPayload).Version
	e.now = clock.Add(2 * time.Millisecond)
	if _, err := e.update(asAlice, task.ID, domain.Patch{Title: ptr("y")}); err != nil {
		t.Fatal(err)
	}
	if err := e.sync(task.ID); err != nil {
		t.Fatal(err)
	}
	if second := e.cards.updates[0].Payload.(usecase.CardPayload).Version; second <= first {
		t.Fatalf("version %d after %d", second, first)
	}
}
