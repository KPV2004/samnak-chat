# Contributing

Thank you for helping with Samnak Chat. This file covers how to set up, what the rules are, and what a change needs before it is merged.

## Before you start

1. Read [`docs/architecture.md`](docs/architecture.md), especially section 4 (dependency rules) and section 5 (modules).
2. Read [`AGENTS.md`](AGENTS.md). It sets the roles and the rules for work done by agents, and the hard rules below come from it.
3. Pick an open task from [`tasks/todo.md`](tasks/todo.md). Use one branch per task: `task/<id>-<slug>`.
4. Use [Conventional Commits](https://www.conventionalcommits.org/) and cite the task: `feat(platform): in-memory event bus (M0-02)`.

## Set up

You need Go 1.26+, Node 22+, Docker and Git. `make lint` also needs `golangci-lint` on your machine. CI runs the latest version.

```sh
cp .env.example .env
npm install
make up
make migrate
make server     # API on :8080
make web        # web app on :5173
```

## Commands

| Command | What it does |
| --- | --- |
| `make up` | Start PostgreSQL, Redis, MinIO and Mailpit with `deploy/docker-compose.yml` |
| `make down` | Stop those services. Data is kept |
| `make migrate` | Run the migrations of the enabled modules (`cmd/migrate`) |
| `make server` | Run the API server on `:8080` |
| `make web` | Run the web app on `:5173` |
| `make openapi` | Regenerate `apps/server/api/openapi.yaml` and the types in `packages/core` |
| `make test` | `go test ./...` in `apps/server`, then `npm test` |
| `make lint` | `go vet`, `golangci-lint` and `npm run lint` |
| `npm run build` | Type-check and build the web app and the packages |
| `bash scripts/coverage.sh [80]` | Print the coverage of every domain and use-case package, and fail below the minimum (NFR-12) |
| `bash scripts/check-modules.sh <MODULES_ENABLED or unset> <optional modules>` | Start the server with one module set, then check `/healthz`, `/meta` and the routes of the optional modules |

For example, `bash scripts/check-modules.sh notify,task notify,task` starts the server with `MODULES_ENABLED=notify,task`. It then checks that `/meta` lists exactly those modules, and that the `notify` and `task` routes are mounted while the `approval` and `helpdesk` routes return 404. The second argument must list the same modules as the first.

## Module boundaries

These rules keep the modules separate. Each one is either checked by a tool or checked in review.

| Rule | How it is checked |
| --- | --- |
| `internal/app` imports nothing from this project | `depguard` in golangci-lint |
| `internal/platform` knows no feature, so it does not import `internal/modules` | `depguard` in golangci-lint |
| A module imports only `api.go` and `events.go` of another module | `internal/archtest`, run by `go test` |
| No module reads or writes another module's tables. Table names start with the module name | Review and the acceptance tests |
| `workspace_id` comes from the `Principal`, never from the request body | Review and the acceptance test for workspace isolation |
| IDs are UUIDv7. Timestamps are `timestamptz` in UTC | Review |
| Contracts in `internal/app` change only with a maintainer's approval | Review |

If you need a change to a contract in `internal/app`, describe the problem in your pull request. Do not change the contract in the same pull request without asking first.

## Tests expected

- **Domain and use-case logic.** Write the failing test first, then the code. Cover success, forbidden and invalid-input cases (SRS 16.2).
- **Server.** `go test ./...` passes, with `-race` in CI. Coverage of domain and use-case packages stays at or above 80%.
- **Web.** Each new behaviour has a component test. `npm test`, `npm run lint` and `npm run build` pass.
- **API changes.** Run `make openapi` and commit the result. CI fails when `apps/server/api` or `packages/core/src/api` does not match the code.
- **Module changes.** Run `check-modules.sh` with the module sets that use the change.
- **Migrations.** They run on an empty database.
- **Docs.** Update the docs when a user-visible behaviour changes, and add an entry under `[Unreleased]` in [`CHANGELOG.md`](CHANGELOG.md).

Do not silence a check to get to green. No `//nolint`, no skipped tests and no weakened assertions without a maintainer's approval.

## Pull requests

CI runs these jobs on every push:

| Job | What it checks |
| --- | --- |
| `go` | `go vet`, `golangci-lint`, `go test -race ./...`, and coverage of 80% |
| `node` | `npm run build`, `npm run lint` and `npm test` |
| `openapi` | The committed OpenAPI spec and generated types match the code |
| `modules` | The server starts with each module set and the routes answer as expected |
| `images` | The server and web images build for `linux/amd64` and `linux/arm64` |

A maintainer reviews and merges each pull request.

## Definition of done (SRS 16.2)

- Tests cover success, forbidden and invalid-input cases
- CI passes: lint, tests, module-boundary check
- OpenAPI and web types regenerated
- Migrations run on an empty database
- Docs updated if user-visible behaviour changed
