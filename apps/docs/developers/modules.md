# Modules and rules

Samnak Chat is built from modules. This page covers the rules that keep them separate, how the server starts them, and how to add one. The full description is in [`docs/architecture.md`](https://github.com/KPV2004/samnak-chat/blob/main/docs/architecture.md), sections 4 to 7.

## The module interface

Every module implements the `Module` interface in `internal/app`:

```go
type Module interface {
    Name() string
    Requires() []string        // modules this one cannot run without
    Provide(s *Services)       // phase 1: expose services to other modules
    Init(d *Deps) error        // phase 2: build use cases, register rules (no I/O)
    Routes(api huma.API)       // register Huma operations under /api/v1/{name}
    Subscribe(bus EventBus)    // listen to events from other modules
}
```

At startup the server reads `MODULES_ENABLED`, checks that every required module is present, and orders the modules by dependency. It then calls `Provide` on every module before any `Init`, so modules can look up each other's services in any order. The job queue and the outbox relay start last, so every handler is registered before the first event or job runs.

If a required module is missing, the server stops at startup with a message. A disabled module's endpoints are never mounted, so they return 404.

## Layout of a module

```
approval/
├── module.go            # wires adapters to use cases and registers with the system
├── api.go               # interfaces other modules may call
├── events.go            # events this module publishes
├── domain/              # entities and business rules
├── usecase/             # application flows and ports (required interfaces)
├── adapter/
│   ├── http/            # handlers and JSON shapes
│   ├── postgres/        # repository, written with sqlc
│   └── wire/            # builds the adapters for module.go
└── migrations/          # migrations of this module's tables
```

A simple module does not need every layer. Interfaces are created only where a boundary is crossed: the database, events and other modules.

## Rules

These rules keep the modules separate. Each one is checked by a tool or in review.

| Rule | How it is checked |
| --- | --- |
| `internal/app` imports nothing from this project | `depguard` in golangci-lint |
| `internal/platform` knows no feature, so it does not import `internal/modules` | `depguard` in golangci-lint |
| A module imports only `api.go` and `events.go` of another module | `internal/archtest`, run by `go test` |
| No module reads or writes another module's tables. Table names start with the module name | Review and the acceptance tests |
| `workspace_id` comes from the signed-in user's `Principal`, never from the request body | Review and the acceptance test for workspace isolation |
| IDs are UUIDv7. Timestamps are `timestamptz` in UTC | Review |
| Contracts in `internal/app` change only with a maintainer's approval | Review |

Inside a module, dependencies point one way: `adapter → usecase → domain`. The domain holds the entities, state changes and business errors. It contains no SQL, HTTP or JSON tags, and no calls to other modules.

If you need a change to a contract in `internal/app`, describe the problem in your pull request. Do not change the contract in the same pull request without asking first.

## Adding a module

1. Create `internal/modules/<name>` with the layout above.
2. Write `module.go` to satisfy `Module`, and list the modules it needs in `Requires()`.
3. Add migrations, with table names that start with the module name.
4. Declare the events in `events.go`. Register notification rules and card kinds in `Init`.
5. Add the module to the list in `cmd/server/main.go`.
6. Add the module to the `modules` matrix in `.github/workflows/ci.yml`, so CI starts the server with it.

A module built this way changes no file in any other module.

## Modules and features

| Module | Kind | Notes |
| --- | --- | --- |
| `auth` | core | Accounts, sessions, email |
| `workspace` | core | Workspaces, members, invites, groups, audit log, and the `Authorizer` that other modules use |
| `chat` | core | Channels, messages, threads, reactions, attachments, slash commands and message actions. Other modules add commands and actions through `ChatExtensions`. |
| `notify` | optional | Notifications, preferences and the WebSocket delivery |
| `task` | optional | Tasks, reminders, and the `/task` command |
| `approval` | optional | Requests, decisions, groups' default approvers |
| `helpdesk` | optional | Queues, categories, tickets, comments and the `/ticket` command |
