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
- [x] Two users in two browsers see each other's messages within 1 second
- [x] After a network drop and reconnect, messages are complete and in order
- [x] Re-sending the same message creates no duplicate

Checked at checkpoint D (2026-11-16) with Playwright Chromium, two users in two browsers against the dev stack: delivery in 53 ms; the receiver went offline with its socket closed, missed 3 messages, came back and showed all 3 in order with no duplicate. The server side is also covered by `internal/acceptance`.

# M3: notifications and the job queue

Plan: `tasks/plan.md` section "M3". Contracts are fixed in M3-00; workers do not change `internal/app`. Wave 1 tasks own disjoint files and run in parallel.

## [x] M3-00 contracts (planner)
- [x] `app.Notification.Key`, `app.NotificationRules` and `app.NotificationRuleSource`; `cmd/server` passes `app.NewNotificationRules()` as `Deps.Notifications`
- [x] `app.Jobs`, `app.JobHandler`, `app.JobOptions`, `app.EnqueueOptions`; `Deps.Jobs`
- [x] notify moves from `app.CoreModules` to `app.DefaultModules`; `config.ParseModules` is shared by `cmd/server` and `cmd/migrate`; `.env.example` sets `MODULES_ENABLED=notify`

## [x] M3-01 platform/jobs: River queue (wave 1)
**SRS:** PLT-09, NFR-11 · **Architecture:** sections 7, 10; ADR 0006 · **Size:** M · **Depends on:** M3-00

Implement `app.Jobs` on River in `internal/platform/jobs`, as described in the plan ("Job queue").

**Acceptance criteria**
- [x] `jobs.New(pool, logger)` returns a value implementing `app.Jobs` without I/O; `Start(ctx)` starts the River client and the no-worker sweeper; `Stop(ctx)` waits for running jobs. `Handle` or `Every` after `Start` returns/logs an error instead of being silently ignored
- [x] Enqueue inside `TxManager.Do` joins the transaction: rolled back → no job; committed → the handler runs with the same JSON args. Outside a transaction it inserts directly
- [x] A handler error is retried up to `MaxAttempts` (default 5); a panic counts as an error
- [x] Jobs whose kind has no handler are `cancelled` at start and within one sweep interval (configurable in tests)
- [x] `Every` runs on one instance only (two clients on one database, one enqueue per interval)
- [x] River migrations run as part of the platform migration set in `cmd/migrate`; `cmd/server` builds `jobs.New` into `Deps.Jobs`, starts it after `reg.Start` and stops it after HTTP drains
- [x] Tests use testcontainers postgres and skip clearly without Docker; `go build ./... && go vet ./... && go test ./...` and `go test -race ./internal/platform/jobs/` pass

**Files:** `internal/platform/jobs/` (new files), `internal/platform/database/tx.go` (export `TxFromContext`) and its test, `internal/platform/database/migrations/` (River set), `cmd/migrate/`, `cmd/server/main.go`, `go.mod`, `go.sum`

## [x] M3-02 notify module (wave 1)
**SRS:** NOTI-01..05 · **Architecture:** section 6.2 · **Size:** L (split into commits: storage, rules dispatch, API, realtime, prune) · **Depends on:** M3-00

Build `internal/modules/notify` as in the plan ("notify module"), following the chat module's layout (`domain`, `usecase`, `adapter/postgres` with sqlc, `adapter/http`, `migrations`, `module.go`, `api.go`, `events.go`).

**Acceptance criteria**
- [x] Migration creates `notify_notifications` with the unique `(user_id, key)` rule; repository tests run on testcontainers postgres
- [x] In `Subscribe`, notify reads `Deps.Notifications` as `app.NotificationRuleSource` and subscribes one handler per event name; rules run, invalid notifications are dropped, rows are stored in one transaction before delivery, a repeated key stores nothing and sends nothing
- [x] A rule that errors or panics is logged and the other rules still run
- [x] After commit, each inserted row goes out as `notification.created` with `unread_count`, only to that user; `Deps.Realtime == nil` is tolerated
- [x] The four endpoints in the plan work, scoped to caller and workspace; marking another user's notification is 404; read and read-all send `notification.read` to the user's devices
- [x] `notify.prune` registered with `Jobs.Every` (guarded when `Deps.Jobs` is nil) deletes read rows older than 90 days
- [x] Unit tests for the use cases with fakes; handler tests with `humatest`; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/notify/` (new). Do not edit `cmd/server` or `cmd/migrate`; the planner registers the module at checkpoint D.

## [x] M3-03 chat: notification rules (wave 1)
**SRS:** NOTI-06, NOTI-01 · **Size:** M · **Depends on:** M3-00

Register chat's rule on `chat.message_created` with `Deps.Notifications.On` in chat's `Init`, as in the plan ("chat notification rules").

**Acceptance criteria**
- [x] Mention, `<!channel>`, DM and thread reply each produce the right kind for the right users; one notification per user per message by priority mention > DM > thread; never the sender or a non-member
- [x] Title, body (plain text, ≤ 140 characters, `<@id>` as `@name`), link, data and `Key = chat.message:{id}` as in the plan
- [x] The rule reads only chat's own tables (thread participants, channel members, names chat already resolves)
- [x] With `app.NoopNotificationRegistry` chat behaves exactly as before (existing tests unchanged and passing)
- [x] Use case tests for every case above; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/chat/` (`module.go`, `usecase/`, `adapter/postgres/` queries if a thread-participant query is needed)

## [x] M3-04 web: notification bell (wave 1)
**SRS:** NOTI-03, NOTI-04, NOTI-06 · **Size:** M · **Depends on:** M3-00 (API in the plan; types are hand-written until checkpoint D regenerates `schema.d.ts`)

**Acceptance criteria**
- [x] `packages/core`: notify API client (list, unread count, read, read-all), `notification.created` and `notification.read` in the realtime event map, and a notification store that loads the count on connect/`ready`, applies both events, and refetches the count after reconnect
- [x] `apps/web`: a bell in the shell header with the unread count (hidden at 0, `99+` cap, accessible label), a panel listing notifications newest first with "load more", click marks read and navigates to `link`, "mark all as read"
- [x] Known kinds (`chat.mention`, `chat.dm`, `chat.thread_reply`) render with Thai and English strings from `data`; unknown kinds use `title` and `body`
- [x] The bell is hidden when `/meta` does not list notify
- [x] Core store tests and web component tests; `npm run build`, `npm test`, `npm run lint` pass

**Files:** `packages/core/src/` (new `notify/` folder, `api/`, `realtime/events.ts`, `index.ts`), `apps/web/src/` (new `notify/` folder, `pages/Shell.tsx`, `i18n/`)

## Checkpoint D: after M3 wave 1 (planner)
- [x] Branches merged; notify registered in `cmd/server` and `cmd/migrate`; OpenAPI and `schema.d.ts` regenerated
- [x] All checks pass; CI green
- [x] Live run: mention over WebSocket raises the bell; reload keeps the count; `MODULES_ENABLED=` (notify off) and chat still works, `/notify/*` 404

Planner notes from checkpoint D:
- Workers: DeepSeek V4.1 Flash (OpenCode), four in parallel; branches merged without conflicts.
- Two-browser run (Playwright Chromium): a mention raised the other user's bell 0 → 1 in 42 ms, the count survived a reload, the panel read "กล่าวถึงคุณ · Somchai ใน #general", and "mark all as read" on one device cleared the bell on the user's other device. With `MODULES_ENABLED=` the server listed auth, chat and workspace only, chat delivered in 55 ms with reconnect catch-up, the bell was gone and `/notify/unread-count` answered 404.
- Changed on review: notify stores on `context.WithoutCancel`, since the bus runs handlers on the publisher's request context (NOTI-02); chat's rule adds `sender_name` and `channel_name` to `data` so the web renders who and where in the reader's language; notify's Huma outputs are `NotificationsOutputBody` and `UnreadCountOutputBody` so workspace's `ListOutputBody` keeps its name; `packages/core` M3 types now come from the generated schema.
- Accepted deviations: River migrations run through `rivermigrate` from `cmd/migrate` (not a copied goose file); enqueuing a kind with no handler on this server is rejected by River, so the no-worker sweep handles jobs left by another module set; a reserved `platform.noop` worker lets the queue start when no module has a handler; chat's thread-participant query is hand-written in `adapter/postgres/extras.go`; bell styles live in `apps/web/src/notify/notify.css`.
- Open: the bell panel lists read and unread together (read dimmed); `?unread=true` exists if we want an unread filter.

## [x] M3-05 M3 acceptance test (wave 2)
**SRS:** 16.3 M3, NOTI-05, PLT-09 · **Size:** M · **Depends on:** checkpoint D

**Acceptance criteria**
- [x] `internal/acceptance` gains an M3 test with the real registry including notify and jobs: a mention reaches the mentioned user as `notification.created` within 1 s; `GET /notify/unread-count` after reconnect is still correct; read-all on one device sends `notification.read` to the other
- [x] The same suite with notify disabled: sending, mentions and DMs work, no notification rows exist, `/notify/*` is 404
- [x] A job enqueued in a rolled-back transaction never runs; a job of a kind with no handler is cancelled
- [x] `go test -race ./internal/acceptance/` passes; CI green

**Files:** `internal/acceptance/`

Planner notes from M3-05 review:
- The DeepSeek Flash worker stopped on its free quota mid-task; Claude Sonnet 5.5 finished it, reviewing the half-done code. It found and fixed a flaky design (a second job queue next to the server's would have been swept as "no handler") and a typed-nil `Deps.Jobs`.
- 16 subtests across `TestSRSM2Acceptance`, `TestSRSM3Acceptance`, `TestSRSM3NotifyDisabled` and `TestSRSM3Jobs` pass under `-race`; the worker ran the suite 3 times. Mention → `notification.created` took about 150 µs. A mutation check (notify enabled in the notify-off test) failed all three subtests, so the assertions bite.
- Fixed on review: one errcheck finding that would have failed CI lint.

## Checkpoint M3: SRS 16.3 acceptance (planner)
- [x] Mentioned in a browser → the bell count rises at once
- [x] Reloading the page keeps the right count
- [x] With notify turned off, chat works normally

All three checked in two browsers at checkpoint D (see its notes); M3-05 adds the same outcomes to `internal/acceptance`.

# M4: tasks, approvals, chat extensions, groups and the outbox

Plan: `tasks/plan.md` section "M4" (APIs, payloads, events, card rules; canonical). Contracts are fixed in M4-00; workers do not change `internal/app` or another module's `api.go`/`events.go`. Wave 1 tasks own disjoint files and run in parallel.

## [x] M4-00 contracts (planner)
- [x] `app.EventBus.Declare` and the outbox rule in its doc; the in-memory bus implements `Declare` as a no-op
- [x] `app.ChatExtensions`, `app.Command`, `app.CommandInput`, `app.CommandResult`, `app.CommandHandler`, `app.MessageAction`, `app.ChannelAccess`
- [x] `workspace.GroupDirectory`
- [x] Plan section M4 and these tasks

## [x] M4-01 platform: transactional outbox (wave 1)
**SRS:** PLT-10 · **Architecture:** section 6.1; ADR 0003 · **Size:** M · **Depends on:** M4-00

**Acceptance criteria**
- [x] `eventbus` keeps today's in-memory behaviour outside a transaction; inside `TxManager.Do` `Publish` writes `platform_outbox_events` and `pg_notify` in that transaction, and an undeclared event returns an error
- [x] A relay (`LISTEN` plus a 2 s poll) claims rows with `FOR UPDATE SKIP LOCKED`, decodes them into the declared types, runs local handlers and marks them published; rolled back → never delivered; committed → delivered within 1 s; two relays on one database deliver each event once
- [x] A crash between handling and marking redelivers (simulate by failing the claim transaction); handler errors and panics are logged, as today
- [x] Job `platform.outbox_prune` (24 h) deletes published rows older than 7 days, when `Deps.Jobs` exists
- [x] `cmd/server` builds the bus with the pool, starts the relay after `reg.Start` and stops it after HTTP drains; with no database (tests, `server openapi`) it stays in memory
- [x] testcontainers tests that run, not skip; `go build ./... && go vet ./... && go test ./...` and `go test -race ./internal/platform/eventbus/` pass

**Files:** `internal/platform/eventbus/`, `cmd/server/main.go`, a migration in `internal/platform/database/migrations/platform/` only if an index is needed

## [x] M4-02 workspace: groups and managers (wave 1)
**SRS:** WSP-09 · **Size:** M · **Depends on:** M4-00

**Acceptance criteria**
- [x] Migration for `workspace_groups` and `workspace_group_members`; the six endpoints in the plan with the permissions shown (`workspace.group.manage` for owner and admin)
- [x] `PUT .../members` replaces the set in one transaction and rejects users who are not active members (400)
- [x] `GroupDirectory.ManagersOf` provided through `app.Services`, ordered as the contract says, excluding the user
- [x] Removing a member from the workspace removes them from its groups
- [x] Use case, handler and repository tests; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/workspace/` except `api.go` and `events.go`

## [x] M4-03 chat: slash commands, message actions and channel access (wave 1)
**SRS:** CHAT-16 · **Size:** M · **Depends on:** M4-00

**Acceptance criteria**
- [x] chat provides `app.ChatExtensions` and `app.ChannelAccess` in `Provide`
- [x] `GET /chat/extensions` and `POST /chat/channels/{id}/commands` as in the plan; the handler gets the caller's `Principal`; `ErrInvalid` → 400, `ErrForbidden` → 403, unknown command → 404, unreadable channel → 404
- [x] A duplicate command name panics at registration; names are validated (lower case letters, digits, `-`, 1-32)
- [x] `ChannelAccess.CanRead` matches the existing channel read rules (public for active members, private and DM for members, archived still readable)
- [x] Tests for all of the above; existing chat tests pass unchanged; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/chat/` except `api.go` and `events.go`

## [x] M4-04 task module (wave 1)
**SRS:** TASK-01..08 · **Size:** L (commit in slices: storage and create, list and get, edit and events, card handlers, rules and due scan, command and action) · **Depends on:** M4-00

Build `internal/modules/task` like notify and chat (`domain`, `usecase`, `adapter/postgres` with sqlc via `go tool sqlc`, `adapter/http`, `migrations`, `module.go`, `api.go`, `events.go`), exactly as the plan's "task module" and "Rules for modules that own cards".

**Acceptance criteria**
- [x] Migration, the four endpoints, filters and cursor paging; visibility through `ChannelAccess`; edit rights per TASK-05; assignee validation
- [x] Events published inside the transaction and declared; card posted and updated by the module's own handlers after commit, idempotently
- [x] Notification rules and `task.due_scan` (guarded when `Deps.Jobs` or a lookup is missing)
- [x] `/task` command and `task.create_from_message` action registered when `ChatExtensions` is available
- [x] Unit tests with fakes for every rule; handler tests with `humatest`; repository tests on testcontainers; a module test on the real registry with the in-memory bus; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/task/` (new). Do not edit `cmd/`; the planner registers the module.

## [x] M4-05 approval module (wave 1)
**SRS:** APR-01..09, 13 · **Size:** L (commit in slices: storage and submit, lists and detail, decide and cancel with history, card handlers, rules, uploads) · **Depends on:** M4-00

Build `internal/modules/approval` the same way, exactly as the plan's "approval module" and "Rules for modules that own cards".

**Acceptance criteria**
- [x] Migrations, payload validation for leave and general, default approver from `GroupDirectory` (400 `approver_required` when none or the lookup is missing)
- [x] Approve, reject (reason required) and cancel with the 403/409 rules; every action written to `approval_history`
- [x] Events in the transaction and declared; card in the requester–approver DM, updated on every decision
- [x] Notification rules; uploads and attachment downloads through `Deps.Storage`
- [x] Unit, handler, repository and module tests as for task; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/approval/` (new). Do not edit `cmd/`.

## Checkpoint E: after M4 wave 1 (planner)
- [x] Branches merged; task and approval registered in `cmd/server` and `cmd/migrate`; OpenAPI and `schema.d.ts` regenerated; no Huma schema name collisions
- [x] All checks pass; CI green
- [x] Live API run: `/task` creates a card that updates on status change; a leave request posts a card in the DM, the approver approves, a third user gets 403; notifications arrive; restart mid-flow loses no event

Planner notes from checkpoint E:
- Workers: Claude Sonnet 5.5, five in parallel; branches merged without conflicts. task and approval registered in `cmd/server` and `cmd/migrate`; OpenAPI regenerated (45 paths), no new numbered schema names; all Go tests, `golangci-lint` (0 issues) and the web build pass.
- Live run with three users over WebSocket against the dev stack, 22/22: groups (admin 201/204, member 403), `/chat/extensions`, `/task` card in 42 ms with the assignee and a `task.assigned` notification, a plain member's PATCH 403, status change → `card.updated` in 31 ms, "My tasks"; a leave request defaults to the group manager, its card reaches the approver's DM in 62 ms with `approval.requested`, the requester and a third member get 403 on approve, reject without reason 400, approve → the requester's card shows approved in 26 ms with `approval.approved`, a second decision 409, history `created,approved`, no manager → 400 `approver_required`.
- "Restart mid-flow" is covered by the outbox tests (a failed claim commit redelivers; rows inserted without a notify are found by the poll), not by killing the dev server.
- Accepted deviations: the relay filters undeclared names in the claim query so unknown rows cannot block newer ones; an undecodable declared payload is logged and marked published; publishing a type other than the declared one is an error; approval prefixes its Huma types with `Approval` to avoid schema collisions; approval events also carry `requester_id`/`approver_id`; `approver_required` is a module-specific error code in the shared error shape; `next_cursor` is an opaque string, empty on the last page.
- Known: two concurrent deliveries of one approval event can post a duplicate card (the request keeps pointing at one; logged). Uploaded approval files never attached to a request are not cleaned up yet (M6).

## [x] M4-06 web: chat extensions and tasks (wave 2)
**SRS:** CHAT-16, TASK-01..05, 07 · **Size:** M · **Depends on:** checkpoint E

**Acceptance criteria**
- [x] `packages/core`: task and extensions API from the generated schema; `card.updated` keeps the newest `version`
- [x] Composer slash-command menu and sending; message menu "Create task"; `task.item` card with status change and "Comments" opening the thread; "My tasks" page with filters
- [x] Hidden when `/meta` lacks task; Thai and English strings; component tests; `npm run build`, `npm test`, `npm run lint` pass

**Files:** `packages/core/src/` (new `task/`, `api/`), `apps/web/src/` (new `task/`, `chat/Composer.tsx`, `chat/MessageItem.tsx`, `chat/cards.tsx`, `App.tsx`, `pages/Shell.tsx`, `i18n/`)

## [x] M4-07 web: approvals and groups (wave 2)
**SRS:** APR-01..09, WSP-09 · **Size:** M · **Depends on:** checkpoint E

**Acceptance criteria**
- [x] `packages/core`: approval and groups API from the generated schema
- [x] `approval.request` card with approve, reject (reason) and cancel; "Approvals" page with the new request form (leave and general, approver picker, attachments) and the two lists; detail with history
- [x] "Groups" page: everyone sees groups and managers, owners and admins edit
- [x] Hidden when `/meta` lacks approval; Thai and English; component tests; build, test and lint pass

**Files:** `packages/core/src/` (new `approval/`, `api/`), `apps/web/src/` (new `approval/` and `groups/`, `App.tsx`, `pages/Shell.tsx`, `i18n/`). Register the card renderer from `approval/`; do not edit `chat/` files.

## [x] M4-08 M4 acceptance test and module matrix (wave 3)
**SRS:** 16.3 M4, PLT-10, PLT-12 · **Size:** M · **Depends on:** M4-06, M4-07

**Acceptance criteria**
- [x] `internal/acceptance`: task from a message → card; status change from the card's API → `card.updated` to channel members; leave request → card in the DM → manager approves → the requester's card updates within 1 s; a non-approver gets 403; events published in a rolled-back transaction are never delivered and a relay restart delivers pending ones
- [x] CI job starts the server with the module sets in the plan's PLT-12 and checks `/healthz`, `/meta` and a disabled module's 404
- [x] `go test -race ./internal/acceptance/` passes; CI green

**Files:** `internal/acceptance/`, `.github/workflows/`

## Checkpoint M4: SRS 16.3 acceptance (planner)
- [x] Create a task from a message, then change its status from the card
- [x] Send a leave request; the manager approves from the card; the requester's card changes at once
- [x] A non-approver who presses approve gets 403

Planner notes from checkpoint M4: checked on the redesigned UI with two browser contexts (Playwright). The status change reached the other user's card in 31 ms and the approval in 51-67 ms over three rounds; a third member calling approve got 403 and the request stayed pending (no card reaches them, so only the API path applies). Follow-ups: the new-request dialog clips its submit button at 1366x900; a 404 page waits ~8 s on react-query retries; a double space after a picked mention; the 403 text ends in ": forbidden"; two unexplained 500s (`/notify/unread-count`, `/chat/channels`) right after one login, not reproduced in 15 tries and with no ERROR log.

# M5: helpdesk v1

Plan: `tasks/plan.md` section "M5" (states, APIs, payloads, events, card rules; canonical). Contracts are fixed in M5-00; workers do not change `internal/app` or another module's `api.go`/`events.go`.

## [x] M5-00 contracts (planner)
- [x] `app.MessageReader`, `app.MessageSnapshot`, `app.MessageAttachment`
- [x] Plan section M5 and these tasks

## [x] M5-01 helpdesk core (wave 1)
**SRS:** HD-01..07, 09, 10, 18 · **Size:** L (commit in slices: migrations and admin API, create with numbering, list and detail, assign and status with the state table, comments and history, uploads and attachments) · **Depends on:** M5-00

Build `internal/modules/helpdesk` like task and approval (`domain`, `usecase`, `adapter/postgres` with sqlc via `go tool sqlc`, `adapter/http`, `migrations`, `module.go`, `api.go`, `events.go`), exactly as the plan's "helpdesk module" and "States".

**Acceptance criteria**
- [x] Migrations; the admin API with `helpdesk.admin` registered for owner and admin; prefixes unique per workspace; one default category
- [x] `POST /helpdesk/tickets` with per-queue numbering in the transaction (concurrent creates never share a number); `source_message_id` read through `app.MessageReader` when it is available (400 when it is not)
- [x] List filters, visibility (requester, queue agents, admins; others 404) and cursor paging with an empty cursor on the last page
- [x] Every transition in the plan's table allowed for the right people, every other one 409, the wrong person 403, reopen only within 7 days; all as table-driven `domain` tests
- [x] Public and internal comments; the requester never sees internal comments or their history rows; every change written to `helpdesk_history`
- [x] Events from `events.go` published in the transaction and declared (handlers come in M5-03)
- [x] Uploads and attachment downloads through `Deps.Storage`; message attachments keep chat's key
- [x] Unit tests with fakes, handler tests with `humatest`, repository tests on testcontainers, a module test on the real registry; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/helpdesk/` (new). Do not edit `cmd/`; the planner registers the module.

## [x] M5-02 chat: MessageReader (wave 1)
**SRS:** HD-03 · **Size:** S · **Depends on:** M5-00

**Acceptance criteria**
- [x] chat provides `app.MessageReader` in `Provide`
- [x] Returns body, kind, sender, channel, creation time and attachments with their object keys; `ErrNotFound` for a missing message, another workspace, a deleted message, or a user who cannot read the channel (same rules as `ChannelAccess`)
- [x] Tests for each case; existing chat tests pass; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/chat/` except `api.go` and `events.go`

## [x] Checkpoint F: after M5 wave 1 (planner)
- [x] Branches merged; helpdesk registered in `cmd/server` and `cmd/migrate`; OpenAPI and `schema.d.ts` regenerated; no schema name collisions
- [x] All checks pass; CI green; a live API run of create, claim, every transition and comments

Planner notes from checkpoint F: workers Claude Haiku 5.5, merged without conflicts; helpdesk registered in `cmd/server` and `cmd/migrate`; OpenAPI regenerated, no new numbered schema names (`ListMembersOutputBody1` predates M5). `go vet`, `golangci-lint` (0 issues), `go test ./...` and the web build, test and lint pass. Live API run 26/26: queue with prefix `it` → `IT`, duplicate prefix 409, a member managing queues 403, ticket `IT-0001` in `new`, new → in_progress 409, an agent assigning someone else 403, claim → open, the requester moving it 403, every path in the state table including waiting and back, resolved → closed by the requester after a reopen, closed → open 409, the requester posting internal 403 and seeing only public comments and no internal history rows, an empty `next_cursor` on the last page.

Accepted deviations (M5-01): history columns are `from_value`/`to_value` in SQL; new → open only through assign; reopen after 7 days is 409; a new default category demotes the old one; prefixes are upper-cased; moving a ticket to a queue where its assignee is not an agent clears the assignee, and a number collision there is 409; validation errors are 400.

## [x] M5-03 helpdesk cards, notifications and chat extensions (wave 2)
**SRS:** HD-03, 08, 20 · **Size:** M · **Depends on:** checkpoint F

**Acceptance criteria**
- [x] Card kind `helpdesk.ticket` registered; requester card for tickets made from chat and agents' card in the queue channel, posted and updated by the module's own event handlers, idempotently, with `version`
- [x] Notification rules as in the plan, with their keys
- [x] `/ticket <title>` (default category; `no_default_category` 400 otherwise) and `helpdesk.create_from_message` registered when `ChatExtensions` is available
- [x] Module tests on the real registry with the in-memory bus for each card and rule; `go build ./... && go vet ./... && go test ./...` pass

**Files:** `internal/modules/helpdesk/`

## [x] M5-04 web: helpdesk pages (wave 2)
**SRS:** HD-01, 02, 05..07, 09, 10, 18 · **Size:** M · **Depends on:** checkpoint F

**Acceptance criteria**
- [x] `packages/core`: helpdesk API from the generated schema
- [x] "Helpdesk" list with "My requests" and "Queue" tabs and filters; ticket detail with the status actions the viewer may take, claim and assign, public and internal comments (the internal switch only for agents and admins), history; new-ticket dialog without attachments; "Helpdesk settings" for admins
- [x] Hidden when `/meta` lacks helpdesk; Thai and English; component tests; shadcn/ui per `docs/ui.md`; `npm run build`, `npm test` and `npm run lint` pass

**Files:** `packages/core/src/` (new `helpdesk/`, `api/`), `apps/web/src/` (new `helpdesk/`, `App.tsx`, `components/AppSidebar.tsx` nav item only, `i18n/` new `helpdesk-messages.ts` and one spread line)

## [x] M5-05 web: ticket card, message action, attachments and task from ticket (wave 3)
**SRS:** HD-03, 08, 19 · **Size:** M · **Depends on:** M5-03, M5-04

**Acceptance criteria**
- [x] `helpdesk.ticket` card renderer registered from `helpdesk/` with Claim and Open; live updates through `card.updated`
- [x] Message menu "Create ticket" opens the new-ticket dialog with the message text and `source_message_id`; attachments in the dialog and on the detail page
- [x] "Create task" on the ticket page when `/meta` lists task and the queue has a channel; hidden otherwise
- [x] Component tests; build, test and lint pass

**Files:** `apps/web/src/helpdesk/` (new `register.ts` like `approval/register.ts`), `i18n/helpdesk-messages.ts`, `App.tsx` (one import line), `chat/MessageItem.tsx` (only the "Create ticket" menu item for `helpdesk.create_from_message`, mirroring "Create task")

## [x] M5-06 M5 acceptance test (wave 3)
**SRS:** 16.3 M5 · **Size:** M · **Depends on:** M5-03

**Acceptance criteria**
- [x] `internal/acceptance`: a ticket from a message gets its number and cards; an agent claims it and walks every path in the state table, including waiting and back and a reopen; a forbidden transition is 409 and the wrong person 403; the requester never receives internal comments (API and card); with task disabled helpdesk still works
- [x] PLT-12 matrix gains `helpdesk` and `notify,task,approval,helpdesk`
- [x] `go test -race ./internal/acceptance/` passes; CI green

**Files:** `internal/acceptance/`, `.github/workflows/ci.yml`, `scripts/check-modules.sh`

## [x] Checkpoint M5: SRS 16.3 acceptance (planner)
- [x] Open a ticket from a message; an agent claims it and changes status along every path in the diagram
- [x] The requester does not see internal notes
- [x] With task turned off, helpdesk still works

# M6: v0.1 release

## [x] M6-00 contracts (planner)
- [x] `app.Mailer`, `app.Mail`, `Deps.Mailer` (`internal/app/mail.go`, `deps.go`)
- [x] `app.Notification.ChannelID`; `auth.UserSummary.Timezone` filled by the auth `Directory`
- [x] M6 section in `tasks/plan.md`

## [x] M6-01 platform: mail (wave 1)
**SRS:** AUTH-09, NFR-05 · **Size:** S · **Depends on:** M6-00

**Acceptance criteria**
- [x] `internal/platform/mail`: `New(cfg, logger) app.Mailer` returns an SMTP mailer (STARTTLS on 587, implicit TLS on 465, auth when a username is set) when `SMTP_HOST` is set, else a log mailer that writes recipient, subject and text at info level
- [x] Config reads `WEB_URL`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` with the defaults in the plan; `.env.example` lists them; Mailpit (`axllent/mailpit`, SMTP 1025, UI 8025) added to `deploy/docker-compose.yml` for development
- [x] Unit tests for config and the log mailer; an integration test sends through Mailpit with testcontainers and reads it back from the Mailpit API
- [x] `go build`, `go vet`, `go test ./...` pass

**Files:** `apps/server/internal/platform/mail/` (new), `apps/server/internal/platform/config/`, `.env.example`, `deploy/docker-compose.yml`

## [x] M6-02 auth: email verification and password reset (wave 1)
**SRS:** AUTH-09, AUTH-03, AUTH-06, NFR-07 · **Size:** M · **Depends on:** M6-00

**Acceptance criteria**
- [x] Migration adds `auth_email_tokens` and `users.email_verified_at`; tokens as in the plan (random, stored hashed, 1 hour, single use)
- [x] Register enqueues `auth.send_email` in its transaction; the job handler renders Thai or English text by the user's locale with links built from `WEB_URL` and calls `Deps.Mailer` (skipped with a log line when nil)
- [x] `POST /auth/email/verify`, `POST /auth/email/verify/resend`, `POST /auth/password/forgot` (always 204, rate limited per email and IP), `POST /auth/password/reset` (sets the password with argon2id and revokes all refresh tokens); `GET /auth/me` includes `email_verified`
- [x] Tests: expired, reused and wrong-purpose tokens are rejected; forgot for an unknown email is 204 and sends nothing; reset ends other sessions
- [x] `go build`, `go vet`, `go test ./...` pass

**Files:** `apps/server/internal/modules/auth/` (all subpackages and migrations). Do not touch `cmd/server`; the planner wires `Mailer` at checkpoint H. Use a fake `app.Mailer` in tests.

## [x] M6-03 workspace: audit log (wave 1)
**SRS:** WSP-10, WSP-06 · **Size:** M · **Depends on:** M6-00

**Acceptance criteria**
- [x] Migration adds `workspace_audit`; every admin endpoint in the plan's action list writes one row in the same transaction as its change, with actor, target and the `data` shown in the plan
- [x] `GET /workspace/audit?cursor=&limit=` (≤ 100, newest first, `{items, next_cursor}`) for Owner and Admin; 403 for others; never rows of another workspace
- [x] Tests for each action, the permission check and workspace isolation
- [x] `go build`, `go vet`, `go test ./...` pass

**Files:** `apps/server/internal/modules/workspace/` (all subpackages and migrations)

## [x] M6-04 notify: preferences, mute and do not disturb (wave 1)
**SRS:** NOTI-07 · **Size:** M · **Depends on:** M6-00

**Acceptance criteria**
- [x] Migration adds `notify_preferences`; `GET` and `PUT /notify/preferences` (`{disabled_kinds, muted_channel_ids, dnd_start, dnd_end}`, times as `HH:MM`, both or neither); `GET` returns defaults when no row exists; kinds validated against the registered rule kinds where known
- [x] Dispatch drops notices whose kind is disabled or whose `ChannelID` is muted; inside the DND window (user's `Timezone` from `auth.Directory`, wrapping past midnight) the notice is stored and counted and the WebSocket frame has `quiet: true`
- [x] chat sets `ChannelID` on every notice it produces (mention, DM, thread reply)
- [x] Tests for each rule, the window across midnight in `Asia/Bangkok`, and that other users are unaffected
- [x] `go build`, `go vet`, `go test ./...` pass

**Files:** `apps/server/internal/modules/notify/` (all subpackages and migrations), `apps/server/internal/modules/chat/` (only where its notification rules build `app.Notification`, plus their tests)

## [x] M6-05 observability: Prometheus metrics (wave 1)
**SRS:** NFR-14 · **Size:** S · **Depends on:** M6-00

**Acceptance criteria**
- [x] `internal/platform/observability` exposes a registry, the HTTP middleware (`http_request_duration_seconds` by route pattern, method and status; no raw paths or IDs as labels) and `GET /metrics`
- [x] `ws_connections` gauge and `ws_messages_sent_total` counter from `platform/realtime`; Go runtime and process collectors
- [x] `cmd/server` mounts `/metrics` and the middleware; a test scrapes `/metrics` after one request and finds the series
- [x] `go build`, `go vet`, `go test ./...` pass

**Files:** `apps/server/internal/platform/observability/`, `apps/server/internal/platform/realtime/` (metric hooks only), `apps/server/internal/platform/httpx/` (middleware hook only), `apps/server/cmd/server/` (mounting only), `apps/server/go.mod`, `go.sum`

## [x] Checkpoint H: after M6 wave 1 (planner)
- [x] Merge M6-01..05; wire `mail.New` into `Deps.Mailer` in `cmd/server`; register new migrations in `cmd/migrate`
- [x] `make openapi` and commit regenerated types; full CI green

## [x] M6-06 web: email verification and password reset (wave 2)
**SRS:** AUTH-09, NFR-17 · **Size:** S · **Depends on:** Checkpoint H

**Acceptance criteria**
- [x] Pages `/verify-email?token=`, `/forgot-password`, `/reset-password?token=` with success, expired and invalid states; "Forgot password?" link on sign-in; banner with "Resend" for unverified users
- [x] Core API client from the generated schema; Thai and English; component tests; build, test and lint pass

**Files:** `packages/core/src/api/` (auth only), `apps/web/src/pages/` (new pages, `Login.tsx` link), `apps/web/src/App.tsx` (routes), `apps/web/src/components/` (new banner), `apps/web/src/i18n/`

## [x] M6-07 web: audit log and notification settings (wave 2)
**SRS:** WSP-10, NOTI-07 · **Size:** M · **Depends on:** Checkpoint H

**Acceptance criteria**
- [x] "Audit log" page for Owner and Admin (actor, action in words, target, time in the user's time zone, load more)
- [x] "Notifications" settings: switch per kind, do-not-disturb start and end, muted channels list; "Mute channel" in the channel header menu; quiet frames show no toast
- [x] Core API clients; Thai and English; component tests; build, test and lint pass

**Files:** `packages/core/src/api/` (workspace audit and notify preferences only), `apps/web/src/pages/` (new pages), `apps/web/src/notify/`, `apps/web/src/chat/ChannelPage.tsx` (mute item in the header only), `apps/web/src/App.tsx`, `apps/web/src/components/AppSidebar.tsx` (nav items only), `apps/web/src/i18n/`

## [x] M6-08 self-hosting stack and multi-arch images (wave 2)
**SRS:** NFR-05, NFR-11, NFR-15 · **Size:** M · **Depends on:** Checkpoint H

**Acceptance criteria**
- [x] `deploy/compose.prod.yml`: `server` (migrate, then serve; healthcheck on `/readyz`), `web` (Caddy serving the build and proxying `/api` and `/ws`; automatic TLS when `DOMAIN` is set), postgres, redis, minio; `/metrics` not published through Caddy; secrets only from `.env`
- [x] `docker compose -f deploy/compose.prod.yml up` on a clean checkout with the example env gives a working app on `http://localhost`
- [x] CI builds both images for `linux/amd64` and `linux/arm64` on every push and pushes to GHCR on `v*` tags
- [x] `deploy/README.md` with the install steps an outsider follows

**Files:** `deploy/`, `apps/server/Dockerfile`, `apps/web/Dockerfile` (new), `.github/workflows/` (new `images.yml`), `.env.example`

## [x] M6-09 hardening sweep (wave 2)
**SRS:** NFR-06, NFR-07, NFR-09, NFR-10, NFR-12 · **Size:** M · **Depends on:** Checkpoint H

**Acceptance criteria**
- [x] One acceptance test that, for every module, calls each workspace-scoped read and write endpoint with a member of another workspace and expects 403 or 404 (NFR-06)
- [x] Every presigned URL has a 15-minute lifetime, asserted in one test; every endpoint has input limits (list any without, do not change them) (NFR-07)
- [x] CI step computes coverage of all `domain` and `usecase` packages and fails below 80% (NFR-12); if it is below today, report the number and the packages instead of lowering the bar
- [x] `go test -race ./...` passes

**Files:** `apps/server/internal/acceptance/`, `.github/workflows/ci.yml`, `scripts/` (new coverage script)

## [ ] Checkpoint I: after M6 wave 2 (planner)
- [x] Merge M6-06..09; full CI green (planner added `S3_PUBLIC_ENDPOINT`, Caddy bucket route, `POSTGRES_PASSWORD`/`PUBLIC_URL` from `.env`, coverage over the whole suite with `-coverpkg`: 90.4%)
- [ ] Fresh clone on a clean machine: follow `deploy/README.md`, working app in under 10 minutes (SRS 16.3 M6)

## [ ] M6-12 API rate limits and input limits (wave 3)
**SRS:** NFR-07 · **Size:** M · **Depends on:** Checkpoint I

Found by M6-09: `platform/ratelimit` has no callers, so only login (auth's own limiter) is rate limited, and `knownLimitGaps` in `internal/acceptance/nfr07_test.go` lists 71 request values with no declared limit.

**Acceptance criteria**
- [ ] A per-user (and per-IP when signed out) limit on every `/api/v1` route through `platform/ratelimit` on Redis, mounted in the router; 429 with `Retry-After`; limits in config with sane defaults; login keeps its stricter limit
- [ ] Every entry of `knownLimitGaps` gets a schema limit (`maxLength`, `maximum`, `maxItems`, `maxProperties`) that matches what the usecase allows; the list ends empty and the test stays exact
- [ ] Tests for 429 and for one limit per module; `go test -race ./...` passes; OpenAPI regenerated

**Files:** `apps/server/internal/platform/ratelimit/`, `apps/server/internal/platform/httpx/` (middleware hook only), `apps/server/internal/platform/config/`, `apps/server/cmd/server/` (mounting only), every module's `adapter/http/` (struct tags only), `apps/server/internal/acceptance/nfr07_test.go`, `apps/server/api/`, `packages/core/src/api/schema.d.ts` (generated)

## [ ] M6-10 load test (wave 3)
**SRS:** NFR-01, NFR-02, NFR-03, NFR-04 · **Size:** M · **Depends on:** Checkpoint I

**Acceptance criteria**
- [ ] k6 scripts in `scripts/load/` with a seed step: REST mix (p95 < 200 ms), send-to-receive over WebSocket (p95 < 300 ms), 1,000 concurrent sockets on one instance, two instances behind Caddy where every message reaches every recipient
- [ ] `make load` runs them against `compose.prod.yml` with a `scale` of 2; thresholds fail the run
- [ ] `docs/load-test.md`: machine, results, `/metrics` screenshots or numbers, and anything fixed or still weak

**Files:** `scripts/load/` (new), `Makefile`, `deploy/compose.prod.yml` (load profile only), `docs/load-test.md` (new). Report server bottlenecks; do not change server code.

## [ ] M6-11 release docs and online demo (wave 3)
**SRS:** NFR-16, NFR-18, SRS 16.3 M6 · **Size:** S · **Depends on:** Checkpoint I, hosting chosen by the user

**Acceptance criteria**
- [ ] README: what it is, screenshots, install via `deploy/README.md`, development setup, modules; status `v0.1.0`
- [ ] `CHANGELOG.md` for 0.1.0; `docs/architecture.md` matches the code (modules, contracts, outbox, metrics); CONTRIBUTING up to date
- [ ] Demo deployed on the chosen host with seed data and a public sign-up; manual check in the latest two Chrome, Firefox, Safari and Edge, and at 360 px width (NFR-16)

**Files:** `README.md`, `CHANGELOG.md` (new), `CONTRIBUTING.md`, `docs/`, `deploy/` (demo config only)

## [ ] Checkpoint M6: SRS 16.3 acceptance and v0.1.0 (planner)
- [ ] NFR-01 to 03 targets met in `docs/load-test.md`
- [ ] An outsider installs from the README in under 10 minutes
- [ ] The online demo works; tag `v0.1.0`

# UI round 2: Slack-style redesign and landing page

Decided 2026-11-19 with the user: follow Slack (workspace rail, channel and DM sidebar, threads in a right pane, dense message list). Use the parts of `/design-taste-frontend` that fit an app (one system: tokens, type, one button pair, one card style, one radius scale; hover lift and press on every control; real full-colour icons for third-party things; no violet; keyboard and focus rules) for the app, and the whole skill for a separate landing page. Workers on Claude Sonnet. Every UI task checks light and dark at 1440, 1024, 768 and 390 with screenshots, and keeps `npm run build`, `npm test` and `npm run lint` green; tests change only where markup changed, never to weaken an assertion.

## [x] UI-07 design foundation and the Slack shell (stage 1)
**Size:** M · **Depends on:** M6 wave 2

**Acceptance criteria**
- [x] New tokens in `index.css` (light and dark): a palette and type pairing dealt with the skill's `start.mjs deal --page software` (no violet; replace the indigo accent), a dark sidebar in the Slack manner, one radius scale, Thai-safe line heights kept; `docs/ui.md` updated to the new system
- [x] Shell: a narrow workspace rail (workspace initials, switch, add), the sidebar with sections (Channels, Direct messages, Apps for Tasks, Approvals, Helpdesk when enabled) that collapse, unread bold and badge, a top search-style bar with the workspace name; below `md` the rail and sidebar are one sheet
- [x] Hover and press feedback on every button, nav item and linked row; visible focus ring; `prefers-reduced-motion` respected
- [x] Screenshots of the shell in light and dark at the four widths in the final report

**Files:** `apps/web/src/index.css`, `apps/web/src/components/` (shell parts and `ui/` restyles), `apps/web/src/pages/Shell.tsx`, `apps/web/src/chat/Sidebar.tsx`, `apps/web/src/chat/ChatLayout.tsx`, `apps/web/src/i18n/ui-messages.ts`, `docs/ui.md`, and the tests of these files

## [x] UI-08 chat view: messages, composer and threads (stage 2)
**Size:** M · **Depends on:** UI-07

**Acceptance criteria**
- [x] Message list in the Slack manner: grouped by sender and minute, avatar and name on the first of a group, hover toolbar (react, reply in thread, more), date dividers, "new messages" line, reactions as pills, cards fit the new card style
- [x] Composer: a bordered box with a formatting row, attach, emoji and send, slash command and mention popovers restyled; thread pane on the right with its own composer, full screen below `md`
- [x] Channel header: name, topic, member count, and the mute and other menu items
- [x] Screenshots in light and dark at the four widths

**Files:** `apps/web/src/chat/` (except `Sidebar.tsx` and `ChatLayout.tsx` unless the change is styling only), `apps/web/src/i18n/chat-messages.ts`, and their tests

## [x] UI-09 module pages: tasks, approvals, helpdesk, settings (stage 2)
**Size:** M · **Depends on:** UI-07

**Acceptance criteria**
- [x] Tasks, Approvals, Helpdesk, Members, Invites, Groups, Audit log, Notification settings and Account pages use one page header, one table or list style, one card style and one empty state, all on the new tokens; filters as a toolbar row; detail pages as a two-column layout (content and a properties panel) on wide screens
- [x] Cards of tasks, approvals and tickets in chat share one card family
- [x] Screenshots of each page in light and dark at 1440 and 390

**Files:** `apps/web/src/task/`, `apps/web/src/approval/`, `apps/web/src/helpdesk/`, `apps/web/src/groups/`, `apps/web/src/notify/`, `apps/web/src/pages/` (except `Shell.tsx`), and their tests and i18n files

## [ ] UI-10 landing page (stage 1, independent)
**Size:** M · **Depends on:** none

**Acceptance criteria**
- [ ] A static landing page built with `/design-taste-frontend` in full, kind `software`: what Samnak Chat is (self-hosted chat for organisations with tasks, approvals and helpdesk inside the conversation), for whom, one hard fact on the first screen, the product's own UI as live HTML in one `app-frame`, modules, self-hosting with Docker Compose, open source (AGPL-3.0); main action "Install on your server" to `deploy/README.md` on GitHub, second action "Open the demo" (link set later); Thai and English pages
- [ ] Nothing invented: no customers, quotes or numbers that are not true of the project
- [ ] `gate.mjs` ends with `GATE: PASS` (or `OPEN` with reasons); the report lists waivers, stand-ins and what the user must supply

**Files:** `apps/landing/` (new, static: `index.html`, `th/index.html`, `assets/`), `README.md` (one link line)

## [ ] Checkpoint UI-2 (planner)
- [ ] Merge UI-07..10; full CI green; review the screenshots with the user

# Lark-style Messenger: conversation list with pictures and pins

Asked by the user 2026-11-20: reaching chats and group chats should be easier, with pictures to click like Lark, and pinning. Group chats are the existing multi-person direct messages (`chat` type `dm` with 3+ members) and private channels.

## [x] CH-01 chat: pins and last-message preview (server)
**SRS:** CHAT (UX extension) · **Size:** S · **Depends on:** none

**Contract (planner)**
- Migration: `chat_channel_members.pinned_at timestamptz NULL`. A pin is per user and channel; at most 30 pinned channels per user and workspace (the 31st is 409 `conflict`).
- `PUT /chat/channels/{id}/pin` and `DELETE /chat/channels/{id}/pin`: 204; members only (404 otherwise); idempotent. Each sends `channel.pin {workspace_id, channel_id, pinned_at|null}` over the WebSocket to the caller's own connections only.
- `Channel` gains `pinned_at` (`null` when not pinned) and `last_message` (`null` for an empty channel): `{id, sender_id, kind, preview, created_at}`, where `preview` is the body as plain text cut to 120 characters, a card's `fallback_text`, an empty string with attachments only, and `""` for a deleted message. The channel list orders pinned first (newest pin first), then by the newest message, then by name. No extra query per channel.

**Acceptance criteria**
- [x] The contract above, with tests: pin/unpin, limit, non-member 404, another workspace 404, the frame reaches only the caller, preview of text, card, attachment-only and deleted messages, list order
- [x] `go build`, `go vet`, `go test ./...` pass; gofmt clean

**Files:** `apps/server/internal/modules/chat/` (all subpackages and migrations), `apps/server/internal/acceptance/isolation_test.go` (add the two new routes to the sweep), `apps/server/internal/acceptance/nfr07_test.go` (only if a new gap appears)

## [x] CH-02 web: Lark-style Messenger list, app rail and pins
**Size:** M · **Depends on:** CH-01 merged and types regenerated

**Acceptance criteria**
- [x] The rail gains app icons with labels under the workspace tiles: Messenger, Tasks, Approvals, Helpdesk (each when its module is on), Members; the current app is marked
- [x] In Messenger the sidebar is a conversation list: search box, filter chips (All, Unread, Groups, Direct), a Pinned group on top, then everything by latest message; each row a picture (the user's avatar for a 1:1 DM, stacked avatars for a group DM, a coloured tile with the initial for a channel; lock or `#` badge), name, `sender: preview`, time, unread badge, mute icon
- [x] Pin and unpin from the row's hover menu and right-click menu and from the channel header menu; `channel.pin` frames keep other tabs in step; "New chat" starts a 1:1 or group DM, or a channel
- [x] Slash-command errors show the server's reason (for example `/ticket` without a default category: "No default category yet: set one in Helpdesk settings or use Create ticket") instead of the generic invalid message
- [x] Thai and English, light and dark, screenshots at 1440 and 390; build, test and lint pass

**Files:** `apps/web/src/chat/`, `apps/web/src/components/` (rail, sidebar parts), `apps/web/src/pages/Shell.tsx`, `apps/web/src/notify/ChannelMuteMenu.tsx`, `apps/web/src/i18n/`, `packages/core/src/` (chat API, store and realtime types for `channel.pin`), and their tests

## [x] CH-03 avatars of other people (server + web)
**Size:** S · **Depends on:** CH-02

Found by CH-02: `Member` (`GET /workspace/members`) and `ChannelMember` (`GET /chat/channels/{id}/members`) carry no `avatar_url`, so only the signed-in user's picture shows; everyone else falls back to initials.

**Acceptance criteria**
- [x] `avatar_url` (string, empty when none) on both responses, filled from `auth.UserSummary.AvatarURL` through the existing directory lookups (no new query per row); tests
- [x] `useAvatarOf` in `apps/web/src/chat/names.ts` reads it, so DM rows, group stacks, message rows and member lists show real pictures with the initials fallback on load error
- [x] OpenAPI and web types regenerated; Go and web checks pass

**Files:** `apps/web/src/components/UserAvatar.tsx` (a picture source prop), `apps/web/src/chat/MessageItem.tsx` and `apps/web/src/chat/MentionList.tsx` (pass the picture only), `apps/server/internal/modules/workspace/` (members response only), `apps/server/internal/modules/chat/` (channel members response only), `apps/server/api/`, `packages/core/src/api/schema.d.ts` (generated), `apps/web/src/chat/names.ts` and its tests, `packages/core/src/chat/fake-server.ts`, `apps/web/src/test-utils.tsx`

## [x] SB-01 web: one sidebar per app (Lark-style)
**Size:** M · **Depends on:** CH-02

Asked by the user 2026-11-20: clicking an app in the rail should show only that app's own sidebar, not the old full sidebar with every section. Today only Messenger has its own list; on every other page the sidebar still shows Channels, Direct messages, Apps, Workspace and Account together.

**Design (planner)**
- The rail picks the app; the sidebar shows only that app. Rail order: Messenger, Tasks, Approvals, Helpdesk (each when its module is on), Workspace (Members, Invites, Groups, Audit log; Members for everyone, the rest by role as today). The current app is marked on the rail; the sidebar header names it.
- **Messenger** (`/chat/*`): the conversation list from CH-02, unchanged.
- **Tasks** (`/tasks`): views as links that set the existing filters: My tasks (open), Due today, Overdue, Done; then a "By channel" list of the channels that have tasks.
- **Approvals** (`/approvals/*`): Waiting for me (with count), My requests, and a "New request" button.
- **Helpdesk** (`/helpdesk/*`): My requests, Queue (agents and admins) with one entry per queue the user is an agent of, Settings (admins), and a "New ticket" button.
- **Workspace** (`/members`, `/invites`, `/groups`, `/audit`): one link each, hidden by role as today.
- **Account** leaves the sidebar: the user block at the bottom of the rail (avatar) opens a menu with Profile, Devices, Notification settings, theme, language and Sign out. `/profile`, `/devices` and `/notifications/settings` show a small Account sidebar with those three links.
- `/` (Home) opens Messenger. Every view in a sidebar is a real URL (query string for filters), so back/forward and reload keep it. Unread counts stay on the rail icons (Messenger, Approvals waiting count, Helpdesk queue count when cheap; no new endpoints).
- Below `md`, the rail and the current app's sidebar are one sheet as today.

**Acceptance criteria**
- [x] Each rail app shows only its own sidebar as designed; no page shows the old all-in-one sidebar; the old `SidebarSection` lists are removed or reused only inside the app they belong to
- [x] Sidebar views drive the existing pages' filters through the URL; the pages' own filter toolbars stay in step
- [x] Account menu on the rail; keyboard reachable, focus visible, hover and press feedback; Thai and English; light and dark
- [x] Tests for app switching, each sidebar's links by role and module, URL filters; screenshots of every app's sidebar at 1440 and 390 in light and dark
- [x] `docs/ui.md` updated for the rail, the per-app sidebars and the account menu; build, test and lint pass

**Files:** `apps/web/src/components/` (rail, sidebar, account menu), `apps/web/src/pages/Shell.tsx`, `apps/web/src/chat/Sidebar.tsx`, `apps/web/src/App.tsx` (Home redirect only), `apps/web/src/task/`, `apps/web/src/approval/`, `apps/web/src/helpdesk/` (their nav and sidebar parts, and filter wiring to the URL), `apps/web/src/i18n/`, `docs/ui.md`, and the tests of these files
