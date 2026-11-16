// Package migrations embeds the notify module's goose migrations. The files
// sit under "notify/" because migrations.Run reads the "<owner>" directory of
// the FS it is given.
package migrations

import "embed"

//go:embed notify/*.sql
var FS embed.FS
