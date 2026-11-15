// Package migrations embeds the workspace module's goose migrations. The
// files sit under "workspace/" because migrations.Run reads the "<owner>"
// directory of the FS it is given.
package migrations

import "embed"

//go:embed workspace/*.sql
var FS embed.FS
