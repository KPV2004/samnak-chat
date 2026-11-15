package chat_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

func (e *env) identity(u user) app.Identity { return e.auth[u.Token] }

func TestTypingFansOutToOtherMembers(t *testing.T) {
	e := newEnv(t)
	ws := e.newWorkspace()
	alice, bob, carol := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "carol")
	e.member(ws, "dave") // in the workspace, not in the channel
	ch := e.createChannel(alice, ws, "private", "team", bob.ID, carol.ID)
	e.rt.reset()

	if err := e.rt.client("typing", e.identity(alice), fmt.Sprintf(`{"channel_id":%q}`, ch.ID)); err != nil {
		t.Fatal(err)
	}
	got := e.rt.of("typing")
	if len(got) != 1 {
		t.Fatalf("typing deliveries = %d, want 1", len(got))
	}
	users := slices.Clone(got[0].Users)
	slices.Sort(users)
	want := []string{bob.ID, carol.ID}
	slices.Sort(want)
	if !slices.Equal(users, want) {
		t.Errorf("recipients = %v, want %v (not alice, not dave who is outside the channel)", users, want)
	}
	var data struct {
		WorkspaceID string `json:"workspace_id"`
		ChannelID   string `json:"channel_id"`
		UserID      string `json:"user_id"`
	}
	if err := json.Unmarshal(got[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.WorkspaceID != ws || data.ChannelID != ch.ID || data.UserID != alice.ID {
		t.Errorf("data = %s", got[0].Data)
	}

	// At most one per user per channel per 3 seconds; other users and
	// channels have their own window.
	for range 3 {
		if err := e.rt.client("typing", e.identity(alice), fmt.Sprintf(`{"channel_id":%q}`, ch.ID)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(e.rt.of("typing")); n != 1 {
		t.Errorf("typing deliveries after repeats = %d, want 1", n)
	}
	if err := e.rt.client("typing", e.identity(bob), fmt.Sprintf(`{"channel_id":%q}`, ch.ID)); err != nil {
		t.Fatal(err)
	}
	other := e.createChannel(alice, ws, "private", "other", bob.ID)
	if err := e.rt.client("typing", e.identity(alice), fmt.Sprintf(`{"channel_id":%q}`, other.ID)); err != nil {
		t.Fatal(err)
	}
	if n := len(e.rt.of("typing")); n != 3 {
		t.Errorf("typing deliveries with another user and channel = %d, want 3", n)
	}
}

func TestTypingRejectsOutsiders(t *testing.T) {
	e := newEnv(t)
	ws, other := e.newWorkspace(), e.newWorkspace()
	alice, bob, dave := e.member(ws, "alice"), e.member(ws, "bob"), e.member(ws, "dave")
	mallory := e.member(other, "mallory")
	private := e.createChannel(alice, ws, "private", "team", bob.ID)
	public := e.createChannel(alice, ws, "public", "open")
	e.rt.reset()
	typing := func(u user, channelID string) error {
		return e.rt.client("typing", e.identity(u), fmt.Sprintf(`{"channel_id":%q}`, channelID))
	}

	for name, c := range map[string]struct {
		err  error
		want error
	}{
		"not a member of a private channel": {typing(dave, private.ID), app.ErrNotFound},
		"public channel not joined":         {typing(dave, public.ID), app.ErrNotFound},
		"another workspace":                 {typing(mallory, private.ID), app.ErrNotFound},
		"unknown channel":                   {typing(alice, uuid.NewString()), app.ErrNotFound},
		"not a UUID":                        {typing(alice, "nope"), app.ErrInvalid},
		"no channel":                        {e.rt.client("typing", e.identity(alice), `{}`), app.ErrInvalid},
		"not JSON":                          {e.rt.client("typing", e.identity(alice), `[1`), app.ErrInvalid},
	} {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s = %v, want %v", name, c.err, c.want)
		}
	}
	if n := len(e.rt.of("typing")); n != 0 {
		t.Errorf("typing reached %d deliveries from rejected frames", n)
	}

	// Removed from the workspace but not yet from the channel: still refused.
	e.members.remove(ws, bob.ID)
	if err := typing(bob, private.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("removed member = %v, want ErrForbidden", err)
	}
	if n := len(e.rt.of("typing")); n != 0 {
		t.Errorf("deliveries = %d", n)
	}
}

func TestTypingWithoutRealtimeRegistersNothing(t *testing.T) {
	e := newEnvWith(t, nil) // Deps.Realtime is nil: Init must not call Handle
	ws := e.newWorkspace()
	alice := e.member(ws, "alice")
	if ch := e.createChannel(alice, ws, "public", "open"); ch.ID == "" {
		t.Fatal("no channel")
	}
}
