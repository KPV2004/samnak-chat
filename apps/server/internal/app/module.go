// Package app holds the shared contracts every module depends on.
// It must not import any other package from this project.
package app

import "github.com/danielgtaylor/huma/v2"

// Module is one feature of the server. See docs/architecture.md section 5.
type Module interface {
	Name() string
	// Requires lists modules this one cannot run without.
	Requires() []string
	// Provide is phase 1: expose services to other modules.
	Provide(s *Services)
	// Init is phase 2: build use cases and register rules. Init must not do
	// I/O (no queries, no network): the openapi command runs it without a
	// database to collect every module's operations.
	Init(d *Deps) error
	// Routes registers endpoints with huma.Register. api is a group already
	// prefixed with /{name} under /api/v1, so operations appear in OpenAPI.
	// Give each operation an OperationID prefixed with the module name.
	Routes(api huma.API)
	// Subscribe listens to events from other modules.
	Subscribe(bus EventBus)
}

// Core modules are always enabled regardless of MODULES_ENABLED.
var CoreModules = []string{"auth", "workspace", "chat", "notify"}
