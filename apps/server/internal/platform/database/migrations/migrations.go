// Package migrations holds the platform goose migration set and the runner
// shared by every owner (platform and modules).
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed platform/*.sql
var platformFS embed.FS

// Platform is the migration set owned by the platform.
// Its files live under the "platform" directory of the FS.
var Platform = platformFS

// Run applies the pending migrations of one owner (the "platform" or a module
// name). The set is read from the "<owner>" directory of fsys and tracked in
// its own version table, goose_<owner>.
func Run(ctx context.Context, db *sql.DB, owner string, fsys fs.FS) error {
	sub, err := fs.Sub(fsys, owner)
	if err != nil {
		return fmt.Errorf("migrations: %s: %w", owner, err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub,
		goose.WithTableName("goose_"+owner))
	if err != nil {
		return fmt.Errorf("migrations: %s: %w", owner, err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrations: %s: %w", owner, err)
	}
	return nil
}
