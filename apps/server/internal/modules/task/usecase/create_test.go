package usecase_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

func TestCreateStoresTheTaskForTheCaller(t *testing.T) {
	e := newEnv(t)
	due := clock.Add(48 * time.Hour)
	got, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{
		ChannelID: ch, Title: "  Fix login ", Description: "see ticket", AssigneeID: bob, DueAt: &due,
		Priority: domain.PriorityHigh, SourceMessageID: "0193f0f0-0000-7000-8000-0000000000d1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != ws || got.CreatorID != alice || got.ChannelID != ch || got.AssigneeID != bob || got.Title != "Fix login" ||
		got.Status != domain.StatusTodo || got.Priority != domain.PriorityHigh || got.SourceMessageID == "" || got.ID == "" {
		t.Fatalf("task = %+v", got)
	}
	if !got.CreatedAt.Equal(clock) || !got.UpdatedAt.Equal(clock) {
		t.Fatalf("clock = %v / %v", got.CreatedAt, got.UpdatedAt)
	}
	if stored, err := e.repo.Get(context.Background(), ws, got.ID); err != nil || stored.Title != "Fix login" {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
}

func TestCreatePublishesInsideTheTransaction(t *testing.T) {
	e := newEnv(t)
	e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "No owner yet"})
	if got := e.events.got(); !slices.Equal(got, []string{"created"}) {
		t.Fatalf("events = %v, want created only", got)
	}

	e.create(asAlice, usecase.CreateInput{ChannelID: ch, Title: "Owned", AssigneeID: bob})
	if got := e.events.got(); !slices.Equal(got, []string{"created", "created", "assigned"}) {
		t.Fatalf("events = %v", got)
	}
	as := e.events.last["assigned"].(assignedEvent)
	if as.Actor != alice || as.Task.AssigneeID != bob {
		t.Fatalf("assigned = %+v", as)
	}
	if len(e.events.outTx) != 0 {
		t.Fatalf("published outside the transaction: %v", e.events.outTx)
	}
}

func TestCreateRollsBackWhenPublishFails(t *testing.T) {
	e := newEnv(t)
	e.events.failOn = "created"
	if _, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"}); err == nil {
		t.Fatal("want the publish error")
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	cases := map[string]usecase.CreateInput{
		"empty title":        {ChannelID: ch, Title: " "},
		"bad channel id":     {ChannelID: "nope", Title: "x"},
		"empty channel":      {Title: "x"},
		"bad assignee id":    {ChannelID: ch, Title: "x", AssigneeID: "nope"},
		"bad source message": {ChannelID: ch, Title: "x", SourceMessageID: "nope"},
		"bad priority":       {ChannelID: ch, Title: "x", Priority: "urgent"},
	}
	for name, in := range cases {
		if _, err := e.uc.Create(context.Background(), asAlice, in); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if len(e.events.got()) != 0 {
		t.Fatalf("events = %v", e.events.got())
	}
}

func TestCreateNeedsAReadableChannel(t *testing.T) {
	e := newEnv(t)
	// carol is a member but cannot read ch; the channel looks like it does not exist.
	if _, err := e.uc.Create(context.Background(), asCarol, usecase.CreateInput{ChannelID: ch, Title: "x"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// An error from chat is not "not found": it must not be hidden.
	e.channels.err = errors.New("db down")
	if _, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"}); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
}

func TestCreateValidatesTheAssignee(t *testing.T) {
	e := newEnv(t)
	create := func(assignee string) error {
		_, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{ChannelID: ch, Title: "x", AssigneeID: assignee})
		return err
	}
	if err := create(bob); err != nil {
		t.Fatalf("member who can read: %v", err)
	}
	// carol is active but cannot read the channel.
	if err := create(carol); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("assignee who cannot read: err = %v, want ErrInvalid", err)
	}
	// dave can read but left the workspace.
	e.channels.allow(ch, dave)
	if err := create(dave); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("inactive assignee: err = %v, want ErrInvalid", err)
	}
}

func TestCreateWithoutMembershipLookupRefusesAnAssignee(t *testing.T) {
	e := newEnv(t)
	e.members = nil
	e.build()
	if _, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{ChannelID: ch, Title: "x", AssigneeID: bob}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if _, err := e.uc.Create(context.Background(), asAlice, usecase.CreateInput{ChannelID: ch, Title: "x"}); err != nil {
		t.Fatalf("a task without an assignee needs no membership lookup: %v", err)
	}
}
