// Package migrations embeds the auth goose migrations. The files live under
// "auth/" because migrations.Run reads the "<owner>" directory of the FS.
package migrations

import "embed"

//go:embed auth/*.sql
var FS embed.FS
