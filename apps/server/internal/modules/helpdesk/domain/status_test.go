package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

const (
	assignee  = "0193f0f0-0000-7000-8000-0000000000b5"
	agentTwo  = "0193f0f0-0000-7000-8000-0000000000b6"
	reqUser   = "0193f0f0-0000-7000-8000-0000000000b7"
	adminUser = "0193f0f0-0000-7000-8000-0000000000b8"
)

var (
	requester   = domain.Actor{UserID: reqUser}
	theAssignee = domain.Actor{UserID: assignee, Agent: true}
	otherAgent  = domain.Actor{UserID: agentTwo, Agent: true}
	admin       = domain.Actor{UserID: adminUser, Admin: true}
	outsider    = domain.Actor{UserID: "0193f0f0-0000-7000-8000-0000000000ff"}
)

// ticketAt returns a ticket in status from, assigned to assignee and opened by
// reqUser, with resolved_at set when from is resolved.
func ticketAt(from domain.Status, resolvedAt time.Time) domain.Ticket {
	t := domain.Ticket{
		ID: ticketID, QueueID: qID, RequesterID: reqUser, AssigneeID: assignee,
		Status: from, Priority: domain.PriorityNormal, CreatedAt: t0, UpdatedAt: t0,
	}
	if from == domain.StatusResolved {
		at := resolvedAt
		t.ResolvedAt = &at
	}
	return t
}

func checkErr(t *testing.T, got error, want error) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("err = %v, want none", got)
	case want != nil && !errors.Is(got, want):
		t.Fatalf("err = %v, want %v", got, want)
	}
}

// TestStatusTransitions covers the table of HD-05 for the status endpoint. Each
// allowed move is checked for the right people; every other move is a 409
// whoever asks, and a right move asked by the wrong person is a 403.
func TestStatusTransitions(t *testing.T) {
	now := t0.Add(time.Hour)
	cases := []struct {
		name   string
		from   domain.Status
		to     domain.Status
		actor  domain.Actor
		resolv time.Time
		want   error
	}{
		{"assignee starts work", domain.StatusOpen, domain.StatusInProgress, theAssignee, time.Time{}, nil},
		{"admin starts work", domain.StatusOpen, domain.StatusInProgress, admin, time.Time{}, nil},
		{"requester cannot start work", domain.StatusOpen, domain.StatusInProgress, requester, time.Time{}, app.ErrForbidden},
		{"another agent cannot start work", domain.StatusOpen, domain.StatusInProgress, otherAgent, time.Time{}, app.ErrForbidden},
		{"assignee waits for the requester", domain.StatusInProgress, domain.StatusWaiting, theAssignee, time.Time{}, nil},
		{"admin puts it on hold", domain.StatusInProgress, domain.StatusWaiting, admin, time.Time{}, nil},
		{"another agent cannot put it on hold", domain.StatusInProgress, domain.StatusWaiting, otherAgent, time.Time{}, app.ErrForbidden},
		{"assignee resumes", domain.StatusWaiting, domain.StatusInProgress, theAssignee, time.Time{}, nil},
		{"requester cannot resume", domain.StatusWaiting, domain.StatusInProgress, requester, time.Time{}, app.ErrForbidden},
		{"assignee resolves", domain.StatusInProgress, domain.StatusResolved, theAssignee, time.Time{}, nil},
		{"admin resolves", domain.StatusInProgress, domain.StatusResolved, admin, time.Time{}, nil},
		{"another agent cannot resolve", domain.StatusInProgress, domain.StatusResolved, otherAgent, time.Time{}, app.ErrForbidden},
		{"requester closes a resolved ticket", domain.StatusResolved, domain.StatusClosed, requester, now, nil},
		{"admin closes a resolved ticket", domain.StatusResolved, domain.StatusClosed, admin, now, nil},
		{"assignee cannot close", domain.StatusResolved, domain.StatusClosed, theAssignee, now, app.ErrForbidden},
		{"outsider cannot close", domain.StatusResolved, domain.StatusClosed, outsider, now, app.ErrForbidden},
		{"requester reopens at once", domain.StatusResolved, domain.StatusOpen, requester, now, nil},
		{"requester reopens on day seven", domain.StatusResolved, domain.StatusOpen, requester, now.Add(-7 * 24 * time.Hour), nil},
		{"requester cannot reopen after seven days", domain.StatusResolved, domain.StatusOpen, requester, now.Add(-8 * 24 * time.Hour), app.ErrConflict},
		{"admin cannot reopen", domain.StatusResolved, domain.StatusOpen, admin, now, app.ErrForbidden},
		{"assignee cannot reopen", domain.StatusResolved, domain.StatusOpen, theAssignee, now, app.ErrForbidden},
		{"new cannot be set by status (assign opens it)", domain.StatusNew, domain.StatusOpen, admin, time.Time{}, app.ErrConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tk := ticketAt(c.from, c.resolv)
			checkErr(t, tk.CheckStatus(c.actor, c.to, now), c.want)
		})
	}
}

// TestEveryOtherStatusMoveIsAConflict walks every pair of statuses that the
// table does not allow and checks each is a 409 for everyone who could ask,
// including the requester and the assignee.
func TestEveryOtherStatusMoveIsAConflict(t *testing.T) {
	allowed := map[[2]domain.Status]bool{
		{domain.StatusOpen, domain.StatusInProgress}:     true,
		{domain.StatusInProgress, domain.StatusWaiting}:  true,
		{domain.StatusWaiting, domain.StatusInProgress}:  true,
		{domain.StatusInProgress, domain.StatusResolved}: true,
		{domain.StatusResolved, domain.StatusClosed}:     true,
		{domain.StatusResolved, domain.StatusOpen}:       true,
	}
	all := []domain.Status{domain.StatusNew, domain.StatusOpen, domain.StatusInProgress, domain.StatusWaiting, domain.StatusResolved, domain.StatusClosed}
	now := t0.Add(time.Hour)
	for _, from := range all {
		for _, to := range all {
			if allowed[[2]domain.Status{from, to}] {
				continue
			}
			for _, who := range []domain.Actor{admin, requester, theAssignee} {
				tk := ticketAt(from, now)
				if err := tk.CheckStatus(who, to, now); !errors.Is(err, app.ErrConflict) {
					t.Errorf("%s -> %s by %s: err = %v, want ErrConflict", from, to, who.UserID, err)
				}
			}
		}
	}
}

func TestAssignRules(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.Status
		assignee   string
		actor      domain.Actor
		assignToID string
		want       error
	}{
		{"admin assigns an agent to a new ticket", domain.StatusNew, "", admin, agentTwo, nil},
		{"admin reassigns an open ticket", domain.StatusOpen, assignee, admin, agentTwo, nil},
		{"admin cannot assign a closed ticket", domain.StatusClosed, assignee, admin, agentTwo, app.ErrConflict},
		{"agent claims an unassigned ticket", domain.StatusNew, "", otherAgent, agentTwo, nil},
		{"agent claims in any open state", domain.StatusWaiting, "", otherAgent, agentTwo, nil},
		{"agent cannot take a ticket another agent holds", domain.StatusOpen, assignee, otherAgent, agentTwo, app.ErrForbidden},
		{"agent cannot assign someone else", domain.StatusNew, "", otherAgent, assignee, app.ErrForbidden},
		{"agent may claim again what is already theirs", domain.StatusOpen, agentTwo, otherAgent, agentTwo, nil},
		{"agent cannot claim a closed ticket", domain.StatusClosed, "", otherAgent, agentTwo, app.ErrConflict},
		{"requester cannot assign", domain.StatusNew, "", requester, reqUser, app.ErrForbidden},
		{"outsider cannot assign", domain.StatusNew, "", outsider, outsider.UserID, app.ErrForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tk := domain.Ticket{ID: ticketID, QueueID: qID, RequesterID: reqUser, AssigneeID: c.assignee, Status: c.status}
			checkErr(t, tk.CheckAssign(c.actor, c.assignToID), c.want)
		})
	}
}

func TestVisibilityIsRequesterAgentsAndAdmins(t *testing.T) {
	tk := ticketAt(domain.StatusNew, time.Time{})
	tk.AssigneeID = ""
	for name, c := range map[string]struct {
		actor domain.Actor
		want  bool
	}{
		"requester":   {requester, true},
		"agent":       {otherAgent, true},
		"admin":       {admin, true},
		"outsider":    {outsider, false},
		"no identity": {domain.Actor{}, false},
	} {
		if got := tk.Visible(c.actor); got != c.want {
			t.Errorf("%s: visible = %v, want %v", name, got, c.want)
		}
	}
}

func TestEditIsForAgentsAndAdminsOnly(t *testing.T) {
	tk := ticketAt(domain.StatusOpen, time.Time{})
	if err := tk.CheckEdit(requester); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("requester: err = %v, want ErrForbidden", err)
	}
	if err := tk.CheckEdit(otherAgent); err != nil {
		t.Errorf("agent: err = %v", err)
	}
	if err := tk.CheckEdit(admin); err != nil {
		t.Errorf("admin: err = %v", err)
	}
}

func TestMoveRecordsTheTimesOfResolveCloseAndReopen(t *testing.T) {
	tk := ticketAt(domain.StatusInProgress, time.Time{})
	resolved := t0.Add(time.Hour)
	tk.Move(domain.StatusResolved, resolved)
	if tk.Status != domain.StatusResolved || tk.ResolvedAt == nil || !tk.ResolvedAt.Equal(resolved) {
		t.Fatalf("after resolve: %+v", tk)
	}
	closed := t0.Add(2 * time.Hour)
	tk.Move(domain.StatusClosed, closed)
	if tk.ClosedAt == nil || !tk.ClosedAt.Equal(closed) {
		t.Fatalf("closed_at = %v", tk.ClosedAt)
	}

	tk2 := ticketAt(domain.StatusResolved, resolved)
	tk2.Move(domain.StatusOpen, t0.Add(3*time.Hour))
	if tk2.ResolvedAt != nil {
		t.Errorf("reopen must clear resolved_at, got %v", tk2.ResolvedAt)
	}
}
