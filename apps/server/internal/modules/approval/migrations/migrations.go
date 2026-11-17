// Package migrations embeds the approval module's goose migrations. The files
// sit under "approval/" because migrations.Run reads the "<owner>" directory
// of the FS it is given.
package migrations

import "embed"

//go:embed approval/*.sql
var FS embed.FS
