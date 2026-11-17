// Package migrations embeds the task module's goose migrations. The files
// sit under "task/" because migrations.Run reads the "<owner>" directory of
// the FS it is given.
package migrations

import "embed"

//go:embed task/*.sql
var FS embed.FS
