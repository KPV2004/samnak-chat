package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

const (
	ticketID = "0193f0f0-0000-7000-8000-0000000000f1"
	catA     = "0193f0f0-0000-7000-8000-0000000000e2"
)

func TestNewTicketChecksTitleDescriptionAndPriority(t *testing.T) {
	base := domain.NewTicketInput{WorkspaceID: ws, QueueID: qID, CategoryID: catA, RequesterID: alice, Title: "Printer jams"}
	cases := []struct {
		name    string
		mutate  func(*domain.NewTicketInput)
		wantErr bool
	}{
		{"a plain ticket", func(*domain.NewTicketInput) {}, false},
		{"blank title", func(in *domain.NewTicketInput) { in.Title = "   " }, true},
		{"title at the limit", func(in *domain.NewTicketInput) { in.Title = strings.Repeat("ก", domain.MaxTitleRunes) }, false},
		{"title over the limit", func(in *domain.NewTicketInput) { in.Title = strings.Repeat("ก", domain.MaxTitleRunes+1) }, true},
		{"description at the limit", func(in *domain.NewTicketInput) { in.Description = strings.Repeat("a", domain.MaxDescriptionRunes) }, false},
		{"description over the limit", func(in *domain.NewTicketInput) { in.Description = strings.Repeat("a", domain.MaxDescriptionRunes+1) }, true},
		{"urgent priority", func(in *domain.NewTicketInput) { in.Priority = domain.PriorityUrgent }, false},
		{"unknown priority", func(in *domain.NewTicketInput) { in.Priority = "critical" }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base
			c.mutate(&in)
			got, err := domain.NewTicket(in, ticketID, t0)
			if c.wantErr {
				if !errors.Is(err, app.ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != domain.StatusNew || got.AssigneeID != "" || got.ResolvedAt != nil || got.ClosedAt != nil {
				t.Errorf("a new ticket starts unassigned in status new: %+v", got)
			}
			if in.Priority == "" && got.Priority != domain.PriorityNormal {
				t.Errorf("priority = %q, want normal by default", got.Priority)
			}
			if got.Title != strings.TrimSpace(in.Title) || got.RequesterID != alice || got.QueueID != qID || got.CategoryID != catA {
				t.Errorf("ticket = %+v", got)
			}
			if !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0) {
				t.Errorf("times = %v / %v, want %v", got.CreatedAt, got.UpdatedAt, t0)
			}
		})
	}
}

func TestTicketKeyIsThePrefixAndAFourDigitNumber(t *testing.T) {
	cases := map[string]struct {
		prefix string
		number int64
	}{"IT-0142": {"IT", 142}, "HR-0001": {"HR", 1}, "IT-10000": {"IT", 10000}}
	for want, c := range cases {
		if got := domain.TicketKey(c.prefix, c.number); got != want {
			t.Errorf("TicketKey(%q, %d) = %q, want %q", c.prefix, c.number, got, want)
		}
	}
}
