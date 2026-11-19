package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/migrations"
	dbmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// testPool starts one Postgres container for the package, migrates it and
// hands out pooled connections. Tests isolate themselves with their own users
// and workspaces.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	dbOnce.Do(func() {
		ctx := context.Background()
		c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("test"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
		)
		if err != nil {
			dbErr = fmt.Sprintf("cannot start postgres container: %v", err)
			return
		}
		url, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			dbErr = err.Error()
			return
		}
		db, err := sql.Open("pgx", url)
		if err != nil {
			dbErr = err.Error()
			return
		}
		defer func() { _ = db.Close() }()
		if err := dbmigrations.Run(ctx, db, "notify", migrations.FS); err != nil {
			dbErr = err.Error()
			return
		}
		dbURL = url
	})
	if dbErr != "" {
		t.Fatal(dbErr)
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func notification(ws, user, key, kind string, at time.Time) domain.Notification {
	return domain.Notification{
		ID: newID(), WorkspaceID: ws, UserID: user, Kind: kind, Title: "Ada in #general", Body: "@bob hi",
		Link: "/chat/c1", Data: map[string]any{"channel_id": "c1"}, Key: key, CreatedAt: at.UTC(),
	}
}

func TestInsertDeduplicatesOnKey(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user := newID(), newID()
	// Postgres keeps microseconds; Linux clocks have nanoseconds.
	at := time.Now().Truncate(time.Microsecond)

	got, inserted, err := repo.Insert(ctx, notification(ws, user, "chat.message:m1", "chat.mention", at))
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	if got.ID == "" || got.Data["channel_id"] != "c1" || !got.CreatedAt.Equal(at) {
		t.Fatalf("first insert returned %+v", got)
	}

	again := notification(ws, user, "chat.message:m1", "chat.dm", at.Add(time.Second))
	if _, inserted, err = repo.Insert(ctx, again); err != nil || inserted {
		t.Fatalf("second insert: inserted=%v err=%v, want false/nil", inserted, err)
	}

	// A different user may use the same key.
	other := newID()
	if _, inserted, err = repo.Insert(ctx, notification(ws, other, "chat.message:m1", "chat.mention", at)); err != nil || !inserted {
		t.Fatalf("other user insert: inserted=%v err=%v, want true/nil", inserted, err)
	}

	// An empty key is never deduplicated.
	a := notification(ws, user, "", "chat.mention", at)
	b := notification(ws, user, "", "chat.mention", at)
	for _, n := range []domain.Notification{a, b} {
		if _, inserted, err = repo.Insert(ctx, n); err != nil || !inserted {
			t.Fatalf("empty-key insert: inserted=%v err=%v, want true/nil", inserted, err)
		}
	}

	if n, err := repo.UnreadCount(ctx, ws, user); err != nil || n != 3 {
		t.Fatalf("unread = %d %v, want 3", n, err)
	}
}

func TestListPagesNewestFirstAndFiltersUnread(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user := newID(), newID()
	base := time.Now().Add(-time.Hour)

	var ids []string
	for i := range 5 {
		n := notification(ws, user, fmt.Sprintf("chat.message:m%d", i), "chat.mention", base.Add(time.Duration(i)*time.Second))
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	page, err := repo.List(ctx, ws, user, false, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != ids[4] || page[1].ID != ids[3] {
		t.Fatalf("first page = %v, want [%s %s]", listIDs(page), ids[4], ids[3])
	}

	next, err := repo.List(ctx, ws, user, false, page[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 2 || next[0].ID != ids[2] || next[1].ID != ids[1] {
		t.Fatalf("second page = %v, want [%s %s]", listIDs(next), ids[2], ids[1])
	}

	if _, err := repo.MarkRead(ctx, ws, user, ids[4], time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkRead(ctx, ws, user, ids[2], time.Now()); err != nil {
		t.Fatal(err)
	}
	unread, err := repo.List(ctx, ws, user, true, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 3 {
		t.Fatalf("unread page = %v, want 3 items", listIDs(unread))
	}

	// Another workspace sees nothing, even with the same user.
	if other, err := repo.List(ctx, newID(), user, false, "", 10); err != nil || len(other) != 0 {
		t.Fatalf("other workspace list = %v %v, want empty", listIDs(other), err)
	}

	// A cursor of another user is not found.
	if _, err := repo.List(ctx, ws, newID(), false, ids[0], 10); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("foreign cursor error = %v, want ErrNotFound", err)
	}
}

func TestMarkReadIsIdempotentAndScoped(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user, other := newID(), newID(), newID()
	n := notification(ws, user, "chat.message:m1", "chat.mention", time.Now())
	if _, _, err := repo.Insert(ctx, n); err != nil {
		t.Fatal(err)
	}

	changed, err := repo.MarkRead(ctx, ws, user, n.ID, time.Now())
	if err != nil || !changed {
		t.Fatalf("first mark: changed=%v err=%v, want true/nil", changed, err)
	}
	changed, err = repo.MarkRead(ctx, ws, user, n.ID, time.Now())
	if err != nil || changed {
		t.Fatalf("second mark: changed=%v err=%v, want false/nil", changed, err)
	}

	for _, tt := range []struct {
		name string
		ws   string
		user string
	}{
		{"another user", ws, other},
		{"another workspace", newID(), user},
		{"unknown id", ws, user},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := n.ID
			if tt.name == "unknown id" {
				id = newID()
			}
			if _, err := repo.MarkRead(ctx, tt.ws, tt.user, id, time.Now()); !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
	}

	if count, err := repo.UnreadCount(ctx, ws, user); err != nil || count != 0 {
		t.Fatalf("unread = %d %v, want 0", count, err)
	}
}

func TestMarkAllRead(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user, other := newID(), newID(), newID()
	var ids []string
	for i := range 3 {
		n := notification(ws, user, fmt.Sprintf("chat.message:m%d", i), "chat.mention", time.Now().Add(time.Duration(i)*time.Millisecond))
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	foreign := notification(ws, other, "chat.message:x1", "chat.mention", time.Now())
	if _, _, err := repo.Insert(ctx, foreign); err != nil {
		t.Fatal(err)
	}

	got, err := repo.MarkAllRead(ctx, ws, user, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !sameSet(got, ids) {
		t.Fatalf("marked = %v, want %v", got, ids)
	}
	if again, err := repo.MarkAllRead(ctx, ws, user, time.Now()); err != nil || len(again) != 0 {
		t.Fatalf("second read-all = %v %v, want none", again, err)
	}
	if count, err := repo.UnreadCount(ctx, ws, other); err != nil || count != 1 {
		t.Fatalf("other unread = %d %v, want 1", count, err)
	}
}

func TestPruneDeletesOnlyOldReadRows(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user := newID(), newID()
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)

	oldRead := notification(ws, user, "chat.message:old-read", "chat.mention", old)
	oldUnread := notification(ws, user, "chat.message:old-unread", "chat.mention", old)
	recentRead := notification(ws, user, "chat.message:recent-read", "chat.mention", now)
	for _, n := range []domain.Notification{oldRead, oldUnread, recentRead} {
		if _, _, err := repo.Insert(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []domain.Notification{oldRead, recentRead} {
		if _, err := repo.MarkRead(ctx, ws, user, n.ID, now); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := repo.Prune(ctx, now.Add(-90*24*time.Hour))
	if err != nil || deleted != 1 {
		t.Fatalf("prune = %d %v, want 1", deleted, err)
	}
	left, err := repo.List(ctx, ws, user, false, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !sameSet(listIDs(left), []string{oldUnread.ID, recentRead.ID}) {
		t.Fatalf("left = %v, want the unread old row and the recent one", listIDs(left))
	}
}

func listIDs(ns []domain.Notification) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[string]bool{}
	for _, s := range got {
		set[s] = true
	}
	for _, s := range want {
		if !set[s] {
			return false
		}
	}
	return true
}

func TestPreferencesRoundTrip(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, user := newID(), newID()
	chanA, chanB := newID(), newID()

	got, err := repo.Preferences(ctx, ws, []string{user})
	if err != nil || len(got) != 0 {
		t.Fatalf("no row yet: %v %v, want an empty map", got, err)
	}

	saved := domain.Preferences{
		UserID: user, WorkspaceID: ws, DisabledKinds: []string{"chat.dm", "chat.thread_reply"},
		MutedChannelIDs: []string{chanA, chanB}, DNDStart: "09:05", DNDEnd: "17:00",
		UpdatedAt: time.Now().Truncate(time.Microsecond),
	}
	if err := repo.SavePreferences(ctx, saved); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Preferences(ctx, ws, []string{user})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := got[user]
	if !ok {
		t.Fatalf("saved row not found: %+v", got)
	}
	if !sameSet(p.DisabledKinds, saved.DisabledKinds) || !sameSet(p.MutedChannelIDs, saved.MutedChannelIDs) {
		t.Fatalf("lists = %v / %v, want %v / %v", p.DisabledKinds, p.MutedChannelIDs, saved.DisabledKinds, saved.MutedChannelIDs)
	}
	if p.DNDStart != "09:05" || p.DNDEnd != "17:00" {
		t.Fatalf("window = %q-%q, want 09:05-17:00", p.DNDStart, p.DNDEnd)
	}
	if !p.UpdatedAt.Equal(saved.UpdatedAt) || p.WorkspaceID != ws || p.UserID != user {
		t.Fatalf("row = %+v", p)
	}

	// A second save replaces the whole row: no window, empty lists.
	cleared := domain.Preferences{UserID: user, WorkspaceID: ws, DisabledKinds: []string{}, MutedChannelIDs: []string{}, UpdatedAt: time.Now()}
	if err := repo.SavePreferences(ctx, cleared); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Preferences(ctx, ws, []string{user})
	if err != nil {
		t.Fatal(err)
	}
	p = got[user]
	if len(p.DisabledKinds) != 0 || len(p.MutedChannelIDs) != 0 || p.DNDStart != "" || p.DNDEnd != "" {
		t.Fatalf("second save left values behind: %+v", p)
	}
}

func TestPreferencesAreScopedToOneWorkspaceAndUser(t *testing.T) {
	repo := postgres.New(testPool(t))
	ctx := context.Background()
	ws, otherWS := newID(), newID()
	alice, bob := newID(), newID()
	at := time.Now().Truncate(time.Microsecond)

	if err := repo.SavePreferences(ctx, domain.Preferences{UserID: alice, WorkspaceID: ws, DisabledKinds: []string{"chat.dm"}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Preferences(ctx, otherWS, []string{alice}); err != nil || len(got) != 0 {
		t.Fatalf("alice in another workspace = %v %v, want none", got, err)
	}
	got, err := repo.Preferences(ctx, ws, []string{alice, bob})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[bob]; ok || len(got) != 1 {
		t.Fatalf("bulk read = %+v, want only alice", got)
	}
	if !sameSet(got[alice].DisabledKinds, []string{"chat.dm"}) {
		t.Fatalf("alice kinds = %v", got[alice].DisabledKinds)
	}
	if got, err := repo.Preferences(ctx, ws, nil); err != nil || len(got) != 0 {
		t.Fatalf("no users = %v %v, want an empty map", got, err)
	}
}
