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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/migrations"
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
// workspaces and users.
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
		if err := dbmigrations.Run(ctx, db, "helpdesk", migrations.FS); err != nil {
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

// fixture is one workspace with a queue, so each test has its own data.
type fixture struct {
	ws      string
	pool    *pgxpool.Pool
	repo    *postgres.Repo
	ctx     context.Context
	t       *testing.T
	members []string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testPool(t)
	return fixture{ws: newID(), pool: pool, repo: postgres.New(pool), ctx: context.Background(), t: t, members: []string{newID(), newID(), newID()}}
}

func (f fixture) queue(prefix string) domain.Queue {
	f.t.Helper()
	q, err := domain.NewQueue(domain.QueueInput{Name: "Queue " + prefix, Prefix: prefix}, newID(), f.ws, time.Now())
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.repo.InsertQueue(f.ctx, q); err != nil {
		f.t.Fatal(err)
	}
	return q
}

func (f fixture) category(queueID string, isDefault bool) domain.Category {
	f.t.Helper()
	c, err := domain.NewCategory(domain.CategoryInput{QueueID: queueID, Name: "Cat", IsDefault: isDefault}, newID(), f.ws, time.Now())
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.repo.InsertCategory(f.ctx, c); err != nil {
		f.t.Fatal(err)
	}
	return c
}

func TestQueuePrefixIsUniquePerWorkspace(t *testing.T) {
	f := newFixture(t)
	f.queue("IT")
	dup, err := domain.NewQueue(domain.QueueInput{Name: "Again", Prefix: "IT"}, newID(), f.ws, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertQueue(f.ctx, dup); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("duplicate prefix: err = %v, want ErrConflict", err)
	}
	other, err := domain.NewQueue(domain.QueueInput{Name: "Other workspace", Prefix: "IT"}, newID(), newID(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertQueue(f.ctx, other); err != nil {
		t.Fatalf("same prefix in another workspace: %v", err)
	}
}

func TestQueueAndAgentsRoundTrip(t *testing.T) {
	f := newFixture(t)
	q := f.queue("HR")
	if err := f.repo.SetAgents(f.ctx, f.ws, q.ID, f.members[:2], time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.Queue(f.ctx, f.ws, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentIDs) != 2 || got.AgentIDs[0] != f.members[0] || got.AgentIDs[1] != f.members[1] {
		t.Fatalf("agents = %v, want %v", got.AgentIDs, f.members[:2])
	}

	channel := newID()
	got.Name, got.ChannelID = "People", channel
	if err := f.repo.UpdateQueue(f.ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetAgents(f.ctx, f.ws, q.ID, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err = f.repo.Queue(f.ctx, f.ws, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "People" || got.ChannelID != channel || len(got.AgentIDs) != 0 {
		t.Fatalf("after update: %+v", got)
	}

	got.ChannelID = ""
	at := time.Now().UTC()
	got.ArchivedAt = &at
	if err := f.repo.UpdateQueue(f.ctx, got); err != nil {
		t.Fatal(err)
	}
	active, err := f.repo.Queues(f.ctx, f.ws, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("archived queue listed as active: %+v", active)
	}
	all, err := f.repo.Queues(f.ctx, f.ws, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ChannelID != "" || all[0].ArchivedAt == nil {
		t.Errorf("archived queue with includeArchived = %+v", all)
	}
}

func TestQueuesAreScopedToTheirWorkspace(t *testing.T) {
	f := newFixture(t)
	q := f.queue("IT")
	if _, err := f.repo.Queue(f.ctx, newID(), q.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("other workspace: err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Queue(f.ctx, f.ws, "not-a-uuid"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("malformed ID: err = %v, want ErrNotFound", err)
	}
}

func TestAtMostOneDefaultCategoryPerWorkspace(t *testing.T) {
	f := newFixture(t)
	q := f.queue("IT")
	first := f.category(q.ID, true)
	c, err := domain.NewCategory(domain.CategoryInput{QueueID: q.ID, Name: "Second", IsDefault: true}, newID(), f.ws, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertCategory(f.ctx, c); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("second default without clearing: err = %v, want ErrConflict", err)
	}
	if err := f.repo.ClearDefault(f.ctx, f.ws, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertCategory(f.ctx, c); err != nil {
		t.Fatal(err)
	}
	cats, err := f.repo.Categories(f.ctx, f.ws, false)
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, cat := range cats {
		if cat.IsDefault {
			defaults++
			if cat.ID != c.ID {
				t.Errorf("default is %s, want the second one", cat.ID)
			}
		}
	}
	if defaults != 1 || first.ID == c.ID {
		t.Fatalf("defaults = %d", defaults)
	}
}

func TestCategoryRoundTripAndArchive(t *testing.T) {
	f := newFixture(t)
	it := f.queue("IT")
	hr := f.queue("HR")
	c := f.category(it.ID, false)

	got, err := f.repo.Category(f.ctx, f.ws, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.QueueID != it.ID || got.Name != "Cat" || got.IsDefault {
		t.Fatalf("category = %+v", got)
	}
	got.QueueID = hr.ID
	got.Name = "Moved"
	if err := f.repo.UpdateCategory(f.ctx, got); err != nil {
		t.Fatal(err)
	}
	moved, err := f.repo.Category(f.ctx, f.ws, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.QueueID != hr.ID || moved.Name != "Moved" {
		t.Fatalf("after move: %+v", moved)
	}
	if _, err := f.repo.Category(f.ctx, newID(), c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("other workspace: err = %v, want ErrNotFound", err)
	}
}

// ticketIn builds a ticket of q that takes number, without failing the test,
// so it can be called from a goroutine.
func ticketIn(ws string, q domain.Queue, categoryID string, number int64) (domain.Ticket, error) {
	t, err := domain.NewTicket(domain.NewTicketInput{
		WorkspaceID: ws, QueueID: q.ID, CategoryID: categoryID, RequesterID: newID(), Title: "Printer jams",
	}, newID(), time.Now())
	if err != nil {
		return domain.Ticket{}, err
	}
	t.Number, t.Key = number, domain.TicketKey(q.Prefix, number)
	return t, nil
}

func TestTicketNumbersAreUniqueUnderConcurrentCreates(t *testing.T) {
	f := newFixture(t)
	q := f.queue("IT")
	cat := f.category(q.ID, false)
	txm := database.NewTxManager(f.pool)

	const n = 25
	var wg sync.WaitGroup
	numbers := make(chan int64, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := txm.Do(f.ctx, func(ctx context.Context) error {
				prefix, number, err := f.repo.NextTicketNumber(ctx, f.ws, q.ID)
				if err != nil {
					return err
				}
				if prefix != "IT" {
					return fmt.Errorf("prefix = %q, want IT", prefix)
				}
				tk, err := ticketIn(f.ws, q, cat.ID, number)
				if err != nil {
					return err
				}
				numbers <- number
				return f.repo.InsertTicket(ctx, tk)
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(numbers)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent create: %v", err)
	}
	seen := map[int64]bool{}
	for num := range numbers {
		if seen[num] {
			t.Errorf("number %d was handed out twice", num)
		}
		seen[num] = true
	}
	for i := int64(1); i <= n; i++ {
		if !seen[i] {
			t.Errorf("number %d was never handed out", i)
		}
	}
}

func TestNumbersCountPerQueueAndStopWhenArchived(t *testing.T) {
	f := newFixture(t)
	it := f.queue("IT")
	hr := f.queue("HR")
	if _, n, err := f.repo.NextTicketNumber(f.ctx, f.ws, it.ID); err != nil || n != 1 {
		t.Fatalf("first IT number = %d, %v; want 1", n, err)
	}
	if _, n, err := f.repo.NextTicketNumber(f.ctx, f.ws, it.ID); err != nil || n != 2 {
		t.Fatalf("second IT number = %d, %v; want 2", n, err)
	}
	if prefix, n, err := f.repo.NextTicketNumber(f.ctx, f.ws, hr.ID); err != nil || n != 1 || prefix != "HR" {
		t.Fatalf("first HR number = %s-%d, %v; want HR-1", prefix, n, err)
	}

	at := time.Now().UTC()
	it.ArchivedAt = &at
	if err := f.repo.UpdateQueue(f.ctx, it); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.repo.NextTicketNumber(f.ctx, f.ws, it.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("archived queue: err = %v, want ErrNotFound", err)
	}
}

func TestTicketNumberIsUniqueWithinItsQueueOnly(t *testing.T) {
	f := newFixture(t)
	it := f.queue("IT")
	hr := f.queue("HR")
	cat := f.category(it.ID, false)
	hrCat := f.category(hr.ID, false)

	first, err := ticketIn(f.ws, it, cat.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertTicket(f.ctx, first); err != nil {
		t.Fatal(err)
	}
	dup, err := ticketIn(f.ws, it, cat.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertTicket(f.ctx, dup); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("same number in the same queue: err = %v, want ErrConflict", err)
	}
	other, err := ticketIn(f.ws, hr, hrCat.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertTicket(f.ctx, other); err != nil {
		t.Fatalf("same number in another queue: %v", err)
	}
}

func TestHistoryLinesAreAddedToATicket(t *testing.T) {
	f := newFixture(t)
	q := f.queue("IT")
	tk, err := ticketIn(f.ws, q, f.category(q.ID, false).ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.InsertTicket(f.ctx, tk); err != nil {
		t.Fatal(err)
	}
	h := domain.HistoryEntry{ID: newID(), TicketID: tk.ID, ActorID: tk.RequesterID, Action: domain.ActionCreated, At: time.Now()}
	if err := f.repo.InsertHistory(f.ctx, h); err != nil {
		t.Fatal(err)
	}
	orphan := h
	orphan.ID, orphan.TicketID = newID(), newID()
	if err := f.repo.InsertHistory(f.ctx, orphan); err == nil {
		t.Error("a history line for a missing ticket was stored")
	}
}
