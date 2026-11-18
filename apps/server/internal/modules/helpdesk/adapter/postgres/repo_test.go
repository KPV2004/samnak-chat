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
	repo    *postgres.Repo
	ctx     context.Context
	t       *testing.T
	members []string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testPool(t)
	return fixture{ws: newID(), repo: postgres.New(pool), ctx: context.Background(), t: t, members: []string{newID(), newID(), newID()}}
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
