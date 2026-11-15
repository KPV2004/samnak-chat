package app

import "log/slog"

// Deps is passed to every module's Init. Infrastructure types are exposed
// through small interfaces so modules never import platform internals they
// do not need.
type Deps struct {
	Logger        *slog.Logger
	Services      *Services
	Bus           EventBus
	Notifications NotificationRegistry
	Tx            TxManager
	// Realtime and Storage may be nil in tests and in the openapi command;
	// modules must not use them in Init, only in request handling.
	Realtime Realtime
	Storage  ObjectStorage
	// Platform carries infrastructure handles (db pool, redis, storage, jobs).
	// Typed accessors live in internal/platform; modules type-assert there.
	Platform map[string]any
}
