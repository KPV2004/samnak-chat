// Package migrations embeds the helpdesk module's goose migrations. The files
// sit under "helpdesk/" because migrations.Run reads the "<owner>" directory of
// the FS it is given.
package migrations

import "embed"

//go:embed helpdesk/*.sql
var FS embed.FS
