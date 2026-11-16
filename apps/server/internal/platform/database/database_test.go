package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

// startPostgres returns a pool on a fresh postgres:17-alpine container.
func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
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
	if _, err := pool.Exec(ctx, "CREATE TABLE items (id int PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func count(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func insert(ctx context.Context, pool *pgxpool.Pool, id int) error {
	_, err := database.Querier(ctx, pool).Exec(ctx, "INSERT INTO items (id) VALUES ($1)", id)
	return err
}

func TestOpenFailsOnBadURL(t *testing.T) {
	if _, err := database.Open(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestQuerierWithoutTxReturnsPool(t *testing.T) {
	pool := startPostgres(t)
	if q := database.Querier(context.Background(), pool); q != any(pool) {
		t.Fatalf("expected pool, got %T", q)
	}
}

func TestTxFromContext(t *testing.T) {
	pool := startPostgres(t)
	tm := database.NewTxManager(pool)
	if tx, ok := database.TxFromContext(context.Background()); ok || tx != nil {
		t.Fatalf("TxFromContext outside Do = %v, %v; want nil, false", tx, ok)
	}
	err := tm.Do(context.Background(), func(ctx context.Context) error {
		tx, ok := database.TxFromContext(ctx)
		if !ok || tx == nil {
			t.Fatal("TxFromContext inside Do = nil, false; want the transaction")
		}
		if q := database.Querier(ctx, pool); q != any(tx) {
			t.Fatalf("Querier = %T, want %T", q, tx)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxCommit(t *testing.T) {
	pool := startPostgres(t)
	tm := database.NewTxManager(pool)
	err := tm.Do(context.Background(), func(ctx context.Context) error { return insert(ctx, pool, 1) })
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestTxRollbackOnError(t *testing.T) {
	pool := startPostgres(t)
	tm := database.NewTxManager(pool)
	boom := errors.New("boom")
	err := tm.Do(context.Background(), func(ctx context.Context) error {
		if err := insert(ctx, pool, 1); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if n := count(t, pool); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
}

func TestTxRollbackOnPanic(t *testing.T) {
	pool := startPostgres(t)
	tm := database.NewTxManager(pool)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic to propagate")
			}
		}()
		_ = tm.Do(context.Background(), func(ctx context.Context) error {
			_ = insert(ctx, pool, 1)
			panic("kaboom")
		})
	}()
	if n := count(t, pool); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
}

func TestTxNestedReusesOuter(t *testing.T) {
	pool := startPostgres(t)
	tm := database.NewTxManager(pool)
	boom := errors.New("boom")
	err := tm.Do(context.Background(), func(ctx context.Context) error {
		if err := insert(ctx, pool, 1); err != nil {
			return err
		}
		if err := tm.Do(ctx, func(ctx context.Context) error { return insert(ctx, pool, 2) }); err != nil {
			return err
		}
		return boom // outer failure rolls back the inner work too
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, pool); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	err = tm.Do(context.Background(), func(ctx context.Context) error {
		if err := insert(ctx, pool, 1); err != nil {
			return err
		}
		return tm.Do(ctx, func(ctx context.Context) error { return insert(ctx, pool, 2) })
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}
