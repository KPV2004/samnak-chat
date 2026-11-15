# Task board

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
- [x] Decide how module endpoints reach OpenAPI: `Module.Routes(api huma.API)`; M1 broken down below

Planner notes from checkpoint M0:
- `cmd/server` has a temporary `skipUnbuiltCoreModules`: core names without a registered module are dropped with a warning. Delete it when auth, workspace, chat and notify exist (M1–M3).
- `.env.example` now leaves `MODULES_ENABLED` empty, because optional modules don't exist yet.
- Open contract decision before M1: `Module.Routes(r chi.Router)` gives modules no Huma handle, so their endpoints would not appear in OpenAPI. Proposed fix: change it to `Routes(api huma.API)`, with the registry passing a Huma group prefixed by `/{name}`.

---

# M1: accounts and workspaces

Read the M1 section of `tasks/plan.md` first: it fixes the API shapes, token rules and the contracts below.
Module layout follows `docs/architecture.md` 5.3. Use sqlc (`go tool sqlc generate`) for queries and goose SQL files for migrations, exported from the module as `var Migrations embed.FS` (files under `migrations/`).
Do **not** edit `cmd/server`, `cmd/migrate`, `internal/app` or another module; the planner registers modules at checkpoint B. Test the module through its own `module.go` with a Huma test API and a real Postgres (testcontainers), as `cmd/server/main_test.go` does.

## [ ] M1-01 auth: register, login, access tokens, profile
**SRS:** AUTH-01, 02, 06, 07 · **Arch:** 5.3, 8, 12 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** none

**Acceptance criteria**
- [ ] Tables `auth_users`, `auth_devices`, `auth_refresh_tokens` per SRS 13 (UUIDv7 IDs, `timestamptz`), email unique case-insensitively
- [ ] Domain: email normalised (trim, lower-case), password 8–128 chars, argon2id hashing with parameters documented in code, locale `th|en`, timezone validated with `time.LoadLocation`
- [ ] `POST /auth/register`, `POST /auth/login`, `GET /auth/me`, `PATCH /auth/me` as in the plan; duplicate email → 409; wrong email or password → the same 401
- [ ] Login limit 5 per minute per email and per IP → 429, through a `usecase` port `LoginLimiter` with an in-memory adapter (the Redis one comes from M1-05)
- [ ] auth provides `app.Authenticator` (JWT verify + device not revoked) and `auth.Directory` in `Provide`
- [ ] Tests: domain unit tests; API tests for success, invalid input, wrong password, duplicate email, rate limit, expired and tampered JWT

**Files:** `internal/modules/auth/` (except `api.go`, which the planner owns)

## [ ] M1-02 auth: refresh rotation, logout, devices
**SRS:** AUTH-03, 04, 05 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** M1-01 (same session)

**Acceptance criteria**
- [ ] `POST /auth/refresh` rotates: the old refresh token is revoked and a new pair issued, in one transaction
- [ ] Reusing a revoked refresh token → 401 and every token of that device is revoked (test it)
- [ ] `POST /auth/logout` revokes the current device, or all the user's devices with `all_devices: true`; access tokens of revoked devices fail `Authenticate` at once
- [ ] `GET /auth/devices` (with `current`) and `DELETE /auth/devices/{id}`; another user's device → 404
- [ ] `auth_devices.last_seen_at` updates on login and refresh

**Files:** `internal/modules/auth/`

## [ ] M1-03 workspace: workspaces, members, roles, `Can`
**SRS:** WSP-01, 03, 04, 05, 06, 07, 08 · **Arch:** 8.1, 12 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** none

**Acceptance criteria**
- [ ] Tables `workspace_workspaces` (slug unique, `^[a-z0-9-]{3,40}$`) and `workspace_members (workspace_id, user_id, role, status, joined_at)`; every query filters on `workspace_id`
- [ ] `POST /workspace` makes the creator owner; `GET /workspace` lists the caller's active memberships with role
- [ ] `GET /workspace/members` (cursor pagination, names and emails through `auth.Directory`, falling back to empty strings if Lookup fails), `PATCH` and `DELETE /workspace/members/{userId}`
- [ ] Domain rules: at least one active owner always remains (409 otherwise); only owners grant or remove the owner role; admins manage everyone else; members and guests cannot manage (403)
- [ ] `workspace/api.go` exposes `Authorizer` with `Can(p app.Principal, action Action) bool` and the action constants M1 needs; document how later modules add actions
- [ ] workspace provides `app.MembershipResolver` (active members only, no cache) and `Authorizer`
- [ ] `events.go`: `MemberRemoved` (`workspace.member_removed`) and `MemberDeactivated` (`workspace.member_deactivated`), published on those changes
- [ ] Tests: domain unit tests for every role rule; API tests including a user of workspace A getting 403 on workspace B (NFR-06)

**Files:** `internal/modules/workspace/`

## [ ] M1-04 workspace: invites
**SRS:** WSP-02 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M1-03 (same session)

**Acceptance criteria**
- [ ] Table `workspace_invites` per SRS 13; tokens are 32 random bytes, only the hash is stored, the raw token is returned once on create
- [ ] `POST`, `GET`, `DELETE /workspace/invites` (admin and owner only) and `POST /workspace/invites/accept`
- [ ] Expired (7 days), revoked or used invites → 404 on accept; an email invite accepted by a different email → 403; accepting when already a member → 409
- [ ] The invite cannot grant `owner`

**Files:** `internal/modules/workspace/`

## [ ] M1-05 platform: Redis and rate limiting
**SRS:** AUTH-06, NFR-04, NFR-07 · **Arch:** 3.2 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

**Acceptance criteria**
- [ ] `platform/redis`: `Open(ctx, url) (*redis.Client, error)` with ping (go-redis v9)
- [ ] `platform/ratelimit`: `Limiter` interface `Allow(ctx, key string, limit int, window time.Duration) (bool, error)` with a Redis implementation (fixed window with `INCR` + `EXPIRE` in one pipeline, or sliding window, documented) and an in-memory one
- [ ] Both implementations pass the same test suite; Redis tests use testcontainers (`redis:7-alpine`)

**Files:** `internal/platform/redis/`, `internal/platform/ratelimit/`

## [ ] M1-06 web: sign-in, session and workspaces
**SRS:** AUTH-01..05, 07, WSP-01, 02, 04, 05 · **Arch:** 13 · **Tier:** low (Sonnet) · **Size:** L (split into commits) · **Depends on:** none (builds against the API table in `tasks/plan.md`)

**Acceptance criteria**
- [ ] `packages/core`: a `Session` with a pluggable `TokenStore` (web uses `localStorage`, no DOM types in core); the client sends `Authorization` and `X-Workspace-ID`, refreshes once on 401 with a single in-flight refresh shared by concurrent requests, and signs out when refresh fails; typed calls for every M1 endpoint (hand-written types in `src/api/m1.ts` until checkpoint B)
- [ ] Web pages: register, login, profile (name, language, time zone), devices (revoke), create workspace, workspace switcher, members (change role, deactivate, remove), invites (create link, copy, revoke), accept invite (`/invite/:token`)
- [ ] Thai and English strings for every new label; accessible forms (labels, errors announced)
- [ ] Tests: vitest for refresh single-flight and sign-out; component tests for login and members with mocked fetch
- [ ] `npm run build && npm test && npm run lint` pass from the root

**Files:** `apps/web/`, `packages/core/`, `package-lock.json`

## [ ] M1-07 Wiring, isolation and acceptance (wave 2)
**SRS:** NFR-06, 16.3 M1 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** checkpoint B

Details are written at checkpoint B.

## Checkpoint B: after M1 wave 1 (planner)
- [ ] Branches merged; auth and workspace registered in `cmd/server` and `cmd/migrate`; Redis limiter wired
- [ ] OpenAPI regenerated; all checks pass; CI green

## Checkpoint M1: SRS 16.3 acceptance (planner)
- [ ] Two users sign up, one creates a workspace and invites the other, who joins
- [ ] A user outside the workspace gets 403
- [ ] A rotated refresh token cannot be reused
