package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

const (
	ws    = "0193f0f0-0000-7000-8000-0000000000a1"
	qID   = "0193f0f0-0000-7000-8000-0000000000d1"
	alice = "0193f0f0-0000-7000-8000-0000000000b1"
)

var t0 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

func TestNewQueueChecksTheNameAndPrefix(t *testing.T) {
	long := strings.Repeat("ก", domain.MaxNameRunes+1)
	cases := []struct {
		name, input, prefix string
		wantErr             bool
	}{
		{"prefix is upper-cased", "IT", "it", false},
		{"two letters is the shortest", "Support", "HR", false},
		{"six letters is the longest", "Support", "ABCDEF", false},
		{"one letter is too short", "Support", "I", true},
		{"seven letters is too long", "Support", "ABCDEFG", true},
		{"digits are not letters", "Support", "IT1", true},
		{"empty prefix", "Support", "", true},
		{"empty name", "   ", "IT", true},
		{"name at the limit", strings.Repeat("ก", domain.MaxNameRunes), "IT", false},
		{"name over the limit", long, "IT", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := domain.NewQueue(domain.QueueInput{Name: c.input, Prefix: c.prefix}, qID, ws, t0)
			if c.wantErr {
				if !errors.Is(err, app.ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q.Prefix != strings.ToUpper(c.prefix) {
				t.Errorf("prefix = %q, want %q", q.Prefix, strings.ToUpper(c.prefix))
			}
			if q.WorkspaceID != ws || q.ID != qID || q.ArchivedAt != nil {
				t.Errorf("queue = %+v", q)
			}
		})
	}
}

func TestApplyRenamesSetsAndClearsTheChannelAndArchives(t *testing.T) {
	channel := "0193f0f0-0000-7000-8000-0000000000c1"
	q, err := domain.NewQueue(domain.QueueInput{Name: "IT", Prefix: "IT", ChannelID: channel}, qID, ws, t0)
	if err != nil {
		t.Fatal(err)
	}
	if q.ChannelID != channel {
		t.Fatalf("channel = %q, want the one given", q.ChannelID)
	}

	name := "Helpdesk IT"
	if err := q.Apply(domain.QueuePatch{Name: &name}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if q.Name != name || q.ChannelID != channel {
		t.Errorf("after rename: name=%q channel=%q", q.Name, q.ChannelID)
	}

	if err := q.Apply(domain.QueuePatch{ChannelSet: true}, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if q.ChannelID != "" {
		t.Errorf("channel = %q after clearing, want empty", q.ChannelID)
	}

	archived := true
	if err := q.Apply(domain.QueuePatch{Archived: &archived}, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if q.ArchivedAt == nil || !q.ArchivedAt.Equal(t0.Add(3*time.Minute)) {
		t.Fatalf("archived_at = %v", q.ArchivedAt)
	}
	restored := false
	if err := q.Apply(domain.QueuePatch{Archived: &restored}, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if q.ArchivedAt != nil {
		t.Errorf("archived_at = %v after restoring", q.ArchivedAt)
	}
}

func TestApplyRejectsAnEmptyName(t *testing.T) {
	q, err := domain.NewQueue(domain.QueueInput{Name: "IT", Prefix: "IT"}, qID, ws, t0)
	if err != nil {
		t.Fatal(err)
	}
	blank := " "
	if err := q.Apply(domain.QueuePatch{Name: &blank}, t0); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if q.Name != "IT" {
		t.Errorf("name changed to %q after a rejected patch", q.Name)
	}
}

func TestIsAgentChecksTheAgentList(t *testing.T) {
	q := domain.Queue{AgentIDs: []string{alice}}
	if !q.IsAgent(alice) {
		t.Error("alice is an agent")
	}
	if q.IsAgent("") || q.IsAgent(qID) {
		t.Error("an empty or other user is not an agent")
	}
}
