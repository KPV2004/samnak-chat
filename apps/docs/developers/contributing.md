# Contributing

Thank you for helping with Samnak Chat. This page summarizes what a change needs before it is merged. The full file is [`CONTRIBUTING.md`](https://github.com/KPV2004/samnak-chat/blob/main/CONTRIBUTING.md) in the repository.

## Before you start

1. Read the [architecture](/developers/architecture), especially the dependency rules and the module rules in [Modules and rules](/developers/modules).
2. Read [`AGENTS.md`](https://github.com/KPV2004/samnak-chat/blob/main/AGENTS.md). It describes how work is split between planner and workers, and the hard rules that come from the architecture.
3. Pick an open task from [`tasks/todo.md`](https://github.com/KPV2004/samnak-chat/blob/main/tasks/todo.md). Use one branch per task, named `task/<id>-<slug>`.
4. Write [Conventional Commits](https://www.conventionalcommits.org/) that cite the task, for example `feat(platform): in-memory event bus (M0-02)`.

Set up your machine with [Development setup](/developers/setup).

## What a change needs

- **Tests first.** For domain and use-case logic, write the failing test, then the code. Cover success, forbidden and invalid-input cases.
- **Server.** `go test ./...` passes, with `-race` in CI. Coverage of domain and use-case packages stays at or above 80%.
- **Web.** Each new behaviour has a component test. `npm test`, `npm run lint` and `npm run build` pass.
- **API changes.** Run `make openapi` and commit the result. CI fails when `apps/server/api` or `packages/core/src/api` does not match the code.
- **Module changes.** Run `check-modules.sh` with the module sets that use the change.
- **Migrations.** They run on an empty database.
- **Docs.** Update the docs when a user-visible behaviour changes, and add an entry under `[Unreleased]` in the [changelog](https://github.com/KPV2004/samnak-chat/blob/main/CHANGELOG.md).

Do not silence a check to get to green. That means no `//nolint`, no skipped tests and no weakened assertions, unless a maintainer approves it.

## What CI runs

Every push runs these jobs:

| Job | What it checks |
| --- | --- |
| `go` | `go vet`, `golangci-lint`, `go test -race ./...`, and the coverage gate |
| `node` | `npm run build`, `npm run lint` and `npm test` |
| `openapi` | The committed OpenAPI spec and generated types match the code |
| `modules` | The server starts with each module set, and the routes answer as expected |
| `images` | The server and web images build for `linux/amd64` and `linux/arm64` |

A maintainer reviews and merges each pull request.

## Definition of done

- Tests cover success, forbidden and invalid-input cases
- CI passes: lint, tests and the module-boundary check
- The OpenAPI spec and the web types are regenerated
- Migrations run on an empty database
- The docs are updated if a user-visible behaviour changed
