// Package app holds the shared contracts every module depends on.
// It must not import any other package from this project.
package app

import "github.com/go-chi/chi/v5"

// Module is one feature of the server. See docs/architecture.md section 5.
type Module interface {
	Name() string
	// Requires lists modules this one cannot run without.
	Requires() []string
	// Provide is phase 1: expose services to other modules.
	Provide(s *Services)
	// Init is phase 2: build use cases and register rules.
	Init(d *Deps) error
	// Routes mounts endpoints; the router is already scoped to /api/v1/{name}.
	Routes(r chi.Router)
	// Subscribe listens to events from other modules.
	Subscribe(bus EventBus)
}

// Core modules are always enabled regardless of MODULES_ENABLED.
var CoreModules = []string{"auth", "workspace", "chat", "notify"}
