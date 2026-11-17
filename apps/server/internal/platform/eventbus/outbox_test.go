package eventbus_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
)

type thingMade struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

func (thingMade) EventName() string { return "demo.thing_made" }

type thingGone struct {
	ID string `json:"id"`
}

func (thingGone) EventName() string { return "demo.thing_gone" }

// syncBuffer collects log output from goroutines that write concurrently.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// sharedPG is one postgres container for the whole package; every test gets
// its own database in it, so tests stay isolated without a container each.
var sharedPG struct {
	once sync.Once
	url  func(db string) string
	err  error
	n    atomic.Int32
	root *pgxpool.Pool
}

func startContainer() {
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		sharedPG.err = err
		return
	}
	cleanups = append(cleanups, func() { _ = testcontainers.TerminateContainer(c) })
	base, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		sharedPG.err = err
		return
	}
	root, err := database.Open(ctx, base)
	if err != nil {
		sharedPG.err = err
		return
	}
	sharedPG.root = root
	sharedPG.url = func(db string) string { return strings.Replace(base, "/test?", "/"+db+"?", 1) }
}

var cleanups []func()

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedPG.root != nil {
		sharedPG.root.Close()
	}
	for _, f := range cleanups {
		f()
	}
	os.Exit(code)
}

// startPool returns a pool on a fresh database with the platform migrations
// applied, as cmd/migrate does before the server starts.
func startPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	sharedPG.once.Do(startContainer)
	if sharedPG.err != nil {
		t.Skipf("skipping integration test: cannot start postgres container: %v", sharedPG.err)
	}
	ctx := context.Background()
	name := fmt.Sprintf("outbox_%d", sharedPG.n.Add(1))
	if _, err := sharedPG.root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	url := sharedPG.url(name)
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := migrations.Run(ctx, db, "platform", migrations.Platform); err != nil {
		t.Fatal(err)
	}
	return pool
}

// newRelay returns a started outbox bus. relay is stopped when the test ends.
func newRelay(t *testing.T, pool *pgxpool.Pool, logger *slog.Logger, protos ...app.Event) *eventbus.Bus {
	t.Helper()
	b := eventbus.NewOutbox(pool, logger)
	b.Declare(protos...)
	return b
}

func startRelay(t *testing.T, b *eventbus.Bus) {
	t.Helper()
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { stopRelay(t, b) })
}

func stopRelay(t *testing.T, b *eventbus.Bus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := b.Stop(ctx); err != nil {
		t.Errorf("stop: %v", err)
	}
}

// received counts what a handler saw, by event ID.
type received struct {
	mu   sync.Mutex
	seen map[string]int
	ch   chan thingMade
}

func newReceived() *received {
	return &received{seen: map[string]int{}, ch: make(chan thingMade, 1024)}
}

func (r *received) handler(_ context.Context, e app.Event) error {
	m, ok := e.(thingMade)
	if !ok {
		return errors.New("not a thingMade")
	}
	r.mu.Lock()
	r.seen[m.ID]++
	r.mu.Unlock()
	r.ch <- m
	return nil
}

func (r *received) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[id]
}

func (r *received) wait(t *testing.T, d time.Duration) thingMade {
	t.Helper()
	select {
	case m := <-r.ch:
		return m
	case <-time.After(d):
		t.Fatalf("no event within %s", d)
		return thingMade{}
	}
}

func (r *received) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case m := <-r.ch:
		t.Fatalf("unexpected event %+v", m)
	case <-time.After(d):
	}
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPublishOutsideTransactionRunsHandlersNow(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)

	// The relay is not started: delivery must be synchronous.
	if err := b.Publish(context.Background(), thingMade{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	if got.count("a") != 1 {
		t.Fatalf("handler ran %d times, want 1 before Publish returned", got.count("a"))
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events`); n != 0 {
		t.Fatalf("outbox rows = %d, want 0 outside a transaction", n)
	}
}

func TestPublishInTransactionNeedsDeclaredType(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	tx := database.NewTxManager(pool)

	err := tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, thingGone{ID: "x"})
	})
	if err == nil || !strings.Contains(err.Error(), "demo.thing_gone") {
		t.Fatalf("undeclared publish error = %v, want one naming the event", err)
	}

	// A pointer is a different type from the declared value, so it would
	// decode into something else; refuse it as well.
	err = tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, &thingMade{ID: "p"})
	})
	if err == nil {
		t.Fatal("publishing *thingMade for a declared thingMade succeeded, want an error")
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events`); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

func TestCommittedEventIsDeliveredWithinOneSecondAndRolledBackNever(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)
	startRelay(t, b)
	tx := database.NewTxManager(pool)

	rollback := errors.New("rollback")
	err := tx.Do(context.Background(), func(ctx context.Context) error {
		if err := b.Publish(ctx, thingMade{ID: "rolled-back"}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("do error = %v", err)
	}
	got.none(t, 3*time.Second) // longer than one poll

	start := time.Now()
	err = tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, thingMade{ID: "committed", Count: 7})
	})
	if err != nil {
		t.Fatal(err)
	}
	m := got.wait(t, time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("delivery took %s, want under 1s", elapsed)
	}
	if m != (thingMade{ID: "committed", Count: 7}) {
		t.Fatalf("decoded event = %+v", m)
	}
	// Marking happens after the handler; give the claim transaction a moment.
	waitFor(t, 2*time.Second, func() bool {
		return scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NOT NULL`) == 1
	})
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events`); n != 1 {
		t.Fatalf("rows = %d, want only the committed event", n)
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func TestEventPublishedInOneRelayIsDeliveredByPollingWithoutNotify(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)
	b.SetPollInterval(100 * time.Millisecond)

	// Insert a row directly: no pg_notify, so only the poll can find it.
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO platform_outbox_events (id, name, payload) VALUES ($1, $2, $3)`,
		id, "demo.thing_made", json.RawMessage(`{"id":"polled","count":1}`)); err != nil {
		t.Fatal(err)
	}
	startRelay(t, b)
	if m := got.wait(t, 2*time.Second); m.ID != "polled" {
		t.Fatalf("got %+v", m)
	}
}

func TestTwoRelaysDeliverEachEventOnce(t *testing.T) {
	pool := startPool(t)
	got := newReceived()
	var relays []*eventbus.Bus
	for range 2 {
		b := newRelay(t, pool, discardLogger(), thingMade{})
		b.Subscribe("demo.thing_made", got.handler)
		b.SetPollInterval(200 * time.Millisecond)
		startRelay(t, b)
		relays = append(relays, b)
	}
	tx := database.NewTxManager(pool)

	const total = 200
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := relays[i%2]
			id := uuid.Must(uuid.NewV7()).String()
			if err := tx.Do(context.Background(), func(ctx context.Context) error {
				return b.Publish(ctx, thingMade{ID: id, Count: i})
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	ids := map[string]bool{}
	for range total {
		ids[got.wait(t, 10*time.Second).ID] = true
	}
	got.none(t, time.Second) // nothing delivered twice
	if len(ids) != total {
		t.Fatalf("distinct events = %d, want %d", len(ids), total)
	}
	for id := range ids {
		if got.count(id) != 1 {
			t.Fatalf("event %s delivered %d times", id, got.count(id))
		}
	}
}

func TestFailedClaimTransactionRedelivers(t *testing.T) {
	pool := startPool(t)
	ctx := context.Background()
	// A failing UPDATE of published_at stands in for a crash between handling
	// and marking: the handler ran, the claim transaction cannot commit.
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_publish() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'simulated crash'; END $$;
		CREATE TRIGGER fail_publish BEFORE UPDATE OF published_at ON platform_outbox_events
		FOR EACH ROW EXECUTE FUNCTION fail_publish();`); err != nil {
		t.Fatal(err)
	}

	logs := &syncBuffer{}
	b := newRelay(t, pool, slog.New(slog.NewTextHandler(logs, nil)), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)
	b.SetPollInterval(100 * time.Millisecond)
	startRelay(t, b)

	tx := database.NewTxManager(pool)
	if err := tx.Do(ctx, func(ctx context.Context) error {
		return b.Publish(ctx, thingMade{ID: "again"})
	}); err != nil {
		t.Fatal(err)
	}
	got.wait(t, 2*time.Second)
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NULL`); n != 1 {
		t.Fatalf("unpublished rows = %d, want 1 after the failed claim", n)
	}

	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_publish ON platform_outbox_events`); err != nil {
		t.Fatal(err)
	}
	got.wait(t, 2*time.Second) // second delivery
	waitFor(t, 2*time.Second, func() bool {
		return scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NULL`) == 0
	})
	if !strings.Contains(logs.String(), "simulated crash") {
		t.Errorf("failed claim not logged:\n%s", logs.String())
	}
}

func TestRelayHandlerErrorAndPanicAreLoggedAndEventIsDelivered(t *testing.T) {
	pool := startPool(t)
	logs := &syncBuffer{}
	b := newRelay(t, pool, slog.New(slog.NewTextHandler(logs, nil)), thingMade{})
	var calls atomic.Int32
	got := newReceived()
	b.Subscribe("demo.thing_made", func(context.Context, app.Event) error {
		calls.Add(1)
		return errors.New("boom")
	})
	b.Subscribe("demo.thing_made", func(context.Context, app.Event) error {
		calls.Add(1)
		panic("kaput")
	})
	b.Subscribe("demo.thing_made", got.handler) // still runs after the others fail
	startRelay(t, b)

	tx := database.NewTxManager(pool)
	if err := tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, thingMade{ID: "x"})
	}); err != nil {
		t.Fatal(err)
	}
	got.wait(t, 2*time.Second)
	waitFor(t, 2*time.Second, func() bool {
		return scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NOT NULL`) == 1
	})
	got.none(t, 2500*time.Millisecond) // a failed handler does not redeliver
	if calls.Load() != 2 {
		t.Errorf("failing handlers ran %d times, want 2", calls.Load())
	}
	for _, want := range []string{"boom", "kaput"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log missing %q:\n%s", want, logs.String())
		}
	}
}

func TestUndeclaredEventStaysUnpublishedAndDoesNotBlockOthers(t *testing.T) {
	pool := startPool(t)
	ctx := context.Background()
	logs := &syncBuffer{}
	// The first instance declares both types and writes both events.
	writer := newRelay(t, pool, discardLogger(), thingMade{}, thingGone{})
	// The second one only knows thingMade, like a server without that module.
	b := newRelay(t, pool, slog.New(slog.NewTextHandler(logs, nil)), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)
	b.SetPollInterval(100 * time.Millisecond)

	tx := database.NewTxManager(pool)
	if err := tx.Do(ctx, func(ctx context.Context) error {
		if err := writer.Publish(ctx, thingGone{ID: "g"}); err != nil {
			return err
		}
		return writer.Publish(ctx, thingMade{ID: "m"})
	}); err != nil {
		t.Fatal(err)
	}
	startRelay(t, b)

	if m := got.wait(t, 2*time.Second); m.ID != "m" {
		t.Fatalf("got %+v", m)
	}
	waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(logs.String(), "demo.thing_gone")
	})
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NULL AND name = 'demo.thing_gone'`); n != 1 {
		t.Fatalf("undeclared rows left unpublished = %d, want 1", n)
	}
	if strings.Count(logs.String(), "demo.thing_gone") > 3 {
		t.Errorf("undeclared event logged on every poll:\n%s", logs.String())
	}
}

func TestUndecodablePayloadIsLoggedAndSkipped(t *testing.T) {
	pool := startPool(t)
	logs := &syncBuffer{}
	b := newRelay(t, pool, slog.New(slog.NewTextHandler(logs, nil)), thingMade{})
	got := newReceived()
	b.Subscribe("demo.thing_made", got.handler)
	b.SetPollInterval(100 * time.Millisecond)

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO platform_outbox_events (id, name, payload) VALUES ($1, $2, '{"id":42}')`,
		uuid.Must(uuid.NewV7()), "demo.thing_made"); err != nil {
		t.Fatal(err)
	}
	startRelay(t, b)
	got.none(t, 600*time.Millisecond)
	waitFor(t, 2*time.Second, func() bool {
		return scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NOT NULL`) == 1
	})
	if !strings.Contains(logs.String(), "decode") {
		t.Errorf("decode failure not logged:\n%s", logs.String())
	}
}

func TestHandlersRunWithoutTheClaimTransaction(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	inTx := make(chan bool, 1)
	b.Subscribe("demo.thing_made", func(ctx context.Context, _ app.Event) error {
		_, ok := database.TxFromContext(ctx)
		inTx <- ok
		return nil
	})
	startRelay(t, b)
	tx := database.NewTxManager(pool)
	if err := tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, thingMade{ID: "x"})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-inTx:
		if ok {
			t.Fatal("handler context carries the claim transaction")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("not delivered")
	}
}

func TestStopWaitsForHandlerAndStartAfterStopWorks(t *testing.T) {
	pool := startPool(t)
	b := newRelay(t, pool, discardLogger(), thingMade{})
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	b.Subscribe("demo.thing_made", func(context.Context, app.Event) error {
		entered <- struct{}{}
		<-release
		return nil
	})
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	tx := database.NewTxManager(pool)
	if err := tx.Do(context.Background(), func(ctx context.Context) error {
		return b.Publish(ctx, thingMade{ID: "slow"})
	}); err != nil {
		t.Fatal(err)
	}
	<-entered

	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopped <- b.Stop(ctx)
	}()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned %v while a handler was running", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	// The handler finished and the claim committed before Stop returned.
	if n := scalar[int](t, pool, `SELECT count(*) FROM platform_outbox_events WHERE published_at IS NOT NULL`); n != 1 {
		t.Fatalf("published rows = %d, want 1", n)
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartWithoutDatabaseFails(t *testing.T) {
	b := eventbus.New(discardLogger())
	if err := b.Start(context.Background()); err == nil {
		t.Fatal("in-memory bus Start succeeded, want an error")
	}
	// A server without a database never starts a relay, and Stop is harmless.
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// fakeJobs records what the bus registers with the queue.
type fakeJobs struct {
	handlers map[string]app.JobHandler
	every    map[string]time.Duration
}

func (f *fakeJobs) Handle(kind string, h app.JobHandler, _ app.JobOptions) {
	if f.handlers == nil {
		f.handlers = map[string]app.JobHandler{}
	}
	f.handlers[kind] = h
}

func (f *fakeJobs) Every(interval time.Duration, kind string, _ any) {
	if f.every == nil {
		f.every = map[string]time.Duration{}
	}
	f.every[kind] = interval
}

func (f *fakeJobs) Enqueue(context.Context, string, any, app.EnqueueOptions) error { return nil }

func TestPruneJobDeletesPublishedRowsOlderThanSevenDays(t *testing.T) {
	pool := startPool(t)
	ctx := context.Background()
	b := newRelay(t, pool, discardLogger(), thingMade{})
	jobs := &fakeJobs{}
	b.RegisterJobs(jobs)

	if jobs.every["platform.outbox_prune"] != 24*time.Hour {
		t.Fatalf("schedule = %v, want every 24h", jobs.every)
	}
	h := jobs.handlers["platform.outbox_prune"]
	if h == nil {
		t.Fatal("no handler for platform.outbox_prune")
	}

	insert := func(name string, createdAgo, publishedAgo *time.Duration) string {
		id := uuid.Must(uuid.NewV7()).String()
		var published any
		if publishedAgo != nil {
			published = time.Now().Add(-*publishedAgo)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO platform_outbox_events (id, name, payload, created_at, published_at) VALUES ($1, $2, '{}', $3, $4)`,
			id, name, time.Now().Add(-*createdAgo), published); err != nil {
			t.Fatal(err)
		}
		return id
	}
	d := func(h int) *time.Duration { x := time.Duration(h) * time.Hour; return &x }
	oldPublished := insert("demo.thing_made", d(24*9), d(24*8))
	recentPublished := insert("demo.thing_made", d(24*3), d(24*2))
	oldUnpublished := insert("demo.thing_made", d(24*30), nil)

	if err := h(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	exists := func(id string) bool {
		return scalar[bool](t, pool, `SELECT EXISTS (SELECT 1 FROM platform_outbox_events WHERE id = $1)`, id)
	}
	if exists(oldPublished) {
		t.Error("published row older than 7 days survived")
	}
	if !exists(recentPublished) {
		t.Error("recent published row was deleted")
	}
	if !exists(oldUnpublished) {
		t.Error("unpublished row was deleted")
	}
}

func TestRegisterJobsIgnoresNilQueue(t *testing.T) {
	b := eventbus.New(discardLogger())
	b.RegisterJobs(nil) // tests and `server openapi` have no queue
}
