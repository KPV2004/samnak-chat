package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	chanA = "0193f0f0-0000-7000-8000-0000000000c1"
	chanB = "0193f0f0-0000-7000-8000-0000000000c2"
)

func TestNormalizeCanonicalizes(t *testing.T) {
	p, err := Preferences{
		UserID: user, WorkspaceID: ws,
		DisabledKinds:   []string{"chat.thread_reply", "chat.dm", "chat.dm"},
		MutedChannelIDs: []string{chanB, "0193F0F0-0000-7000-8000-0000000000C1", chanB},
		DNDStart:        "22:00", DNDEnd: "07:00",
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"chat.dm", "chat.thread_reply"}; !reflect.DeepEqual(p.DisabledKinds, want) {
		t.Fatalf("kinds = %v, want %v", p.DisabledKinds, want)
	}
	if want := []string{chanA, chanB}; !reflect.DeepEqual(p.MutedChannelIDs, want) {
		t.Fatalf("channels = %v, want %v", p.MutedChannelIDs, want)
	}
	if p.DNDStart != "22:00" || p.DNDEnd != "07:00" {
		t.Fatalf("window = %q-%q", p.DNDStart, p.DNDEnd)
	}
}

func TestNormalizeEmptyListsAreNotNil(t *testing.T) {
	p, err := Preferences{UserID: user, WorkspaceID: ws}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if p.DisabledKinds == nil || p.MutedChannelIDs == nil {
		t.Fatalf("empty lists must be non-nil so they encode as []: %+v", p)
	}
}

func TestNormalizeRejects(t *testing.T) {
	tests := []struct {
		name string
		p    Preferences
	}{
		{"kind without a module", Preferences{DisabledKinds: []string{"mention"}}},
		{"kind with spaces", Preferences{DisabledKinds: []string{"chat. mention"}}},
		{"kind with upper case", Preferences{DisabledKinds: []string{"Chat.Mention"}}},
		{"channel not a uuid", Preferences{MutedChannelIDs: []string{"general"}}},
		{"only a start", Preferences{DNDStart: "22:00"}},
		{"only an end", Preferences{DNDEnd: "07:00"}},
		{"hour 24", Preferences{DNDStart: "24:00", DNDEnd: "07:00"}},
		{"minute 60", Preferences{DNDStart: "22:60", DNDEnd: "07:00"}},
		{"one digit hour", Preferences{DNDStart: "9:00", DNDEnd: "17:00"}},
		{"with seconds", Preferences{DNDStart: "22:00:00", DNDEnd: "07:00"}},
		{"empty window", Preferences{DNDStart: "08:00", DNDEnd: "08:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.p.UserID, tt.p.WorkspaceID = user, ws
			if _, err := tt.p.Normalize(); !errors.Is(err, app.ErrInvalid) {
				t.Fatalf("Normalize = %v, want app.ErrInvalid", err)
			}
		})
	}
}

func TestKindAndChannelChecks(t *testing.T) {
	p := Preferences{DisabledKinds: []string{"chat.dm"}, MutedChannelIDs: []string{chanA}}
	if !p.KindDisabled("chat.dm") || p.KindDisabled("chat.mention") {
		t.Fatal("KindDisabled answered wrongly")
	}
	if !p.ChannelMuted(chanA) || p.ChannelMuted(chanB) {
		t.Fatal("ChannelMuted answered wrongly")
	}
	// A notice without a channel is never muted.
	if p.ChannelMuted("") {
		t.Fatal("a notice with no channel reported as muted")
	}
}

// at is 2026-11-16 in UTC; Asia/Bangkok is UTC+7.
func at(hour, minute int) time.Time {
	return time.Date(2026, 11, 16, hour, minute, 0, 0, time.UTC)
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestInWindowWrapsPastMidnight(t *testing.T) {
	bangkok := mustLoad(t, "Asia/Bangkok")
	p := Preferences{DNDStart: "22:00", DNDEnd: "07:00"}

	tests := []struct {
		name string
		at   time.Time
		loc  *time.Location
		want bool
	}{
		// 22:00 Bangkok is 15:00 UTC; 07:00 Bangkok is 00:00 UTC.
		{"bangkok 21:59 is outside", time.Date(2026, 11, 16, 14, 59, 0, 0, time.UTC), bangkok, false},
		{"bangkok 22:00 starts the window", time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC), bangkok, true},
		{"bangkok 23:30 is inside", time.Date(2026, 11, 16, 16, 30, 0, 0, time.UTC), bangkok, true},
		{"bangkok 00:30 is inside after midnight", time.Date(2026, 11, 16, 17, 30, 0, 0, time.UTC), bangkok, true},
		{"bangkok 06:59 is inside", time.Date(2026, 11, 16, 23, 59, 0, 0, time.UTC), bangkok, true},
		{"bangkok 07:00 ends the window", time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC), bangkok, false},
		{"bangkok 16:30 is outside", time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC), bangkok, false},
		{"utc 16:30 is outside when read as utc", at(16, 30), time.UTC, false},
		{"utc 23:30 is inside when no zone is given", at(23, 30), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.InWindow(tt.at, tt.loc); got != tt.want {
				t.Fatalf("InWindow(%s) = %v, want %v", tt.at.UTC().Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestInWindowSameDay(t *testing.T) {
	p := Preferences{DNDStart: "09:00", DNDEnd: "17:00"}
	if !p.InWindow(at(9, 0), time.UTC) {
		t.Fatal("09:00 is the start and must be inside")
	}
	if p.InWindow(at(8, 59), time.UTC) {
		t.Fatal("08:59 must be outside")
	}
	if p.InWindow(at(17, 0), time.UTC) {
		t.Fatal("17:00 is the end and must be outside")
	}
}

func TestInWindowNeedsBothEnds(t *testing.T) {
	if (Preferences{}).InWindow(at(23, 0), time.UTC) {
		t.Fatal("no window means never quiet")
	}
	if (Preferences{DNDStart: "22:00"}).InWindow(at(23, 0), time.UTC) {
		t.Fatal("a half window means never quiet")
	}
}

func TestDefaultsHaveNoRules(t *testing.T) {
	p := Defaults(ws, user)
	if p.KindDisabled("chat.mention") || p.ChannelMuted(chanA) || p.InWindow(at(23, 0), time.UTC) {
		t.Fatalf("defaults must not filter anything: %+v", p)
	}
	if p.UserID != user || p.WorkspaceID != ws {
		t.Fatalf("defaults owner = %s/%s", p.WorkspaceID, p.UserID)
	}
}
