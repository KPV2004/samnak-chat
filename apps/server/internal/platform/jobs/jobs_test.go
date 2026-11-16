package jobs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/jobs"
)

// syncBuffer collects log output from handlers that may write concurrently.
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

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// requireDocker skips the test when Docker is not available, as the rest of
// the server integration tests do.
func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
}

// startPool returns a pool on a fresh postgres:17-alpine container without any
// migration applied.
func startPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	requireDocker(t)
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Skipf("skipping integration test: cannot start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// startMigratedPool returns a pool with River's migrations applied, as
// cmd/migrate does before the server starts.
func startMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := startPool(t)
	if err := migrations.River(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func newQueue(t *testing.T, pool *pgxpool.Pool, logger *slog.Logger) *jobs.Queue {
	t.Helper()
	if logger == nil {
		logger = testLogger()
	}
	return jobs.New(pool, logger)
}

func startQueue(t *testing.T, q *jobs.Queue) {
	t.Helper()
	if err := q.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { stopQueue(t, q) })
}

func stopQueue(t *testing.T, q *jobs.Queue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := q.Stop(ctx); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func rowState(t *testing.T, pool *pgxpool.Pool, kind string) (state string, attempts int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT state, attempt FROM river_job WHERE kind = $1 ORDER BY id DESC LIMIT 1`, kind).
		Scan(&state, &attempts)
	if err != nil {
		t.Fatalf("read job %s: %v", kind, err)
	}
	return state, attempts
}

func waitState(t *testing.T, pool *pgxpool.Pool, kind, want string, within time.Duration) (attempts int) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		state, attempts := rowState(t, pool, kind)
		if state == want {
			return attempts
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s state = %s after %s, want %s", kind, state, within, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// rawArgs inserts a job whose kind this server does not know, as a deploy
// that removed a module would leave behind.
type rawArgs struct {
	Name string `json:"kind"`
}

func (a rawArgs) Kind() string { return a.Name }

// insertUnknownKind writes a job directly with an insert-only client, the way
// another deployment (with a worker for the kind) would have.
func insertUnknownKind(t *testing.T, pool *pgxpool.Pool, kind string) {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: testLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insert(context.Background(), rawArgs{Name: kind}, nil); err != nil {
		t.Fatalf("insert %s: %v", kind, err)
	}
}

func countJobs(t *testing.T, pool *pgxpool.Pool, kind string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// New must not touch the database: the server builds it before modules start.
func TestNewDoesNotConnect(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var q app.Jobs = jobs.New(pool, testLogger())
	if q == nil {
		t.Fatal("New returned nil")
	}
}

func TestMigrateCreatesRiverTables(t *testing.T) {
	pool := startPool(t)
	for i := 0; i < 2; i++ { // the second run is a no-op
		if err := migrations.River(context.Background(), pool); err != nil {
			t.Fatalf("migrate %d: %v", i, err)
		}
	}
	for _, table := range []string{"river_job", "river_leader", "river_queue", "river_migration"} {
		if !tableExists(t, pool, table) {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestEnqueueJoinsTransaction(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)

	type payload struct {
		Message string `json:"message"`
	}
	received := make(chan json.RawMessage, 10)
	q.Handle("test.echo", func(_ context.Context, args json.RawMessage) error {
		received <- args
		return nil
	}, app.JobOptions{})
	startQueue(t, q)

	tm := database.NewTxManager(pool)
	rolledBack := errors.New("roll back")
	err := tm.Do(context.Background(), func(ctx context.Context) error {
		if err := q.Enqueue(ctx, "test.echo", payload{Message: "rolled back"}, app.EnqueueOptions{}); err != nil {
			return err
		}
		return rolledBack
	})
	if !errors.Is(err, rolledBack) {
		t.Fatalf("tx error = %v, want rolled back", err)
	}
	select {
	case args := <-received:
		t.Fatalf("handler ran for a rolled back job: %s", args)
	case <-time.After(500 * time.Millisecond):
	}
	if n := countJobs(t, pool, "test.echo"); n != 0 {
		t.Fatalf("jobs after rollback = %d, want 0", n)
	}

	if err := tm.Do(context.Background(), func(ctx context.Context) error {
		return q.Enqueue(ctx, "test.echo", payload{Message: "committed"}, app.EnqueueOptions{})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-received:
		var got payload
		if err := json.Unmarshal(args, &got); err != nil {
			t.Fatalf("args = %s: %v", args, err)
		}
		if got.Message != "committed" {
			t.Fatalf("args = %+v, want committed", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not run for the committed job")
	}

	// Outside a transaction the insert goes straight to the pool.
	if err := q.Enqueue(context.Background(), "test.echo", payload{Message: "direct"}, app.EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-received:
		var got payload
		if err := json.Unmarshal(args, &got); err != nil {
			t.Fatalf("args = %s: %v", args, err)
		}
		if got.Message != "direct" {
			t.Fatalf("args = %+v, want direct", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not run for the job enqueued outside a transaction")
	}
}

func TestEnqueueDefaultsToFiveAttempts(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	q.Handle("test.defaults", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
	startQueue(t, q)

	if err := q.Enqueue(context.Background(), "test.defaults", map[string]bool{"ok": true}, app.EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}
	var maxAttempts int
	if err := pool.QueryRow(context.Background(),
		`SELECT max_attempts FROM river_job WHERE kind = 'test.defaults'`).Scan(&maxAttempts); err != nil {
		t.Fatal(err)
	}
	if maxAttempts != 5 {
		t.Fatalf("max_attempts = %d, want 5", maxAttempts)
	}
}

func TestHandlerErrorRetriesUpToMaxAttempts(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	var calls atomic.Int32
	q.Handle("test.fail", func(context.Context, json.RawMessage) error {
		calls.Add(1)
		return errors.New("boom")
	}, app.JobOptions{MaxAttempts: 2})
	startQueue(t, q)

	if err := q.Enqueue(context.Background(), "test.fail", nil, app.EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}
	attempts := waitState(t, pool, "test.fail", "discarded", 15*time.Second)
	if attempts != 2 || calls.Load() != 2 {
		t.Fatalf("attempts = %d, handler calls = %d; want 2 and 2", attempts, calls.Load())
	}
}

func TestPanicCountsAsError(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	var calls atomic.Int32
	q.Handle("test.panic", func(context.Context, json.RawMessage) error {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		return nil
	}, app.JobOptions{MaxAttempts: 2})
	startQueue(t, q)

	if err := q.Enqueue(context.Background(), "test.panic", nil, app.EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}
	attempts := waitState(t, pool, "test.panic", "completed", 15*time.Second)
	if attempts != 2 || calls.Load() != 2 {
		t.Fatalf("attempts = %d, handler calls = %d; want 2 and 2", attempts, calls.Load())
	}
}

func TestUnknownKindCancelledAtStart(t *testing.T) {
	pool := startMigratedPool(t)

	// A previous deployment inserted the job and then went away.
	insertUnknownKind(t, pool, "test.unknown")
	if state, _ := rowState(t, pool, "test.unknown"); state == "cancelled" {
		t.Fatal("job was cancelled before any server started")
	}

	// A restarted server has no worker for the kind either, so it cancels the
	// job as soon as it starts.
	q2 := newQueue(t, pool, nil)
	q2.SetSweepInterval(time.Hour)
	startQueue(t, q2)
	if attempts := waitState(t, pool, "test.unknown", "cancelled", 5*time.Second); attempts != 0 {
		t.Fatalf("attempts = %d, want 0: an unknown kind is cancelled, never attempted", attempts)
	}
}

func TestUnknownKindCancelledWithinSweepInterval(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	q.SetSweepInterval(50 * time.Millisecond)
	q.Handle("test.known", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
	startQueue(t, q)

	insertUnknownKind(t, pool, "test.unknown")
	waitState(t, pool, "test.unknown", "cancelled", 5*time.Second)
}

func TestEveryRunsOnOneInstanceOnly(t *testing.T) {
	pool := startMigratedPool(t)
	q1 := newQueue(t, pool, nil)
	q2 := newQueue(t, pool, nil)
	for _, q := range []*jobs.Queue{q1, q2} {
		q.Handle("test.tick", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
		q.Every(200*time.Millisecond, "test.tick", map[string]int{"tick": 1})
		startQueue(t, q)
	}

	// Wait for a leader, then count the jobs the periodic scheduler inserts
	// during one second: one instance inserts about five, two would insert ten.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var leaders int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM river_leader WHERE expires_at > now()`).Scan(&leaders); err != nil {
			t.Fatal(err)
		}
		if leaders == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("leader count = %d, want 1", leaders)
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Maintenance services start staggered after election; wait for the first
	// periodic job before measuring.
	deadline = time.Now().Add(10 * time.Second)
	for countJobs(t, pool, "test.tick") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("periodic job did not run")
		}
		time.Sleep(25 * time.Millisecond)
	}

	before := countJobs(t, pool, "test.tick")
	time.Sleep(time.Second)
	got := countJobs(t, pool, "test.tick") - before
	if got < 2 || got > 8 {
		t.Fatalf("periodic jobs in 1s = %d, want about 5 (one instance)", got)
	}
}

func TestStopWaitsForRunningJob(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	q.Handle("test.slow", func(context.Context, json.RawMessage) error {
		close(started)
		<-release
		return nil
	}, app.JobOptions{Timeout: 30 * time.Second})
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(context.Background(), "test.slow", nil, app.EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not start")
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		done <- q.Stop(ctx)
	}()
	select {
	case err := <-done:
		t.Fatalf("Stop returned before the running job finished: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Stop did not return after the job finished")
	}
	if state, _ := rowState(t, pool, "test.slow"); state != "completed" {
		t.Fatalf("state = %s, want completed", state)
	}
}

func TestHandleAndEveryAfterStartAreReported(t *testing.T) {
	pool := startMigratedPool(t)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	q := newQueue(t, pool, logger)
	q.Handle("test.before", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
	startQueue(t, q)

	q.Handle("test.after", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
	q.Every(time.Second, "test.after", nil)

	out := logs.String()
	if !strings.Contains(out, "Handle") || !strings.Contains(out, "test.after") {
		t.Errorf("late Handle not reported in logs:\n%s", out)
	}
	if !strings.Contains(out, "Every") || !strings.Contains(out, "test.after") {
		t.Errorf("late Every not reported in logs:\n%s", out)
	}
}

func TestStartRejectsPeriodicJobWithoutHandler(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	q.Every(time.Minute, "test.orphan", nil)
	err := q.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "test.orphan") {
		t.Fatalf("start error = %v, want one naming test.orphan", err)
	}
}

func TestEnqueueUnknownKindFails(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	q.Handle("test.known", func(context.Context, json.RawMessage) error { return nil }, app.JobOptions{})
	startQueue(t, q)

	err := q.Enqueue(context.Background(), "test.unknown", nil, app.EnqueueOptions{})
	if err == nil || !strings.Contains(err.Error(), "test.unknown") {
		t.Fatalf("enqueue error = %v, want one naming test.unknown", err)
	}
}

func TestEnqueueBeforeStartFails(t *testing.T) {
	pool := startMigratedPool(t)
	q := newQueue(t, pool, nil)
	err := q.Enqueue(context.Background(), "test.kind", nil, app.EnqueueOptions{})
	if err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("enqueue error = %v, want not started", err)
	}
}
