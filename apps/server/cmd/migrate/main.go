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
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

// moduleMigrations lists the migration sets of modules, keyed by module name.
// Each FS holds its files under a "<module>" directory. Modules join here later.
var moduleMigrations = map[string]embed.FS{}

type settings struct {
	DatabaseURL string
	Modules     []string
}

// loadSettings reads the two variables migrate needs.
// The planner swaps this for config.Load() once M0-01 is merged.
func loadSettings() (settings, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return settings{}, errors.New("DATABASE_URL is required")
	}
	return settings{DatabaseURL: url, Modules: parseModules(os.Getenv("MODULES_ENABLED"))}, nil
}

func parseModules(s string) []string {
	var out []string
	for _, m := range strings.Split(s, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
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
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return migrate(ctx, db, moduleMigrations, cfg.Modules)
}

// migrate runs the platform set, then the set of every enabled module.
func migrate(ctx context.Context, db *sql.DB, sets map[string]embed.FS, enabled []string) error {
	if err := migrations.Run(ctx, db, "platform", migrations.Platform); err != nil {
		return err
	}
	for _, m := range selectOwners(sets, enabled) {
		if err := migrations.Run(ctx, db, m, sets[m]); err != nil {
			return err
		}
	}
	return nil
}
