package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func TestNormalizeChannelName(t *testing.T) {
	got, err := domain.NormalizeChannelName("  general  ")
	if err != nil || got != "general" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := domain.NormalizeChannelName("ห้องทั่วไป"); err != nil {
		t.Errorf("Thai name rejected: %v", err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", 81), "line\nbreak", "nul\x00"} {
		if _, err := domain.NormalizeChannelName(bad); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeChannelName(%q) = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestNormalizeTopic(t *testing.T) {
	if got, err := domain.NormalizeTopic("  hello "); err != nil || got != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := domain.NormalizeTopic(strings.Repeat("a", 251)); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("long topic = %v, want ErrInvalid", err)
	}
	if _, err := domain.NormalizeTopic("a\x00b"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("NUL in topic = %v, want ErrInvalid", err)
	}
}

func TestParseIDs(t *testing.T) {
	a := newID()
	got, err := domain.ParseIDs([]string{strings.ToUpper(a), a})
	if err != nil || len(got) != 1 || got[0] != a {
		t.Fatalf("got %v, %v; want one canonical ID", got, err)
	}
	if _, err := domain.ParseIDs([]string{"nope"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("bad id = %v, want ErrInvalid", err)
	}
}

func TestDirectKey(t *testing.T) {
	a, b, c := newID(), newID(), newID()
	k1, members, err := domain.DirectKey(a, []string{b, c})
	if err != nil {
		t.Fatal(err)
	}
	k2, _, err := domain.DirectKey(c, []string{a, b, b})
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("same set gave different keys: %q vs %q", k1, k2)
	}
	if len(members) != 3 {
		t.Errorf("members = %v, want 3", members)
	}
	k3, _, _ := domain.DirectKey(a, []string{b})
	if k3 == k1 {
		t.Error("different sets gave the same key")
	}
	// A DM with yourself, or with nobody, is not allowed.
	for _, others := range [][]string{nil, {a}} {
		if _, _, err := domain.DirectKey(a, others); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("DirectKey(self, %v) = %v, want ErrInvalid", others, err)
		}
	}
	nine := make([]string, 9)
	for i := range nine {
		nine[i] = newID()
	}
	if _, _, err := domain.DirectKey(a, nine); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("9 others = %v, want ErrInvalid", err)
	}
	if _, _, err := domain.DirectKey(a, nine[:8]); err != nil {
		t.Errorf("8 others = %v, want nil", err)
	}
}

func TestVisible(t *testing.T) {
	pub := domain.Channel{Type: domain.TypePublic}
	priv := domain.Channel{Type: domain.TypePrivate}
	dm := domain.Channel{Type: domain.TypeDirect}
	tests := []struct {
		name   string
		ch     domain.Channel
		role   app.Role
		member bool
		want   bool
	}{
		{"member sees public", pub, app.RoleMember, false, true},
		{"guest outside public", pub, app.RoleGuest, false, false},
		{"guest added to public", pub, app.RoleGuest, true, true},
		{"admin outside private", priv, app.RoleAdmin, false, false},
		{"owner outside dm", dm, app.RoleOwner, false, false},
		{"member of private", priv, app.RoleMember, true, true},
		{"member of dm", dm, app.RoleGuest, true, true},
	}
	for _, tt := range tests {
		if got := domain.Visible(tt.ch, tt.role, tt.member); got != tt.want {
			t.Errorf("%s: Visible = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestValidateBody(t *testing.T) {
	for _, ok := range []string{"hi", "สวัสดี", strings.Repeat("ก", 4000), strings.Repeat("a", 4000)} {
		if err := domain.ValidateBody(ok); err != nil {
			t.Errorf("ValidateBody(len %d) = %v", len(ok), err)
		}
	}
	for _, bad := range []string{"", "   \n\t", strings.Repeat("ก", 4001), "a\x00b"} {
		if err := domain.ValidateBody(bad); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("ValidateBody(%q) = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestParseMentions(t *testing.T) {
	a, b := newID(), newID()
	ids, all := domain.ParseMentions("hi <@" + a + "> and <@" + b + "> and <@" + a + ">, ping <!channel>")
	slices.Sort(ids)
	want := []string{a, b}
	slices.Sort(want)
	if !slices.Equal(ids, want) || !all {
		t.Errorf("got %v %v, want %v true", ids, all, want)
	}
	ids, all = domain.ParseMentions("<@not-a-uuid> <@ " + a + "> @channel <!everyone> `x`")
	if len(ids) != 0 || all {
		t.Errorf("malformed tokens parsed: %v %v", ids, all)
	}
	ids, _ = domain.ParseMentions("<@" + strings.ToUpper(a) + ">")
	if len(ids) != 1 || ids[0] != a {
		t.Errorf("uppercase UUID: got %v, want canonical %s", ids, a)
	}
}

func TestMessageRules(t *testing.T) {
	now := time.Now()
	author, other := newID(), newID()
	m := domain.Message{SenderID: author, Kind: domain.KindText}

	if err := m.CanEdit(author); err != nil {
		t.Errorf("author edit = %v", err)
	}
	if err := m.CanEdit(other); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("other edit = %v, want ErrForbidden", err)
	}
	card := domain.Message{SenderID: author, Kind: "approval.request"}
	if err := card.CanEdit(author); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("card edit = %v, want ErrInvalid", err)
	}
	gone := domain.Message{SenderID: author, Kind: domain.KindText, DeletedAt: &now}
	if err := gone.CanEdit(author); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("deleted edit = %v, want ErrNotFound", err)
	}

	if err := m.CanDelete(author, false); err != nil {
		t.Errorf("author delete = %v", err)
	}
	if err := m.CanDelete(other, false); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("other delete = %v, want ErrForbidden", err)
	}
	if err := m.CanDelete(other, true); err != nil {
		t.Errorf("moderator delete = %v", err)
	}
	if err := gone.CanDelete(author, false); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("double delete = %v, want ErrNotFound", err)
	}
}
