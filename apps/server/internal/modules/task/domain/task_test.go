package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

const (
	alice = "0193f0f0-0000-7000-8000-0000000000b1"
	bob   = "0193f0f0-0000-7000-8000-0000000000b2"
	carol = "0193f0f0-0000-7000-8000-0000000000b3"
	ws    = "0193f0f0-0000-7000-8000-0000000000a1"
	ch    = "0193f0f0-0000-7000-8000-0000000000c1"
)

var t0 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

func newTask(t *testing.T, in domain.NewInput) domain.Task {
	t.Helper()
	if in.WorkspaceID == "" {
		in.WorkspaceID = ws
	}
	if in.ChannelID == "" {
		in.ChannelID = ch
	}
	if in.CreatorID == "" {
		in.CreatorID = alice
	}
	if in.Title == "" {
		in.Title = "Fix the login page"
	}
	task, err := domain.New(in, "id-1", t0)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestNewAppliesDefaultsAndTrims(t *testing.T) {
	task := newTask(t, domain.NewInput{Title: "  Fix it  "})
	if task.Title != "Fix it" || task.Priority != domain.PriorityNormal || task.Status != domain.StatusTodo {
		t.Fatalf("task = %+v", task)
	}
	if !task.CreatedAt.Equal(t0) || !task.UpdatedAt.Equal(t0) || task.ID != "id-1" {
		t.Fatalf("identity and clock = %+v", task)
	}
}

func TestNewValidates(t *testing.T) {
	cases := map[string]domain.NewInput{
		"empty title":      {Title: "   "},
		"long title":       {Title: strings.Repeat("a", 201)},
		"long description": {Description: strings.Repeat("a", 4001)},
		"bad priority":     {Priority: "urgent"},
	}
	for name, in := range cases {
		in.WorkspaceID, in.ChannelID, in.CreatorID = ws, ch, alice
		if in.Title == "" {
			in.Title = "ok"
		}
		if name == "empty title" {
			in.Title = "   "
		}
		if _, err := domain.New(in, "id", t0); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// 200 Thai runes are 600 bytes: the limit counts characters.
	in := domain.NewInput{WorkspaceID: ws, ChannelID: ch, CreatorID: alice, Title: strings.Repeat("ก", 200)}
	if _, err := domain.New(in, "id", t0); err != nil {
		t.Fatalf("200 runes: %v", err)
	}
}

func TestCanEdit(t *testing.T) {
	task := newTask(t, domain.NewInput{AssigneeID: bob})
	cases := []struct {
		name string
		p    app.Principal
		want bool
	}{
		{"creator", app.Principal{UserID: alice, Role: app.RoleMember}, true},
		{"assignee", app.Principal{UserID: bob, Role: app.RoleMember}, true},
		{"owner", app.Principal{UserID: carol, Role: app.RoleOwner}, true},
		{"admin", app.Principal{UserID: carol, Role: app.RoleAdmin}, true},
		{"manager", app.Principal{UserID: carol, Role: app.RoleManager}, false},
		{"member", app.Principal{UserID: carol, Role: app.RoleMember}, false},
		{"guest", app.Principal{UserID: carol, Role: app.RoleGuest}, false},
	}
	for _, c := range cases {
		if got := task.CanEdit(c.p); got != c.want {
			t.Errorf("%s: CanEdit = %v, want %v", c.name, got, c.want)
		}
	}
	if (domain.Task{CreatorID: alice}).CanEdit(app.Principal{UserID: ""}) {
		t.Error("an empty user must not match an empty assignee")
	}
}

func ptr[T any](v T) *T { return &v }

func TestApplyReportsWhatChanged(t *testing.T) {
	later := t0.Add(time.Hour)
	due := t0.Add(48 * time.Hour)

	t.Run("status", func(t *testing.T) {
		task := newTask(t, domain.NewInput{})
		out, err := task.Apply(domain.Patch{Status: ptr(domain.StatusDoing)}, later)
		if err != nil {
			t.Fatal(err)
		}
		if !out.StatusChanged || out.From != domain.StatusTodo || task.Status != domain.StatusDoing || out.Assigned || out.Updated {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
		if !task.UpdatedAt.Equal(later) {
			t.Fatalf("updated_at = %v", task.UpdatedAt)
		}
	})
	t.Run("same status is no change", func(t *testing.T) {
		task := newTask(t, domain.NewInput{})
		out, err := task.Apply(domain.Patch{Status: ptr(domain.StatusTodo)}, later)
		if err != nil {
			t.Fatal(err)
		}
		if out.Any() || !task.UpdatedAt.Equal(t0) {
			t.Fatalf("out = %+v updated_at = %v", out, task.UpdatedAt)
		}
	})
	t.Run("assign", func(t *testing.T) {
		task := newTask(t, domain.NewInput{})
		out, err := task.Apply(domain.Patch{AssigneeSet: true, AssigneeID: bob}, later)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Assigned || out.Updated || task.AssigneeID != bob {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("clear assignee is a plain update", func(t *testing.T) {
		task := newTask(t, domain.NewInput{AssigneeID: bob})
		out, err := task.Apply(domain.Patch{AssigneeSet: true}, later)
		if err != nil {
			t.Fatal(err)
		}
		if out.Assigned || !out.Updated || task.AssigneeID != "" {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("reassign to the same person changes nothing", func(t *testing.T) {
		task := newTask(t, domain.NewInput{AssigneeID: bob})
		out, _ := task.Apply(domain.Patch{AssigneeSet: true, AssigneeID: bob}, later)
		if out.Any() {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("other fields", func(t *testing.T) {
		task := newTask(t, domain.NewInput{})
		out, err := task.Apply(domain.Patch{Title: ptr(" New "), Description: ptr("d"), Priority: ptr(domain.PriorityHigh)}, later)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Updated || out.Assigned || out.StatusChanged || task.Title != "New" || task.Description != "d" || task.Priority != domain.PriorityHigh {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("changing due_at clears both sent marks", func(t *testing.T) {
		task := newTask(t, domain.NewInput{DueAt: &due})
		task.DueSoonSentAt, task.OverdueSentAt = &t0, &t0
		out, err := task.Apply(domain.Patch{DueSet: true, DueAt: ptr(due.Add(time.Hour))}, later)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Updated || task.DueSoonSentAt != nil || task.OverdueSentAt != nil {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("the same due_at keeps the marks", func(t *testing.T) {
		task := newTask(t, domain.NewInput{DueAt: &due})
		task.DueSoonSentAt = &t0
		out, _ := task.Apply(domain.Patch{DueSet: true, DueAt: ptr(due)}, later)
		if out.Any() || task.DueSoonSentAt == nil {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("clearing due_at", func(t *testing.T) {
		task := newTask(t, domain.NewInput{DueAt: &due})
		out, _ := task.Apply(domain.Patch{DueSet: true}, later)
		if !out.Updated || task.DueAt != nil {
			t.Fatalf("out = %+v task = %+v", out, task)
		}
	})
	t.Run("status and assignee together", func(t *testing.T) {
		task := newTask(t, domain.NewInput{})
		out, _ := task.Apply(domain.Patch{Status: ptr(domain.StatusDone), AssigneeSet: true, AssigneeID: bob}, later)
		if !out.StatusChanged || !out.Assigned {
			t.Fatalf("out = %+v", out)
		}
	})
}

func TestApplyValidatesAndLeavesTheTaskAlone(t *testing.T) {
	bad := []domain.Patch{
		{Title: ptr("  ")},
		{Title: ptr(strings.Repeat("a", 201))},
		{Description: ptr(strings.Repeat("a", 4001))},
		{Priority: ptr(domain.Priority("urgent"))},
		{Status: ptr(domain.Status("blocked"))},
	}
	for i, p := range bad {
		task := newTask(t, domain.NewInput{})
		before := task
		if _, err := task.Apply(p, t0.Add(time.Hour)); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("patch %d: err = %v, want ErrInvalid", i, err)
		}
		if task != before {
			t.Errorf("patch %d changed the task: %+v", i, task)
		}
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct {
		args, title, assignee string
	}{
		{"Fix the login page", "Fix the login page", ""},
		{"<@" + bob + "> Fix the login page", "Fix the login page", bob},
		{"Fix the login page <@" + bob + ">", "Fix the login page", bob},
		{"Fix <@" + bob + "> the page", "Fix the page", bob},
		{"Fix <@" + bob + "> with <@" + carol + ">", "Fix with <@" + carol + ">", bob},
		{"Fix <@" + strings.ToUpper(bob) + ">", "Fix", bob},
		{"Fix <@not-an-id> now", "Fix <@not-an-id> now", ""},
	}
	for _, c := range cases {
		title, assignee, err := domain.ParseCommand(c.args)
		if err != nil || title != c.title || assignee != c.assignee {
			t.Errorf("%q = (%q, %q, %v), want (%q, %q)", c.args, title, assignee, err, c.title, c.assignee)
		}
	}
	for _, args := range []string{"", "   ", "<@" + bob + ">", strings.Repeat("a", 201)} {
		if _, _, err := domain.ParseCommand(args); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%q: err = %v, want ErrInvalid", args, err)
		}
	}
}
