// Command migrate runs per-module goose migrations for enabled modules (task M0-06).
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/config"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

// moduleMigrations lists the migration sets of modules, keyed by module name.
// Each FS holds its files under a "<module>" directory. Modules join here later.
var moduleMigrations = map[string]embed.FS{
	"auth":      auth.Migrations,
	"chat":      chat.Migrations,
	"workspace": workspace.Migrations,
}

type settings struct {
	DatabaseURL string
	Modules     []string
}

// loadSettings reads only the two variables migrate needs. config.Load is not
// used because it also requires server secrets such as JWT_SECRET.
func loadSettings() (settings, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return settings{}, errors.New("DATABASE_URL is required")
	}
	// The same module list as the server, so their migrations run.
	mods := config.ParseModules(os.LookupEnv("MODULES_ENABLED"))
	return settings{DatabaseURL: url, Modules: mods}, nil
}

// selectOwners returns the enabled modules that have a migration set.
func selectOwners(sets map[string]embed.FS, enabled []string) []string {
	var out []string
	for _, m := range enabled {
		if _, ok := sets[m]; ok {
			out = append(out, m)
		}
	}
	return out
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadSettings()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	// River's migrator needs a pgx pool; goose above works on database/sql.
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return migrate(ctx, db, pool, moduleMigrations, cfg.Modules)
}

// migrate runs the platform set (goose plus River's own tables), then the set
// of every enabled module.
func migrate(ctx context.Context, db *sql.DB, pool *pgxpool.Pool, sets map[string]embed.FS, enabled []string) error {
	if err := migrations.Run(ctx, db, "platform", migrations.Platform); err != nil {
		return err
	}
	if err := migrations.River(ctx, pool); err != nil {
		return err
	}
	for _, m := range selectOwners(sets, enabled) {
		if err := migrations.Run(ctx, db, m, sets[m]); err != nil {
			return err
		}
	}
	return nil
}
