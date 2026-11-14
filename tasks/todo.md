# Task board: M0 project skeleton and M1 accounts

Rules for workers are in `AGENTS.md`. The planner ticks boxes after merging.
Server paths are relative to `apps/server/`. Verify server tasks with `go build ./... && go vet ./... && go test ./...`.

---

## [x] M0-01 platform/config: read configuration from the environment
**SRS:** PLT-01 · **Arch:** 3.2, 7 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

Load all settings from environment variables into a typed `Config` struct. Parse `MODULES_ENABLED` (comma-separated, trimmed, lower-cased, de-duplicated) and always add the core modules from `app.CoreModules`.

**Acceptance criteria**
- [ ] `config.Load() (Config, error)` reads every key in `.env.example`, with defaults for `HTTP_ADDR` (`:8080`)
- [ ] `Config.Modules` = core modules + `MODULES_ENABLED`, with no duplicates and core first
- [ ] Missing `DATABASE_URL` or `JWT_SECRET` returns an error that names the variable
- [ ] Table-driven tests cover empty, spaced, duplicate and upper-case module lists

**Files:** `internal/platform/config/`

---

## [x] M0-02 platform/eventbus: in-memory event bus
**SRS:** PLT-04 · **Arch:** 6.1 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

Implement `app.EventBus` in memory. The outbox implementation comes in M4 (PLT-10).

**Acceptance criteria**
- [ ] Publishing an event with no subscribers returns nil
- [ ] A handler that returns an error or panics is logged with `slog` and does not stop other handlers or return an error to the publisher
- [ ] Safe for concurrent `Publish` and `Subscribe` (test with `-race`)
- [ ] Handlers run synchronously in subscription order (documented), which keeps tests deterministic

**Files:** `internal/platform/eventbus/`

---

## [x] M0-03 app.Registry: dependency check, ordering and startup
**SRS:** PLT-01, 02, 03, 05 · **Arch:** 5.1, 5.2, 7 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** none

Complete `internal/app/registry.go`. This is the one task allowed to edit `internal/app`, and only `registry.go` and `registry_test.go`.

**Acceptance criteria**
- [ ] `(*Registry).Enable(names []string) ([]Module, error)` returns enabled modules in dependency order. Unknown module name → error naming it. A missing `Requires()` → error such as `module "approval" requires "task", which is not enabled`. A cycle → error listing the cycle
- [ ] `(*Registry).Start(mods []Module, d *Deps, r chi.Router) error` calls `Provide` on **all** modules, then `Init` on all, then mounts each module's `Routes` under `/{name}` on `r`, then `Subscribe`. The first `Init` error is returned wrapped with the module name
- [ ] Order is deterministic: ties are broken alphabetically
- [ ] Tests use fake modules and cover: happy path, missing dependency, cycle, unknown name, Provide-before-Init (a module looks up a service provided by a module ordered after it), and a disabled module's routes return 404

**Files:** `internal/app/registry.go`, `internal/app/registry_test.go`

---

## [x] M0-04 platform/httpx: router, middleware and error mapping
**SRS:** NFR-11, NFR-14, 14.1 · **Arch:** 3.2 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** none

**Acceptance criteria**
- [ ] `httpx.NewRouter(logger *slog.Logger) chi.Router` with middleware: request ID (`X-Request-ID`, generated if absent), JSON access log with request ID, panic recovery returning 500 JSON
- [ ] `httpx.WriteError(w, r, err)` maps `app.ErrInvalid/ErrForbidden/ErrNotFound/ErrConflict` (also when wrapped) to the error JSON and status codes in `tasks/plan.md`; anything else → 500 `internal` with no internal detail leaked
- [ ] `httpx.WriteJSON(w, status, v)` helper
- [ ] `GET /healthz` always 200; `GET /readyz` runs registered `func(ctx) error` checks and returns 503 with the failing check names
- [ ] `httptest` tests for each mapping, recovery and readiness

**Files:** `internal/platform/httpx/`

---

## [x] M0-05 platform/database: pgx pool and TxManager
**SRS:** PLT-07 · **Arch:** 3.2, 8.3 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** none

**Acceptance criteria**
- [ ] `database.Open(ctx, url) (*pgxpool.Pool, error)` with ping
- [ ] `TxManager` implements `app.TxManager`: stores `pgx.Tx` in the context, commits on nil, rolls back on error or panic, and reuses the outer transaction for nested `Do`
- [ ] `database.Querier(ctx, pool)` returns the tx from ctx or the pool, for sqlc-generated code to use
- [ ] Integration tests with testcontainers-go (`postgres:17-alpine`) for commit, rollback and nesting; skipped with a clear message when Docker is unavailable (`testing.Short()` or no Docker socket)

**Files:** `internal/platform/database/`

## [x] M0-06 cmd/migrate: per-module migrations and the outbox table
**SRS:** PLT-07, PLT-10 (table only) · **Arch:** 8.2 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M0-01, M0-05 (same session as M0-05)

**Acceptance criteria**
- [ ] goose migrations embedded per owner via `embed.FS`; a package `internal/platform/database/migrations` holds the platform set, starting with `platform_outbox_events (id uuid pk, name text, payload jsonb, created_at timestamptz, published_at timestamptz null)`
- [ ] Each module set uses its own goose version table (`goose_<module>`), so enabling a module later runs only its migrations
- [ ] `go run ./cmd/migrate` runs the platform set plus the sets of enabled modules (a `map[string]embed.FS` that modules join later), using `config.Load()`; it reads only `DATABASE_URL` and `MODULES_ENABLED` if M0-01 is not merged yet
- [ ] Integration test: migrate an empty database twice (second run is a no-op)

**Files:** `cmd/migrate/`, `internal/platform/database/migrations/`

---

## [x] M0-07 cmd/server: wiring, /meta, graceful shutdown, OpenAPI
**SRS:** PLT-01, 02, 06, NFR-11, NFR-13 · **Arch:** 5.2, 7 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** M0-01..05 merged (wave 2)

**Acceptance criteria**
- [ ] `main.go`: config → logger → db pool → event bus → registry `Enable` (an error stops the process with exit 1 and the message) → `Start` under `/api/v1` → HTTP server; SIGINT/SIGTERM shuts down gracefully with a 15 s timeout
- [ ] `GET /api/v1/meta` returns the contract in `tasks/plan.md`, served through Huma (`humachi`) so it appears in OpenAPI
- [ ] `go run ./cmd/server openapi` prints the OpenAPI YAML; `api/openapi.yaml` is committed
- [ ] API test with `httptest`: `/meta` lists modules; a fake module with a missing dependency makes startup fail with its name

**Files:** `cmd/server/`, `api/openapi.yaml`, `internal/platform/httpx/huma.go`

---

## [x] M0-08 Dev stack, Dockerfile, CI and module-boundary check
**SRS:** PLT-08, NFR-15 · **Arch:** 4 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** none

**Acceptance criteria**
- [ ] `deploy/docker-compose.yml`: postgres 17, redis 7, minio (with a bucket-creating init container) using the values in `.env.example`, with healthchecks; `make up` works
- [ ] `apps/server/Dockerfile`: multi-stage, distroless or alpine final image, builds for amd64 and arm64
- [ ] `apps/server/.golangci.yml` with `depguard` rules: `internal/app` may not import `internal/platform` or `internal/modules`; `internal/platform` may not import `internal/modules`; no package may import `internal/modules/*/domain|usecase|adapter` of a **different** module (write one rule per module, or add `internal/archtest/archtest_test.go` that walks `go list -deps` and enforces it)
- [ ] `.github/workflows/ci.yml`: Go job (vet, golangci-lint, `go test -race ./...` with Docker available for testcontainers) and Node job (`npm ci`, build, lint, test). Runs on push and pull request
- [ ] Prove the boundary check fails: describe in the final report a temporary violating import you tried and the error it produced (do not commit it)

**Files:** `deploy/`, `apps/server/Dockerfile`, `apps/server/.golangci.yml`, `apps/server/internal/archtest/`, `.github/workflows/`

---

## [x] M0-09 Web and packages/core skeleton
**SRS:** NFR-13, NFR-16, NFR-17 · **Arch:** 13 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** none (uses the `/meta` contract in `tasks/plan.md`)

**Acceptance criteria**
- [ ] `packages/core` (`@samnak/core`): TypeScript, no DOM types (`lib: ["ES2023"]`), exports `createApiClient({ baseUrl, getToken })` and `getMeta()` typed by the contract; `gen:api` script runs `openapi-typescript ../../apps/server/api/openapi.yaml -o src/api/schema.d.ts`; vitest test for `getMeta` with a mocked `fetch`
- [ ] `apps/web` (`@samnak/web`): Vite + React + TypeScript (strict) + TanStack Query + Zustand + ESLint; Vite dev proxy `/api` → `http://localhost:8080`; the home page shows "Samnak Chat" and the enabled modules from `/meta` with loading and error states
- [ ] i18n wiring with Thai and English strings (a small dictionary is fine; no library required)
- [ ] `npm install && npm run build && npm test && npm run lint` pass from the repo root
- [ ] `package-lock.json` committed

**Files:** `apps/web/`, `packages/core/`, root `package.json` (scripts only), `package-lock.json`

---

## [x] M0-10 Decision records 0001–0008
**SRS:** NFR-18 · **Arch:** 16 · **Tier:** low (Haiku) · **Size:** S · **Depends on:** none

**Acceptance criteria**
- [ ] One file per row of `docs/architecture.md` section 16, named `docs/adr/NNNN-kebab-title.md`, with sections Status (Accepted, 2026-11-14), Context, Decision, Consequences, Alternatives considered
- [ ] Content comes from `docs/architecture.md` and `docs/srs.md` only; nothing invented beyond common, well-known trade-offs
- [ ] `docs/architecture.md` section 16 links each row to its file

**Files:** `docs/adr/`, `docs/architecture.md` (section 16 only)

---

## Checkpoint A: after wave 1 (planner)
- [x] All wave-1 branches merged into `main`; `go mod tidy` clean
- [x] `go build ./... && go vet ./... && go test -race ./...` pass; `npm run build && npm test` pass; `golangci-lint run` 0 issues
- [x] CI green on `main` (run on 4917c4e)

Planner notes from checkpoint A:
- Added `app.ErrUnauthorized` and `app.ErrRateLimited`, mapped by httpx to 401 and 429.
- `Registry.Enable` does not add core modules; `cmd/server` passes `config.Modules`, which already includes them (M0-07).
- `httpx.NewRouter` mounts `/healthz`; `/readyz` is mounted with `httpx.MountReadiness(r, checks...)` (M0-07).
- `cmd/migrate` keeps its own `loadSettings`, because `config.Load` also requires `JWT_SECRET`.
- MinIO uses `bitnamilegacy/minio:latest`, since the official images no longer pull. Pin a tag or choose a maintained image before v0.1.
- Postgres host port is overridable with `POSTGRES_PORT`.

## Checkpoint M0: SRS 16.3 acceptance (planner)
- [x] `make up && make migrate && make server` → `GET /api/v1/meta` returns 200 with `modules: []`, `/readyz` 200 (checked with curl)
- [ ] The web page shows the modules from `/meta` in a browser (component tests pass with mocked fetch; not yet checked in a real browser)
- [x] An unknown or unsatisfied module stops the server with exit 1 and a message naming it (tests and manual run)
- [x] Decide how module endpoints reach OpenAPI (see below), then break M1 into tasks in this file

Planner notes from checkpoint M0:
- `cmd/server` has a temporary `skipUnbuiltCoreModules`: core names without a registered module are dropped with a warning. Delete it when auth, workspace, chat and notify exist (M1–M3).
- `.env.example` now leaves `MODULES_ENABLED` empty, because optional modules don't exist yet.
- Open contract decision before M1: `Module.Routes(r chi.Router)` gives modules no Huma handle, so their endpoints would not appear in OpenAPI. Decided and done: `Routes(api huma.API)`, with the registry passing a Huma group prefixed by `/{name}` (`Registry.Start` now takes a `huma.API`). Note: `server openapi` registers only `/meta`; when the first module lands, decide how the spec command builds module routes without a database.

---

# M1: accounts and workspaces

Scope: AUTH-01..07, WSP-01..08 (SRS section 16.3, M1). Draft by the worker session on user request; **the planner reviews the decisions below and the contract task M1-01 before any worker starts.** Module folders follow architecture 5.3; each module imports only the other's `api.go` and `events.go`.

Decisions to confirm:
- Access token (JWT, 15 min) carries `user_id` and `device_id` only. The workspace is chosen per request with the `X-Workspace-ID` header; the workspace module checks membership and builds the `Principal` from the database, never from the header alone. This gives WSP-05 (switching without logging in again) and makes WSP-08 immediate for REST, because a removed member simply fails the membership check.
- `workspace` requires `auth`; `auth` does not know `workspace`. `auth` exposes `Authenticator` in `auth/api.go`; `workspace` exposes `Authorizer` (`Can`) in `workspace/api.go`.
- Rate limiting (AUTH-06) uses Redis, so M1 adds `platform/redis`.
- Core modules `chat` and `notify` stay skipped by `skipUnbuiltCoreModules` until M2/M3; only `auth` and `workspace` leave that list.

## [ ] M1-01 Contracts for auth and workspace (planner)
**SRS:** AUTH-02, WSP-07 · **Arch:** 5, 12 · **Tier:** high (Opus) · **Size:** S · **Depends on:** none

- [ ] `internal/modules/auth/api.go`: `Authenticator` (`Authenticate(ctx, bearer string) (UserID, DeviceID string, err error)`), and `events.go` (`auth.user_registered`, `auth.device_revoked`)
- [ ] `internal/modules/workspace/api.go`: `Authorizer` with `Can(p app.Principal, action string, resource Resource) bool`, the action list per role from SRS section 2, and `events.go` (`workspace.created`, `workspace.member_added`, `workspace.member_removed`, `workspace.role_changed`)
- [ ] Confirm or change the decisions above; record them in an ADR if they change `docs/architecture.md`

**Files:** `internal/modules/auth/api.go`, `internal/modules/auth/events.go`, `internal/modules/workspace/api.go`, `internal/modules/workspace/events.go`

---

## [ ] M1-02 platform/redis: client, readiness check and rate limiter
**SRS:** AUTH-06, PLT-08 · **Arch:** 3.2 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

- [ ] `redis.Open(ctx, url)` returns a client and pings it; `Check` for `httpx.MountReadiness`
- [ ] `RateLimiter.Allow(ctx, key, limit, window) (bool, error)` using a fixed window in Redis; the limiter is an interface so use cases can take a fake
- [ ] Integration tests with testcontainers (redis:7), skipped with a clear message when Docker is unavailable

**Files:** `internal/platform/redis/`

---

## [ ] M1-03 auth domain and use cases
**SRS:** AUTH-01..04, 06 · **Arch:** 5.3, 12 · **Tier:** low (Sonnet, effort high) · **Size:** L · **Depends on:** M1-01, M1-02 (interface only)

- [ ] Register (email normalised, password hashed with argon2id), login (creates a device, returns access JWT 15 min and refresh token 30 days)
- [ ] Refresh rotates the token; reusing an old refresh token revokes every token of that device
- [ ] Logout one device and logout all; login attempts limited to 5 per minute per email and per IP through the rate-limiter port
- [ ] Ports for repository, clock and rate limiter; unit tests with fakes cover success, wrong password, unknown email, expired and reused refresh token, and rate limiting

**Files:** `internal/modules/auth/domain/`, `internal/modules/auth/usecase/`

---

## [ ] M1-04 auth Postgres adapter and migrations
**SRS:** AUTH-01..05 · **Arch:** 8.2 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M1-03

- [ ] Migrations `auth_users`, `auth_devices`, `auth_refresh_tokens` (UUIDv7 ids, `timestamptz`, refresh tokens stored hashed), embedded and registered in `cmd/migrate`
- [ ] Repository implements the use-case ports; integration tests with testcontainers cover rotation and reuse revocation in a real transaction

**Files:** `internal/modules/auth/adapter/postgres/`, `internal/modules/auth/migrations/`, `cmd/migrate/`

---

## [ ] M1-05 auth HTTP, module wiring and profile
**SRS:** AUTH-01..07 · **Arch:** 5.3, 12 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M1-03, M1-04

- [ ] `module.go` implements `app.Module` (`Routes(api huma.API)`) and provides `Authenticator` through `Services`
- [ ] Endpoints: register, login, refresh, logout, logout-all, list devices, revoke device, get and update profile (display name, avatar, language, time zone); errors use the error JSON
- [ ] API tests with `httptest` cover success, unauthorized and invalid input for each endpoint

**Files:** `internal/modules/auth/module.go`, `internal/modules/auth/adapter/http/`

---

## [ ] M1-06 workspace domain and use cases
**SRS:** WSP-01..04, 07 · **Arch:** 5.3, 12 · **Tier:** low (Sonnet, effort high) · **Size:** L · **Depends on:** M1-01

- [ ] Create workspace (creator becomes Owner), invite by email or link (expires in 7 days, can be revoked), accept invite, change role, remove member
- [ ] A workspace always keeps at least one Owner (changing the last Owner's role or removing them fails with `conflict`)
- [ ] `Can(principal, action, resource)` implements the role table in SRS section 2 and is the only permission check; table-driven tests cover every role and action
- [ ] Unit tests with fakes; `workspace_id` always comes from the `Principal`

**Files:** `internal/modules/workspace/domain/`, `internal/modules/workspace/usecase/`

---

## [ ] M1-07 workspace Postgres adapter and migrations
**SRS:** WSP-01..06 · **Arch:** 8.1, 8.2 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M1-06

- [ ] Migrations `workspace_workspaces`, `workspace_members`, `workspace_invites` (no foreign keys to auth tables; users are referenced by id only), registered in `cmd/migrate`
- [ ] Every query filters on `workspace_id`; integration tests prove a member of workspace A cannot read workspace B rows

**Files:** `internal/modules/workspace/adapter/postgres/`, `internal/modules/workspace/migrations/`, `cmd/migrate/`

---

## [ ] M1-08 workspace HTTP, membership middleware and module wiring
**SRS:** WSP-01..08 · **Arch:** 5.3, 8.1, 12 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** M1-05, M1-06, M1-07

- [ ] `module.go` (`Requires() = ["auth"]`) provides `Authorizer`; middleware reads the bearer token through `auth.Authenticator`, reads `X-Workspace-ID`, checks membership and puts the `Principal` into the context; a non-member gets 403
- [ ] Endpoints: create, list mine, get, invite, accept, revoke invite, list members, change role, remove member
- [ ] API tests cover non-member 403, removed member loses access on the next request, and last-Owner protection

**Files:** `internal/modules/workspace/module.go`, `internal/modules/workspace/adapter/http/`

---

## [ ] M1-09 Register auth and workspace in cmd/server
**SRS:** PLT-01, PLT-06 · **Arch:** 5.2, 7 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** M1-05, M1-08, M1-02

- [ ] `modules` in `cmd/server` lists auth and workspace; `Platform["redis"]` and a Redis readiness check are added; `skipUnbuiltCoreModules` only skips chat and notify
- [ ] Decide how `server openapi` includes module routes without a database, then regenerate `api/openapi.yaml` and the web types (`make openapi`)
- [ ] A test starts the server with the real modules and a fake database and checks `/meta` lists `auth, workspace`

**Files:** `cmd/server/`, `api/openapi.yaml`, `packages/core/src/api/`

---

## [ ] M1-10 Web: sign-up, login, workspaces
**SRS:** AUTH-01, 02, 04, 05, 07, WSP-01, 02, 05 · **Arch:** 13 · **Tier:** low (Sonnet) · **Size:** L · **Depends on:** M1-09

- [ ] `packages/core` API client sends the bearer token and `X-Workspace-ID`, refreshes the access token once on 401, and logs out when refresh fails
- [ ] Pages in Thai and English: register, login, create workspace, invite and accept, switch workspace, device list, profile
- [ ] Component tests with mocked fetch cover the happy path and error states

**Files:** `packages/core/src/`, `apps/web/src/`

---

## [ ] M1-11 M1 acceptance test (SRS 16.3)
**SRS:** AUTH-03, WSP-06 · **Arch:** 16 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** M1-09

- [ ] An API test with the real modules and testcontainers (Postgres and Redis): two users register, one creates a workspace and invites the other who joins; a user outside the workspace gets 403; a rotated refresh token used again is rejected and revokes the device

**Files:** `internal/modules/auth/`, `internal/modules/workspace/` (test files only), `cmd/server/` (test file only)

---

## Checkpoint M1: SRS 16.3 acceptance (planner)
- [ ] Two users register, one creates a workspace and invites the other
- [ ] A user outside the workspace gets 403
- [ ] An old rotated refresh token cannot be used again
- [ ] `go build ./... && go vet ./... && go test -race ./...`, `golangci-lint run`, `npm run build && npm test` pass; CI green
- [ ] Delete `auth` and `workspace` from `skipUnbuiltCoreModules`; break M2 into tasks

```
M1-01 contracts ──┬─► M1-03 auth usecase ─► M1-04 auth postgres ─► M1-05 auth http ──┐
M1-02 redis ──────┘                                                                   ├─► M1-08 workspace http ─► M1-09 wiring ─┬─► M1-10 web
                  └─► M1-06 workspace usecase ─► M1-07 workspace postgres ───────────┘                                          └─► M1-11 acceptance
```
