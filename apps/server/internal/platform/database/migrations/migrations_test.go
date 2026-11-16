package migrations_test

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

//go:embed testdata/demo/*.sql
var testdataFS embed.FS

func startDB(t *testing.T) (*sql.DB, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("test"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
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
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return db, pool
}

func exists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var ok bool
	if err := db.QueryRow("SELECT to_regclass($1) IS NOT NULL", table).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestRunTwiceIsNoOpAndUsesOwnVersionTable(t *testing.T) {
	db, _ := startDB(t)
	ctx := context.Background()
	demoFS, err := fs.Sub(testdataFS, "testdata")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrations.Run(ctx, db, "platform", migrations.Platform); err != nil {
			t.Fatalf("run %d platform: %v", i, err)
		}
		if err := migrations.Run(ctx, db, "demo", demoFS); err != nil {
			t.Fatalf("run %d demo: %v", i, err)
		}
	}
	for _, tb := range []string{"platform_outbox_events", "goose_platform", "demo_things", "goose_demo"} {
		if !exists(t, db, tb) {
			t.Errorf("table %s missing", tb)
		}
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM goose_platform WHERE version_id > 0").Scan(&n); err != nil || n != 1 {
		t.Fatalf("goose_platform applied = %d, err %v; want 1", n, err)
	}
}

// River's tables belong to the platform set, so cmd/migrate creates them
// before any module migration.
func TestRiverRunsInThePlatformSet(t *testing.T) {
	_, pool := startDB(t)
	for i := 0; i < 2; i++ {
		if err := migrations.River(context.Background(), pool); err != nil {
			t.Fatalf("river %d: %v", i, err)
		}
	}
	for _, table := range []string{"river_job", "river_leader", "river_queue", "river_migration"} {
		if !existsPool(t, pool, table) {
			t.Errorf("table %s missing", table)
		}
	}
}

func existsPool(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", table).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}
