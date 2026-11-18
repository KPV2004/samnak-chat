package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// TestTheTicketCommandOpensATicketInTheChannel: "/ticket <title>" takes the
// default category, puts the requester card in the caller's channel and the
// callers as the requester (HD-03).
func TestTheTicketCommandOpensATicketInTheChannel(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	if _, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	def, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Hardware", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	err = e.svc.CreateFromCommand(ctx, e.p["bob"], app.CommandInput{ChannelID: chatChannel, Args: "  Printer jams  "})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.repo.tickets); n != 1 {
		t.Fatalf("tickets = %d, want 1", n)
	}
	var tk domain.Ticket
	for _, v := range e.repo.tickets {
		tk = v
	}
	if tk.Title != "Printer jams" || tk.RequesterID != bob || tk.CategoryID != def.ID || tk.QueueID != q.ID {
		t.Errorf("ticket = %+v, want bob's Printer jams in the default category", tk)
	}
	if tk.SourceChannelID != chatChannel || tk.Key != "IT-0001" {
		t.Errorf("channel = %q key = %q", tk.SourceChannelID, tk.Key)
	}
	if err := e.svc.SyncCard(ctx, ws, tk.ID); err != nil {
		t.Fatal(err)
	}
	if c, ok := e.postedIn(chatChannel); !ok || c.SenderID != bob || c.Kind != usecase.CardKind {
		t.Errorf("requester card = %+v, %v; want one in the channel sent as bob", c, ok)
	}
}

// TestTheTicketCommandNeedsADefaultCategory: without one it is a 400 that
// points at the form, and nothing is created.
func TestTheTicketCommandNeedsADefaultCategory(t *testing.T) {
	e := newEnv(t)
	q := e.createQueue(t, "alice", "IT", "IT")
	if _, err := e.svc.CreateCategory(context.Background(), e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Hardware"}); err != nil {
		t.Fatal(err)
	}
	err := e.svc.CreateFromCommand(context.Background(), e.p["bob"], app.CommandInput{ChannelID: chatChannel, Args: "Printer jams"})
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "no_default_category") {
		t.Errorf("err = %v, want the no_default_category hint", err)
	}
	if len(e.repo.tickets) != 0 {
		t.Errorf("created %d tickets, want none", len(e.repo.tickets))
	}
}

// TestTheTicketCommandChecksItsArguments.
func TestTheTicketCommandChecksItsArguments(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.createQueue(t, "alice", "IT", "IT")
	if _, err := e.svc.CreateCategory(ctx, e.p["alice"], domain.CategoryInput{QueueID: q.ID, Name: "Hardware", IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{"", "   ", strings.Repeat("x", domain.MaxTitleRunes+1)} {
		if err := e.svc.CreateFromCommand(ctx, e.p["bob"], app.CommandInput{ChannelID: chatChannel, Args: args}); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("args %q: err = %v, want ErrInvalid", args, err)
		}
	}
}
