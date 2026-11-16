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

## [x] M1-01 auth: register, login, access tokens, profile
**SRS:** AUTH-01, 02, 06, 07 · **Arch:** 5.3, 8, 12 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** none

**Acceptance criteria**
- [ ] Tables `auth_users`, `auth_devices`, `auth_refresh_tokens` per SRS 13 (UUIDv7 IDs, `timestamptz`), email unique case-insensitively
- [ ] Domain: email normalised (trim, lower-case), password 8–128 chars, argon2id hashing with parameters documented in code, locale `th|en`, timezone validated with `time.LoadLocation`
- [ ] `POST /auth/register`, `POST /auth/login`, `GET /auth/me`, `PATCH /auth/me` as in the plan; duplicate email → 409; wrong email or password → the same 401
- [ ] Login limit 5 per minute per email and per IP → 429, through a `usecase` port `LoginLimiter` with an in-memory adapter (the Redis one comes from M1-05)
- [ ] auth provides `app.Authenticator` (JWT verify + device not revoked) and `auth.Directory` in `Provide`
- [ ] Tests: domain unit tests; API tests for success, invalid input, wrong password, duplicate email, rate limit, expired and tampered JWT

**Files:** `internal/modules/auth/` (except `api.go`, which the planner owns)

## [x] M1-02 auth: refresh rotation, logout, devices
**SRS:** AUTH-03, 04, 05 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** M1-01 (same session)

**Acceptance criteria**
- [ ] `POST /auth/refresh` rotates: the old refresh token is revoked and a new pair issued, in one transaction
- [ ] Reusing a revoked refresh token → 401 and every token of that device is revoked (test it)
- [ ] `POST /auth/logout` revokes the current device, or all the user's devices with `all_devices: true`; access tokens of revoked devices fail `Authenticate` at once
- [ ] `GET /auth/devices` (with `current`) and `DELETE /auth/devices/{id}`; another user's device → 404
- [ ] `auth_devices.last_seen_at` updates on login and refresh

**Files:** `internal/modules/auth/`

## [x] M1-03 workspace: workspaces, members, roles, `Can`
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

## [x] M1-04 workspace: invites
**SRS:** WSP-02 · **Tier:** low (Sonnet) · **Size:** M · **Depends on:** M1-03 (same session)

**Acceptance criteria**
- [ ] Table `workspace_invites` per SRS 13; tokens are 32 random bytes, only the hash is stored, the raw token is returned once on create
- [ ] `POST`, `GET`, `DELETE /workspace/invites` (admin and owner only) and `POST /workspace/invites/accept`
- [ ] Expired (7 days), revoked or used invites → 404 on accept; an email invite accepted by a different email → 403; accepting when already a member → 409
- [ ] The invite cannot grant `owner`

**Files:** `internal/modules/workspace/`

## [x] M1-05 platform: Redis and rate limiting
**SRS:** AUTH-06, NFR-04, NFR-07 · **Arch:** 3.2 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

**Acceptance criteria**
- [ ] `platform/redis`: `Open(ctx, url) (*redis.Client, error)` with ping (go-redis v9)
- [ ] `platform/ratelimit`: `Limiter` interface `Allow(ctx, key string, limit int, window time.Duration) (bool, error)` with a Redis implementation (fixed window with `INCR` + `EXPIRE` in one pipeline, or sliding window, documented) and an in-memory one
- [ ] Both implementations pass the same test suite; Redis tests use testcontainers (`redis:7-alpine`)

**Files:** `internal/platform/redis/`, `internal/platform/ratelimit/`

## [x] M1-06 web: sign-in, session and workspaces
**SRS:** AUTH-01..05, 07, WSP-01, 02, 04, 05 · **Arch:** 13 · **Tier:** low (Sonnet) · **Size:** L (split into commits) · **Depends on:** none (builds against the API table in `tasks/plan.md`)

**Acceptance criteria**
- [ ] `packages/core`: a `Session` with a pluggable `TokenStore` (web uses `localStorage`, no DOM types in core); the client sends `Authorization` and `X-Workspace-ID`, refreshes once on 401 with a single in-flight refresh shared by concurrent requests, and signs out when refresh fails; typed calls for every M1 endpoint (hand-written types in `src/api/m1.ts` until checkpoint B)
- [ ] Web pages: register, login, profile (name, language, time zone), devices (revoke), create workspace, workspace switcher, members (change role, deactivate, remove), invites (create link, copy, revoke), accept invite (`/invite/:token`)
- [ ] Thai and English strings for every new label; accessible forms (labels, errors announced)
- [ ] Tests: vitest for refresh single-flight and sign-out; component tests for login and members with mocked fetch
- [ ] `npm run build && npm test && npm run lint` pass from the root

**Files:** `apps/web/`, `packages/core/`, `package-lock.json`

## [x] M1-07 Redis login limiter, generated web types, acceptance test (wave 2)
**SRS:** AUTH-06, NFR-04, NFR-06, NFR-13, 16.3 M1 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** checkpoint B

**Acceptance criteria**
- [ ] Redis login limiter: an auth adapter implementing `usecase.LoginLimiter` on Redis (`Blocked` reads the count, `Fail` does `INCR` + `PEXPIRE … NX` in one pipeline, 5 per minute). `auth.Init` uses it when `d.Platform["redis"]` holds a `*redis.Client`, otherwise the in-memory one. testcontainers test
- [ ] `cmd/server`: opens Redis from `REDIS_URL` (startup fails with a clear message if unreachable), adds it to `Deps.Platform["redis"]` and to `/readyz` as check `redis`, closes it on shutdown
- [ ] `packages/core`: M1 request and response types come from the generated `schema.d.ts` (keep `m1.ts` only as re-exports and the typed calls); `npm run build` fails if the server shape drifts
- [ ] `cmd/server` acceptance test (testcontainers Postgres and Redis, real modules, `httptest` server) that runs the SRS 16.3 M1 flow: two users register, one creates a workspace and invites the other by email, the other accepts and lists members; a third user gets 403 on that workspace for members, invites and member changes; a rotated refresh token is rejected on reuse
- [ ] All Go and web checks pass, and `make openapi` leaves no diff

**Files:** `internal/modules/auth/adapter/redis/`, `internal/modules/auth/module.go`, `cmd/server/`, `packages/core/src/api/`, `go.mod`, `go.sum`

## Checkpoint B: after M1 wave 1 (planner)
- [x] Branches merged; auth and workspace registered in `cmd/server` and `cmd/migrate` (Redis limiter moved to M1-07)
- [x] OpenAPI regenerated (18 operations); Go and web checks pass; CI pending on push
- [x] Live run with curl: register, duplicate 409, create workspace, outsider 403, email invite and accept, member list with names, refresh rotation, reuse 401 and device revoked, 6th bad login 429

Planner notes from checkpoint B:
- `cmd/migrate` now always runs core module migrations; before, auth and workspace tables were never created.
- `cmd/server` passes `jwt_secret` in `Deps.Platform`; `server openapi` passes a placeholder secret.
- MinIO: the init container raced the Bitnami setup restart. It is replaced by `MINIO_DEFAULT_BUCKETS`.
- Accepted worker deviations: refresh reuse also revokes the device row; the login limiter counts failures only; client IP is the socket address (no `X-Forwarded-For` until a trusted-proxy setting exists); `ActionRegistry` lets later modules add actions to `Can`.

## Checkpoint M1: SRS 16.3 acceptance (planner)
- [x] Two users sign up, one creates a workspace and invites the other, who joins (`cmd/server/acceptance_test.go`, and a manual curl run at checkpoint B)
- [x] A user outside the workspace gets 403 (members, invites, member changes)
- [x] A rotated refresh token cannot be reused
- [ ] The M1 web pages checked in a real browser against the running server

Planner notes from checkpoint M1:
- The web now offers the `manager` role (SRS section 2, WSP-03). Invites still cannot grant owner or manager; managers are assigned after joining.
- `REDIS_URL` is required at startup; the login limiter uses Redis when present.
- Before v0.1: require a `JWT_SECRET` of at least 32 bytes outside dev, and decide on a trusted-proxy setting for client IPs.

---

# M2: realtime chat

Read the M2 section of `tasks/plan.md` first: it fixes the WebSocket protocol, the chat API, the rules and the new contracts (`app.Realtime`, `app.ObjectStorage`, `app.CardRegistry`).
Same rules as M1: do not edit `cmd/server`, `cmd/migrate`, `internal/app`, another module, `tasks/` or `AGENTS.md`. Test through your own package with fakes for the contracts and real Postgres or Redis via testcontainers.

## [x] M2-01 platform/realtime: WebSocket hub
**SRS:** CHAT-07, CHAT-17 (presence only), NFR-02, 03, 04 · **Arch:** 9.2 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** none

**Acceptance criteria**
- [ ] `realtime.NewHub(logger, auth app.Authenticator-lookup via *app.Services, redis *redis.Client)` implements `app.Realtime`; `hub.Mount(r chi.Router)` serves `GET /api/v1/ws` (use `github.com/coder/websocket`)
- [ ] Handshake, envelope, ping/pong, 60 s idle timeout, 64 KiB inbound limit and close codes exactly as in the plan; `Handle` dispatches registered client types and rejects unknown ones without closing
- [ ] One reader and one writer goroutine per connection, bounded queue of 256, slow clients disconnected; graceful `Close(ctx)` for shutdown
- [ ] Redis fan-out (`rt:users`, `rt:control`) so `SendToUsers` and `DisconnectDevice` work across two hubs on the same Redis; presence keys with 60 s TTL and `Online`
- [ ] Tests with `httptest` servers and real WebSocket clients: handshake success and failure, ping, idle timeout (short in tests), slow-client drop, two hubs sharing a Redis container deliver to each other's clients, `DisconnectDevice` across hubs, `go test -race`
- [ ] A benchmark or test that holds 1,000 concurrent connections on one hub (NFR-03), skipped with `-short`

**Files:** `internal/platform/realtime/`

## [x] M2-02 platform/storage: presigned URLs
**SRS:** CHAT-14, NFR-07 · **Arch:** 3.2 · **Tier:** low (Sonnet) · **Size:** S · **Depends on:** none

**Acceptance criteria**
- [ ] `storage.New(ctx, cfg)` (endpoint, bucket, access key, secret, region default `us-east-1`, path-style for MinIO) implements `app.ObjectStorage` with `github.com/minio/minio-go/v7`
- [ ] `PresignPut` signs content type and length, so the upload must match them; `PresignGet` sets `Content-Disposition: attachment; filename*=UTF-8''...` (Thai file names work); `Stat` maps a missing key to `app.ErrNotFound`
- [ ] testcontainers test against `bitnamilegacy/minio` (or `minio/minio` if it pulls): upload through the presigned PUT with `net/http`, a mismatched size is rejected, download through the presigned GET, Stat before and after

**Files:** `internal/platform/storage/`

## [x] M2-03 chat: channels, messages, threads, read state
**SRS:** CHAT-01..13 · **Arch:** 5.3, 8, 9.1 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** none

**Acceptance criteria**
- [ ] Tables `chat_channels`, `chat_channel_members`, `chat_messages` per SRS 13 with the unique keys in the plan; migrations exported as `chat.Migrations`
- [ ] Channel endpoints from the plan (create, DM, list joined and public, get, patch, archive, join, leave, members), with the visibility rules (private and DM → 404 for non-members; guests only see channels they were added to)
- [ ] Message endpoints: send with `seq` and `client_msg_id` exactly as in the plan's rules (test a concurrent duplicate send), `after_seq` and `before_seq` pagination, edit and soft delete, one-level threads with `reply_count`, mentions parsing, 4,000-character limit
- [ ] Read state: `POST .../read` (only moves forward), `unread_count` and `mention_count` per channel
- [ ] After commit, sends `message.created`, `message.updated`, `message.deleted`, `channel.read` (to the reader's own devices) and `channel.updated` through `Deps.Realtime` when it is not nil; publishes `chat.message_created`; subscribes to `workspace.member_removed`
- [ ] Provides a stub `app.CardRegistry` whose methods return an error wrapping `app.ErrInvalid` ("cards arrive in M2-05"), so the contract is wired
- [ ] Tests: domain unit tests; API tests for every visibility rule, NFR-06 (workspace A cannot see workspace B's channels), ordering under 20 concurrent senders (seqs are 1..20 with no gaps), duplicate send, catch-up after a gap; a fake `app.Realtime` records deliveries

**Files:** `internal/modules/chat/` (create the module, including `api.go` and `events.go`)

## [x] M2-04 packages/core: realtime client and chat sync engine
**SRS:** CHAT-05..09, 12, NFR-09 · **Arch:** 9.1, 13 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** none (protocol and shapes in the plan)

**Acceptance criteria**
- [ ] `RealtimeClient` with an injected WebSocket factory (no DOM types in core): auth handshake with the current access token, ping every 30 s, reconnect with exponential backoff and jitter (1 s → 30 s), re-auth after token refresh, typed `on(type, handler)`
- [ ] `ChatStore` (framework-free, `subscribe`/`getState` for `useSyncExternalStore`): per channel, messages kept sorted by `seq`, deduplicated by `id` and `client_msg_id`; optimistic send with a generated UUID `client_msg_id`, marked pending, then failed or confirmed; retry resends the same `client_msg_id`
- [ ] Gap handling: an incoming `seq` greater than `last + 1`, or a reconnect, triggers `GET messages?after_seq=` until `has_more` is false
- [ ] Applies `message.updated`, `message.deleted`, `channel.read` and `card.updated`; unread counts per channel
- [ ] Hand-written types for the M2 shapes in `src/api/m2.ts` until checkpoint C
- [ ] vitest with a fake socket and fake fetch: out-of-order delivery, duplicate delivery, reconnect catch-up, optimistic send confirmed by both the POST response and the WebSocket echo (one message, not two), retry after failure keeps the same `client_msg_id`

**Files:** `packages/core/`

## [x] M2-05 chat: uploads, reactions, cards, typing, presence (wave 2)
**SRS:** CHAT-14, 15, 17, 18, PLT-11 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** checkpoint C

**Acceptance criteria**
- [x] Tables `chat_attachments` and `chat_reactions` per SRS 13. `POST /chat/uploads` and `GET /chat/attachments/{id}` as in the plan through `Deps.Storage` (50 MB cap, key `chat/{workspace_id}/{attachment_id}`, 15-minute URLs); sending with `attachment_ids` checks `Stat`, owner and channel, and allows an empty body; `Message.attachments` is filled, with `is_image` for `image/*`
- [x] Reactions: add and remove (idempotent), `Message.reactions` with `count` and `me`, sends `message.updated`
- [x] Real `app.CardRegistry` replacing the stub: `Register` (kind must start with its module name), `Post` (fills `kind`, `payload`, `fallback_text`; sends `message.created`), `Update` (only for registered kinds; sends `card.updated`), `DirectChannel` (reuses the DM rules). An unregistered kind shown to clients keeps `kind` and `payload`, and clients fall back to `fallback_text` (PLT-11)
- [x] Typing: `Deps.Realtime.Handle("typing", …)` in `Init` when Realtime is set; checks membership, fans out `typing` to other members, at most one per user per channel per 3 s
- [x] Presence: `presence.changed` is outside chat's scope; skip it and note it
- [x] Tests for each, including NFR-06 for attachments (another workspace's attachment ID → 404), a card from a test-only module registered in the test, and `go test -race`

**Files:** `internal/modules/chat/` (its `api.go` may gain exported helpers), `go.mod`, `go.sum`

## [x] M2-06 web: chat UI and core alignment (wave 2)
**SRS:** CHAT-01..13, 15, 17, 18 · **Tier:** low (Sonnet, effort high) · **Size:** L (split into commits) · **Depends on:** checkpoint C

**Acceptance criteria**
- [x] Align `packages/core` with the canonical event payloads and page order in `tasks/plan.md` ("Event payloads"): fix `realtime/events.ts` and the store (history pages are ascending; `message.deleted` carries `message`; `channel.read` carries counts; `channel.updated` is flat), handle `error` frames and the close codes; replace hand-written M2 types with ones from the generated `schema.d.ts` (`make openapi` first), keeping `m2.ts` as re-exports and calls
- [x] Web: sidebar of joined channels and DMs with unread and mention badges, browse and join public channels, create channel, start a DM; channel view with history loaded upward (`before_seq`), composer (Enter sends, Shift+Enter newline, 4,000-character counter), optimistic send with pending and failed states and retry, edit and delete own messages, thread panel, `@` mention picker inserting `<@user_id>`, rendered mentions, markdown subset (bold, italic, code, links) with no raw HTML, mark read when the newest message is visible, typing indicator, reaction display and toggle, card messages rendered as `fallback_text` unless the kind is known
- [x] The realtime client connects once per signed-in session, re-auths after token refresh, and `store.reset()` runs on workspace switch
- [x] Thai and English strings for every new label; keyboard and screen-reader friendly (message list is a log region)
- [x] Tests: component tests for sending (pending → confirmed), receiving over a fake socket, failed send and retry, thread open, mention picker; `npm run build && npm test && npm run lint` pass
- [ ] Manual: run the server and `API_URL=... npm run dev -w apps/web -- --port 5174`, and check two browser sessions exchange messages (report what you could and could not check)

**Files:** `apps/web/`, `packages/core/`, `package-lock.json`

## [x] M2-07 AUTH-08 and M2 acceptance test (wave 3)
**SRS:** AUTH-08, 16.3 M2 · **Tier:** low (Sonnet, effort high) · **Size:** M · **Depends on:** M2-05, M2-06 (wave 2 merged)

The hub already authenticates the `auth` frame with the access token and closes a revoked device's sockets through `Hub.DisconnectDevice` (tested with Redis fan-out). What is missing is the trigger from auth and an end-to-end acceptance test for M2.

**Acceptance criteria**
- [x] auth publishes `auth.device_revoked` (its own `events.go`, payload `{user_id, device_id}`) on logout, "revoke this device" and "revoke all other devices", through `Deps.EventBus` inside the transaction-free path after the write commits; a replay of a revoke publishes nothing extra
- [x] `cmd/server` subscribes to `auth.device_revoked` and calls `hub.DisconnectDevice(ctx, deviceID)`; a revoked device's WebSocket closes with code 4401 on this and on another instance (Redis fan-out), and its next `auth` frame is rejected
- [x] An expired or rotated-out access token is rejected at the `auth` frame; chat, workspace and auth HTTP calls with it return 401 (they already do; assert it)
- [x] New package `internal/acceptance` boots the real registry (auth, workspace, chat) with the real hub over `httptest` and testcontainers postgres and redis, then asserts the three SRS 16.3 M2 outcomes: two users see each other's message over WebSocket within 1 s; after closing and reconnecting with `after_seq`, messages are complete and in order; resending the same `client_msg_id` creates one message and one `message.created`. Skip with a clear message when Docker is unavailable
- [x] `go build ./... && go vet ./... && go test ./...` and `go test -race ./internal/acceptance/` pass; CI green

**Files:** `internal/modules/auth/` (`api.go`, `events.go`, `usecase/`), `internal/acceptance/` (new), `apps/server/cmd/server/`, `go.mod`, `go.sum`

Planner notes from M2-07 review:
- Worker: DeepSeek V4.1 Flash (OpenCode). Accepted deviations: `Hub.DisconnectUser` in `platform/realtime` (logout of every device publishes an event with an empty `device_id`, and `cmd/server` closes all of the user's sockets through a type assertion, since `app.Realtime` only knows devices); publishing goes through a `usecase.Events` port; `Logout` reads active devices first so a replay publishes nothing.
- Added on review: refresh reuse (AUTH-03) also publishes `auth.device_revoked`, so a stolen-token detection closes the device's open sockets.
- Two-browser check (2026-11-16, Playwright Chromium against the dev stack): revoking a device from another browser closes its socket with 4401 and the web client lands on /login while the revoker stays signed in; "sign out every device" does the same for the other browser and the own one.

## Checkpoint C: after M2 wave 1 (planner)
- [x] Branches merged; chat registered; `cmd/server` creates the hub and storage, mounts `/api/v1/ws`, fills `Deps.Realtime` and `Deps.Storage`, closes the hub on shutdown
- [x] OpenAPI regenerated (32 operations); all checks pass; CI pending on push
- [x] Live run: two users over WebSocket, message delivered in 5 ms to both, resend with the same `client_msg_id` → 200 with the same message and no second event, unread 1 and mention 1, catch-up `after_seq=0`

Planner notes from checkpoint C:
- Bug found: chat and workspace both registered a Huma schema named `Member`, which panics at startup when both are enabled. Chat's is now `ChannelMember`, and `TestOpenAPIWithAllModules` builds the spec from every real module.
- The server's event payloads are canonical (plan, "Event payloads"); `packages/core` assumed other shapes and descending history, fixed in M2-06.
- `apps/web` gained `@types/node` because `vite.config.ts` reads `API_URL`.
- Presigned URLs use the host in `S3_ENDPOINT`, so production needs a browser-reachable endpoint there.
- Known gap: edits and deletes missed while offline are not recovered by `after_seq` catch-up. Revisit in M6 (an `updated_since` query).

## Checkpoint C2: after M2 wave 2 (planner)
- [x] Branches `task/m2-05-chat-extras` and `task/m2-06-web-chat` merged into `main` without conflicts; `make openapi` regenerates nothing (32 operations)
- [x] `go build ./... && go vet ./... && go test ./...` and `go test -race ./internal/modules/chat/...` pass; `npm run build`, `npm test` (128 web + 118 core), `npm run lint` pass
- [x] Live API run against postgres, redis and minio: 19/19 checks (upload presign → PUT → attach → presigned download returns the bytes, NFR-06 another workspace 404, used file refused, reaction idempotent with per-viewer `me`, `message.created` with attachment and `message.updated` with reaction over WebSocket, typing fan-out and the 3 s drop)

Planner notes from checkpoint C2:
- Attachments are bound to the channel at presign time, so a file uploaded for one channel cannot be posted to another; `GET /chat/attachments/{id}` serves members of that channel and the uploader's not-yet-sent files, and 404s everything else.
- `me` on reactions is computed per recipient: a message with reactions goes out once per member on the hub, not once for the channel.
- Typing rate limiting is in-process; with several server instances a user can exceed one frame per 3 s per channel. Harmless frames, revisit only if presence/typing moves to Redis.
- Presence stays out of chat (`presence.changed` comes from the realtime hub), as the task said.
- The two-browser manual check of the web UI is still open because this session had no browser attached; code-level behaviour is covered by component tests and the API live run. Checkpoint M2 covers it.

## Checkpoint M2: SRS 16.3 acceptance (planner)
- [ ] Two users in two browsers see each other's messages within 1 second
- [ ] After a network drop and reconnect, messages are complete and in order
- [ ] Re-sending the same message creates no duplicate
