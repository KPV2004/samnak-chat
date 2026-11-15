// Package migrations embeds the chat module's goose migrations. The files sit
// under "chat/" because migrations.Run reads the "<owner>" directory of the
// FS it is given.
package migrations

import "embed"

//go:embed chat/*.sql
var FS embed.FS
