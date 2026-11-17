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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	dbmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// testPool starts one Postgres container for the package, migrates it and
// hands out pooled connections. Tests isolate themselves with their own
// workspaces.
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
		if err := dbmigrations.Run(ctx, db, "task", migrations.FS); err != nil {
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

// at is a time Postgres stores exactly: it keeps microseconds, Linux clocks
// have nanoseconds.
func at(d time.Duration) time.Time { return time.Now().Add(d).UTC().Truncate(time.Microsecond) }

type fixture struct {
	repo           *postgres.Repo
	ws, ch, author string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{repo: postgres.New(testPool(t)), ws: newID(), ch: newID(), author: newID()}
}

func (f fixture) insert(t *testing.T, mod func(*domain.Task)) domain.Task {
	t.Helper()
	now := at(0)
	task := domain.Task{
		ID: newID(), WorkspaceID: f.ws, ChannelID: f.ch, Title: "Fix login", CreatorID: f.author,
		Priority: domain.PriorityNormal, Status: domain.StatusTodo, CreatedAt: now, UpdatedAt: now,
	}
	if mod != nil {
		mod(&task)
	}
	got, err := f.repo.Insert(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ptr[T any](v T) *T { return &v }

func TestInsertAndGetRoundTrip(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	assignee, source := newID(), newID()
	due := at(48 * time.Hour)

	in := f.insert(t, func(k *domain.Task) {
		k.Description, k.AssigneeID, k.SourceMessageID, k.DueAt, k.Priority = "details", assignee, source, &due, domain.PriorityHigh
	})
	got, err := f.repo.Get(ctx, f.ws, in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Fix login" || got.Description != "details" || got.AssigneeID != assignee || got.SourceMessageID != source ||
		got.Priority != domain.PriorityHigh || got.Status != domain.StatusTodo || got.CreatorID != f.author || got.ChannelID != f.ch {
		t.Fatalf("got %+v", got)
	}
	if got.DueAt == nil || !got.DueAt.Equal(due) || got.DueAt.Location() != time.UTC {
		t.Fatalf("due_at = %v, want %v in UTC", got.DueAt, due)
	}
	if got.MessageID != "" || got.DueSoonSentAt != nil || got.OverdueSentAt != nil {
		t.Fatalf("fresh task has card or marks: %+v", got)
	}

	// No assignee and no due date round-trip as empty.
	bare := f.insert(t, nil)
	if got, _ = f.repo.Get(ctx, f.ws, bare.ID); got.AssigneeID != "" || got.DueAt != nil || got.SourceMessageID != "" {
		t.Fatalf("bare task = %+v", got)
	}
}

func TestGetIsScopedToTheWorkspace(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task := f.insert(t, nil)

	if _, err := f.repo.Get(ctx, newID(), task.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("other workspace: err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Get(ctx, f.ws, newID()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown id: err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Get(ctx, f.ws, "not-a-uuid"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("bad id: err = %v, want ErrNotFound", err)
	}
	other := domain.Task{ID: task.ID, WorkspaceID: newID(), Title: "x", Priority: domain.PriorityLow, Status: domain.StatusDone, UpdatedAt: at(0)}
	if _, err := f.repo.Update(ctx, other); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("update from another workspace: err = %v, want ErrNotFound", err)
	}
}

func TestUpdateWritesEveryEditableField(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task := f.insert(t, func(k *domain.Task) { k.AssigneeID, k.DueAt = newID(), ptr(at(time.Hour)) })

	bob := newID()
	due := at(72 * time.Hour)
	mark := at(0)
	task.Title, task.Description, task.AssigneeID, task.DueAt = "New", "more", bob, &due
	task.Priority, task.Status = domain.PriorityLow, domain.StatusDoing
	task.DueSoonSentAt, task.OverdueSentAt, task.UpdatedAt = &mark, &mark, at(time.Minute)
	got, err := f.repo.Update(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New" || got.Description != "more" || got.AssigneeID != bob || !got.DueAt.Equal(due) ||
		got.Priority != domain.PriorityLow || got.Status != domain.StatusDoing || !got.UpdatedAt.Equal(task.UpdatedAt) ||
		got.DueSoonSentAt == nil || !got.DueSoonSentAt.Equal(mark) || got.OverdueSentAt == nil {
		t.Fatalf("got %+v", got)
	}
	if got.CreatorID != f.author || !got.CreatedAt.Equal(task.CreatedAt) {
		t.Fatalf("update changed what is not editable: %+v", got)
	}

	// Clearing the assignee and the due date stores NULL.
	task.AssigneeID, task.DueAt, task.DueSoonSentAt, task.OverdueSentAt = "", nil, nil, nil
	if got, err = f.repo.Update(ctx, task); err != nil {
		t.Fatal(err)
	}
	if got.AssigneeID != "" || got.DueAt != nil || got.DueSoonSentAt != nil || got.OverdueSentAt != nil {
		t.Fatalf("cleared task = %+v", got)
	}
}

func TestSetMessageOnlyOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task := f.insert(t, nil)
	first, second := newID(), newID()

	if ok, err := f.repo.SetMessage(ctx, f.ws, task.ID, first); err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if ok, err := f.repo.SetMessage(ctx, f.ws, task.ID, second); err != nil || ok {
		t.Fatalf("second: ok=%v err=%v, want false/nil", ok, err)
	}
	got, _ := f.repo.Get(ctx, f.ws, task.ID)
	if got.MessageID != first {
		t.Fatalf("message_id = %s, want the first %s", got.MessageID, first)
	}
	// Another workspace cannot claim it either.
	if ok, _ := f.repo.SetMessage(ctx, newID(), task.ID, second); ok {
		t.Fatal("set a message from another workspace")
	}
}

func ids(ts []domain.Task) []string {
	out := make([]string, len(ts))
	for i, k := range ts {
		out[i] = k.ID
	}
	return out
}

func TestListFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	me, other := newID(), newID()
	otherCh := newID()

	a := f.insert(t, func(k *domain.Task) { k.AssigneeID = me; k.DueAt = ptr(at(-time.Hour)) })
	b := f.insert(t, func(k *domain.Task) {
		k.AssigneeID = me
		k.Status = domain.StatusDoing
		k.DueAt = ptr(at(2 * time.Hour))
	})
	c := f.insert(t, func(k *domain.Task) {
		k.AssigneeID = other
		k.Status = domain.StatusDone
		k.ChannelID = otherCh
		k.CreatorID = me
	})
	d := f.insert(t, func(k *domain.Task) {
		k.AssigneeID = me
		k.Status = domain.StatusDone
		k.DueAt = ptr(at(-2 * time.Hour))
	})
	// Another workspace never shows up.
	foreign := postgres.New(testPool(t))
	if _, err := foreign.Insert(ctx, domain.Task{
		ID: newID(), WorkspaceID: newID(), ChannelID: f.ch, Title: "x", CreatorID: me, AssigneeID: me,
		Priority: domain.PriorityNormal, Status: domain.StatusTodo, CreatedAt: at(0), UpdatedAt: at(0),
	}); err != nil {
		t.Fatal(err)
	}

	list := func(flt usecase.Filter, after string, limit int) []string {
		t.Helper()
		got, err := f.repo.List(ctx, f.ws, flt, after, limit)
		if err != nil {
			t.Fatal(err)
		}
		return ids(got)
	}
	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	eq("all, newest first", list(usecase.Filter{}, "", 10), d.ID, c.ID, b.ID, a.ID)
	eq("assignee", list(usecase.Filter{AssigneeID: me}, "", 10), d.ID, b.ID, a.ID)
	eq("creator", list(usecase.Filter{CreatorID: me}, "", 10), c.ID)
	eq("channel", list(usecase.Filter{ChannelID: otherCh}, "", 10), c.ID)
	eq("statuses", list(usecase.Filter{Statuses: []domain.Status{domain.StatusTodo, domain.StatusDoing}}, "", 10), b.ID, a.ID)
	eq("open only", list(usecase.Filter{OpenOnly: true}, "", 10), b.ID, a.ID)
	now := at(0)
	eq("due before now", list(usecase.Filter{DueBefore: &now}, "", 10), d.ID, a.ID)
	from := at(-90 * time.Minute)
	eq("due window", list(usecase.Filter{DueFrom: &from, DueBefore: &now}, "", 10), a.ID)
	eq("page 1", list(usecase.Filter{}, "", 2), d.ID, c.ID)
	eq("page 2", list(usecase.Filter{}, c.ID, 2), b.ID, a.ID)
	eq("past the end", list(usecase.Filter{}, a.ID, 2))
	if _, err := f.repo.List(ctx, f.ws, usecase.Filter{}, "nope", 2); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("bad cursor: err = %v, want ErrNotFound", err)
	}
}

// emptyTable removes every task. The claims look at all workspaces, so the
// tests that use them start from an empty table; tests of the package do not
// run in parallel.
func emptyTable(t *testing.T) {
	t.Helper()
	if _, err := testPool(t).Exec(context.Background(), `DELETE FROM task_items`); err != nil {
		t.Fatal(err)
	}
}

// claim runs fn in a transaction like the due scan does.
func claim(t *testing.T, f fixture, fn func(ctx context.Context) error) {
	t.Helper()
	if err := database.NewTxManager(testPool(t)).Do(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func TestClaimDueSoonAndOverdue(t *testing.T) {
	emptyTable(t)
	f := newFixture(t)
	ctx := context.Background()
	me := newID()
	now := at(0)
	in := func(d time.Duration) func(*domain.Task) {
		return func(k *domain.Task) { k.AssigneeID, k.DueAt = me, ptr(now.Add(d)) }
	}
	soon := f.insert(t, in(6*time.Hour))
	edge := f.insert(t, in(24*time.Hour)) // exactly 24 hours: due soon
	far := f.insert(t, in(25*time.Hour))
	late := f.insert(t, in(-time.Minute))
	f.insert(t, func(k *domain.Task) { k.DueAt = ptr(now.Add(time.Hour)) })                                                // no assignee
	f.insert(t, func(k *domain.Task) { k.AssigneeID, k.Status, k.DueAt = me, domain.StatusDone, ptr(now.Add(time.Hour)) }) // done
	f.insert(t, func(k *domain.Task) { k.AssigneeID = me })                                                                // no due date

	var got []usecase.Due
	claim(t, f, func(ctx context.Context) error {
		var err error
		got, err = f.repo.ClaimDueSoon(ctx, now, now.Add(24*time.Hour), 100)
		return err
	})
	have := map[string]usecase.Due{}
	for _, d := range got {
		have[d.TaskID] = d
	}
	if len(got) != 2 || have[soon.ID].TaskID == "" || have[edge.ID].TaskID == "" {
		t.Fatalf("due soon claimed %+v, want the 6h and 24h tasks", got)
	}
	d := have[soon.ID]
	if d.WorkspaceID != f.ws || d.AssigneeID != me || !d.DueAt.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("due = %+v", d)
	}

	// The marks are stored, and updated_at is left alone so cards keep their version.
	row, _ := f.repo.Get(ctx, f.ws, soon.ID)
	if row.DueSoonSentAt == nil || !row.DueSoonSentAt.Equal(now) || row.OverdueSentAt != nil || !row.UpdatedAt.Equal(soon.UpdatedAt) {
		t.Fatalf("marked row = %+v", row)
	}
	if r, _ := f.repo.Get(ctx, f.ws, far.ID); r.DueSoonSentAt != nil {
		t.Fatal("a task 25h out was marked")
	}

	// A second scan finds nothing new.
	claim(t, f, func(ctx context.Context) error {
		var err error
		got, err = f.repo.ClaimDueSoon(ctx, now, now.Add(24*time.Hour), 100)
		return err
	})
	if len(got) != 0 {
		t.Fatalf("second claim = %+v, want none", got)
	}

	claim(t, f, func(ctx context.Context) error {
		var err error
		got, err = f.repo.ClaimOverdue(ctx, now, 100)
		return err
	})
	if len(got) != 1 || got[0].TaskID != late.ID {
		t.Fatalf("overdue claimed %+v, want only %s", got, late.ID)
	}
	if r, _ := f.repo.Get(ctx, f.ws, late.ID); r.OverdueSentAt == nil || r.DueSoonSentAt != nil {
		t.Fatalf("overdue row = %+v", r)
	}
}

func TestClaimHonoursTheBatchAndSkipsLockedRows(t *testing.T) {
	emptyTable(t)
	f := newFixture(t)
	ctx := context.Background()
	me := newID()
	now := at(0)
	var all []string
	for range 3 {
		all = append(all, f.insert(t, func(k *domain.Task) { k.AssigneeID, k.DueAt = me, ptr(now.Add(-time.Hour)) }).ID)
	}

	pool := testPool(t)
	tx1 := database.NewTxManager(pool)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var held []usecase.Due
	go func() {
		done <- tx1.Do(ctx, func(ctx context.Context) error {
			var err error
			held, err = f.repo.ClaimOverdue(ctx, now, 2)
			close(started)
			<-release
			return err
		})
	}()
	<-started

	// While the first claim is open its two rows are locked; the other
	// transaction gets the remaining one instead of waiting.
	var second []usecase.Due
	claim(t, f, func(ctx context.Context) error {
		var err error
		second, err = f.repo.ClaimOverdue(ctx, now, 2)
		return err
	})
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(held) != 2 || len(second) != 1 {
		t.Fatalf("claims = %d and %d, want 2 and 1", len(held), len(second))
	}
	seen := map[string]bool{}
	for _, d := range append(held, second...) {
		seen[d.TaskID] = true
	}
	for _, id := range all {
		if !seen[id] {
			t.Errorf("task %s was never claimed", id)
		}
	}
}

func TestRolledBackClaimLeavesTheMarkUnset(t *testing.T) {
	emptyTable(t)
	f := newFixture(t)
	ctx := context.Background()
	now := at(0)
	task := f.insert(t, func(k *domain.Task) { k.AssigneeID, k.DueAt = newID(), ptr(now.Add(-time.Hour)) })

	boom := errors.New("publish failed")
	err := database.NewTxManager(testPool(t)).Do(ctx, func(ctx context.Context) error {
		if _, err := f.repo.ClaimOverdue(ctx, now, 10); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if row, _ := f.repo.Get(ctx, f.ws, task.ID); row.OverdueSentAt != nil {
		t.Fatal("the mark survived a rollback, so the event would be lost")
	}
}

func TestWithoutDatabase(t *testing.T) {
	repo := postgres.New(nil)
	if _, err := repo.Get(context.Background(), newID(), newID()); err == nil {
		t.Fatal("want an error without a pool")
	}
}
