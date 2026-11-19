package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

const (
	chanA = "0193f0f0-0000-7000-8000-0000000000c1"
	chanB = "0193f0f0-0000-7000-8000-0000000000c2"
	ws2   = "0193f0f0-0000-7000-8000-0000000000a2"
)

// bangkokNight is 23:30 in Asia/Bangkok (UTC+7), inside a 22:00-07:00 window.
var bangkokNight = time.Date(2026, 11, 16, 16, 30, 0, 0, time.UTC)

// nightWindow is a do-not-disturb window that wraps past midnight.
func nightWindow(p domain.Preferences) domain.Preferences {
	p.DNDStart, p.DNDEnd = "22:00", "07:00"
	return p
}

// noticeIn is a rule result about one channel.
func noticeIn(user, kind, channel, key string) app.Notification {
	return app.Notification{
		WorkspaceID: ws, UserID: user, Kind: kind, Title: "Ada in #general", Body: "hi",
		Link: "/chat/" + channel, ChannelID: channel, Data: map[string]any{"channel_id": channel}, Key: key,
	}
}

func TestGetPreferencesDefaultsWhenNoneSaved(t *testing.T) {
	svc, _, _, _, _ := newService(t)

	got, err := svc.GetPreferences(context.Background(), principal(ws, u1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DisabledKinds) != 0 || len(got.MutedChannelIDs) != 0 || got.DNDStart != "" || got.DNDEnd != "" {
		t.Fatalf("defaults = %+v, want nothing filtered", got)
	}
	if got.UserID != u1 || got.WorkspaceID != ws {
		t.Fatalf("defaults owner = %s/%s", got.WorkspaceID, got.UserID)
	}
}

func TestSetPreferencesStoresTheCallersRowOnly(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()

	// The body's owner fields are ignored: the Principal decides the row.
	in := domain.Preferences{
		UserID: u2, WorkspaceID: ws2, DisabledKinds: []string{"chat.dm"},
		MutedChannelIDs: []string{chanB, chanA},
	}
	saved, err := svc.SetPreferences(ctx, principal(ws, u1), in)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UserID != u1 || saved.WorkspaceID != ws {
		t.Fatalf("saved owner = %s/%s, want %s/%s", saved.WorkspaceID, saved.UserID, ws, u1)
	}
	if want := []string{chanA, chanB}; len(saved.MutedChannelIDs) != 2 || saved.MutedChannelIDs[0] != want[0] {
		t.Fatalf("channels = %v, want sorted %v", saved.MutedChannelIDs, want)
	}
	if !saved.UpdatedAt.Equal(fixedNow()()) {
		t.Fatalf("updated_at = %v, want the service clock", saved.UpdatedAt)
	}
	if _, ok := repo.prefs[[2]string{ws2, u2}]; ok {
		t.Fatal("a body that named another user or workspace wrote their row")
	}

	got, err := svc.GetPreferences(ctx, principal(ws, u1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DisabledKinds) != 1 || got.DisabledKinds[0] != "chat.dm" || len(got.MutedChannelIDs) != 2 {
		t.Fatalf("get after set = %+v", got)
	}
	// Another user in the same workspace still has the defaults.
	other, err := svc.GetPreferences(ctx, principal(ws, u2))
	if err != nil {
		t.Fatal(err)
	}
	if len(other.DisabledKinds) != 0 || len(other.MutedChannelIDs) != 0 {
		t.Fatalf("bob's settings changed with alice's: %+v", other)
	}
}

func TestSetPreferencesReplacesEverything(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	ctx := context.Background()
	p := principal(ws, u1)

	if _, err := svc.SetPreferences(ctx, p, nightWindow(domain.Preferences{DisabledKinds: []string{"chat.dm"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPreferences(ctx, p, domain.Preferences{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetPreferences(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DisabledKinds) != 0 || got.DNDStart != "" || got.DNDEnd != "" {
		t.Fatalf("a second PUT left old values behind: %+v", got)
	}
}

func TestSetPreferencesRejectsInvalidAndStoresNothing(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	ctx := context.Background()

	bad := []domain.Preferences{
		{DisabledKinds: []string{"nope"}},
		{MutedChannelIDs: []string{"general"}},
		{DNDStart: "22:00"},
	}
	for _, in := range bad {
		if _, err := svc.SetPreferences(ctx, principal(ws, u1), in); !errors.Is(err, app.ErrInvalid) {
			t.Fatalf("SetPreferences(%+v) = %v, want app.ErrInvalid", in, err)
		}
	}
	if len(repo.prefs) != 0 {
		t.Fatalf("invalid input stored %d rows", len(repo.prefs))
	}
}

func TestGetPreferencesReadErrorIsReturned(t *testing.T) {
	svc, repo, _, _, _ := newService(t)
	repo.prefsErr = errors.New("database down")

	if _, err := svc.GetPreferences(context.Background(), principal(ws, u1)); err == nil {
		t.Fatal("GetPreferences = nil, want the read error")
	}
}

func TestDispatchDropsDisabledKindsForThatUserOnly(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), domain.Preferences{DisabledKinds: []string{"chat.dm"}}); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(
		noticeIn(u1, "chat.dm", chanA, "chat.message:m1"),
		noticeIn(u2, "chat.dm", chanA, "chat.message:m1"),
		noticeIn(u1, "chat.mention", chanA, "chat.message:m2"),
	))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	rows := repo.all()
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2 (alice's dm dropped): %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.UserID == u1 && r.Kind == "chat.dm" {
			t.Fatal("a disabled kind was stored")
		}
	}
	if n := len(ev.createdCalls()); n != 2 {
		t.Fatalf("delivered %d frames, want 2", n)
	}
}

func TestDispatchDropsMutedChannelsForThatUserOnly(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), domain.Preferences{MutedChannelIDs: []string{chanA}}); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(
		noticeIn(u1, "chat.mention", chanA, "chat.message:m1"), // alice muted chanA
		noticeIn(u2, "chat.mention", chanA, "chat.message:m1"), // bob did not
		noticeIn(u1, "chat.mention", chanB, "chat.message:m2"), // another channel
	))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	var alice, bob int
	for _, r := range repo.all() {
		switch {
		case r.UserID == u1 && r.Link == "/chat/"+chanA:
			t.Fatal("a muted channel was stored for alice")
		case r.UserID == u1:
			alice++
		case r.UserID == u2:
			bob++
		}
	}
	if alice != 1 || bob != 1 {
		t.Fatalf("stored alice=%d bob=%d, want 1 and 1", alice, bob)
	}
	calls := ev.createdCalls()
	if len(calls) != 2 {
		t.Fatalf("delivered %d frames, want 2", len(calls))
	}
	for _, c := range calls {
		if c.Unread != 1 {
			t.Fatalf("unread count %d, want 1: a dropped notice must not be counted", c.Unread)
		}
	}
}

func TestDispatchNoticeWithoutChannelIsNeverMuted(t *testing.T) {
	svc, repo, _, _, rules := newService(t)
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), domain.Preferences{MutedChannelIDs: []string{chanA}}); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(mention(u1, "approval.approved", "approval:r1")))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1", n)
	}
}

func TestDispatchInsideTheWindowStoresCountsAndMarksQuiet(t *testing.T) {
	dir := &fakeDirectory{zones: map[string]string{u1: "Asia/Bangkok"}}
	svc, repo, _, ev, rules := newServiceWith(t, dir, func() time.Time { return bangkokNight })
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), nightWindow(domain.Preferences{})); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(noticeIn(u1, "chat.mention", chanA, "chat.message:m1")))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1: quiet notices are stored", n)
	}
	calls := ev.createdCalls()
	if len(calls) != 1 {
		t.Fatalf("delivered %d frames, want 1", len(calls))
	}
	if !calls[0].Quiet {
		t.Fatal("frame is not quiet inside the window")
	}
	if calls[0].Unread != 1 {
		t.Fatalf("unread = %d, want 1: quiet notices are counted", calls[0].Unread)
	}
}

func TestDispatchOutsideTheWindowIsNotQuiet(t *testing.T) {
	dir := &fakeDirectory{zones: map[string]string{u1: "Asia/Bangkok"}}
	// 09:30 UTC is 16:30 in Bangkok, outside 22:00-07:00.
	svc, _, _, ev, rules := newServiceWith(t, dir, fixedNow())
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), nightWindow(domain.Preferences{})); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(noticeIn(u1, "chat.mention", chanA, "chat.message:m1")))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	calls := ev.createdCalls()
	if len(calls) != 1 || calls[0].Quiet {
		t.Fatalf("frames = %+v, want one that is not quiet", calls)
	}
}

func TestDispatchWindowIsReadInEachUsersZone(t *testing.T) {
	// The same instant is 23:30 for alice in Bangkok (inside the window) and
	// 16:30 for bob in UTC, the default for a user with no time zone.
	dir := &fakeDirectory{zones: map[string]string{u1: "Asia/Bangkok"}}
	svc, repo, _, ev, rules := newServiceWith(t, dir, func() time.Time { return bangkokNight })
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), nightWindow(domain.Preferences{})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPreferences(ctx, principal(ws, u2), nightWindow(domain.Preferences{})); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(
		noticeIn(u1, "chat.mention", chanA, "chat.message:m1"),
		noticeIn(u2, "chat.mention", chanA, "chat.message:m1"),
	))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatal(err)
	}
	if n := len(repo.all()); n != 2 {
		t.Fatalf("stored %d rows, want 2", n)
	}
	quiet := map[string]bool{}
	for _, c := range ev.createdCalls() {
		quiet[c.N.UserID] = c.Quiet
	}
	if !quiet[u1] || quiet[u2] {
		t.Fatalf("quiet = %v, want alice quiet and bob not", quiet)
	}
}

func TestDispatchWindowFallsBackToUTCWhenTheDirectoryFails(t *testing.T) {
	dir := &fakeDirectory{err: errors.New("auth is down")}
	svc, repo, _, ev, rules := newServiceWith(t, dir, func() time.Time { return bangkokNight })
	ctx := context.Background()
	if _, err := svc.SetPreferences(ctx, principal(ws, u1), nightWindow(domain.Preferences{})); err != nil {
		t.Fatal(err)
	}
	rules.On("test.thing", ruleOf(noticeIn(u1, "chat.mention", chanA, "chat.message:m1")))

	if err := svc.Dispatch(ctx, testEvent{Name: "test.thing"}); err != nil {
		t.Fatalf("Dispatch = %v, want the notice still stored", err)
	}
	if n := len(repo.all()); n != 1 {
		t.Fatalf("stored %d rows, want 1", n)
	}
	// 16:30 UTC is outside 22:00-07:00, so the fallback is not quiet.
	if calls := ev.createdCalls(); len(calls) != 1 || calls[0].Quiet {
		t.Fatalf("frames = %+v, want one that is not quiet", calls)
	}
}

func TestDispatchPreferencesFailureStoresNothing(t *testing.T) {
	svc, repo, _, ev, rules := newService(t)
	repo.prefsErr = errors.New("database down")
	rules.On("test.thing", ruleOf(noticeIn(u1, "chat.mention", chanA, "chat.message:m1")))

	if err := svc.Dispatch(context.Background(), testEvent{Name: "test.thing"}); err == nil {
		t.Fatal("Dispatch = nil, want the preferences error")
	}
	if len(repo.all()) != 0 || len(ev.createdCalls()) != 0 {
		t.Fatal("notices were stored without knowing the user's preferences")
	}
}
