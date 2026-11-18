package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

func TestNewCommentChecksTheBody(t *testing.T) {
	cases := map[string]struct {
		body    string
		wantErr bool
	}{
		"plain text":         {"Restarted the printer", false},
		"blank":              {"   ", true},
		"at the limit":       {strings.Repeat("a", domain.MaxCommentRunes), false},
		"over the limit":     {strings.Repeat("a", domain.MaxCommentRunes+1), true},
		"trimmed on the way": {"  hello  ", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := domain.NewComment(domain.NewCommentInput{TicketID: ticketID, AuthorID: alice, Body: c.body}, "0193f0f0-0000-7000-8000-0000000000c9", t0)
			if c.wantErr {
				if !errors.Is(err, app.ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Body != strings.TrimSpace(c.body) || got.TicketID != ticketID || got.AuthorID != alice {
				t.Errorf("comment = %+v", got)
			}
		})
	}
}

// TestCommentVisibilityRules: anyone who may see the ticket may post a public
// comment; internal comments are for agents and admins (HD-07).
func TestCommentVisibilityRules(t *testing.T) {
	tk := ticketAt(domain.StatusOpen, t0)
	cases := []struct {
		name     string
		actor    domain.Actor
		internal bool
		want     error
	}{
		{"requester writes publicly", requester, false, nil},
		{"requester cannot write internally", requester, true, app.ErrForbidden},
		{"assignee writes internally", theAssignee, true, nil},
		{"another agent writes internally", otherAgent, true, nil},
		{"admin writes internally", admin, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkErr(t, tk.CheckComment(c.actor, c.internal), c.want)
		})
	}
}
