# Implementation Plan: Samnak Chat, Phase 1

Source specs: `docs/srs.md` (requirements), `docs/architecture.md` (design).
Tasks are tracked in `tasks/todo.md`. Roles and the worker loop are in `AGENTS.md`.

## Overview

Phase 1 delivers the Go server and the React web app up to v0.1 in milestones M0–M6 (SRS section 15).
This plan details **M0: project skeleton** (PLT-01 to PLT-08). Later milestones are listed so the order stays visible, and each will be broken into tasks when the previous checkpoint passes.

## Architecture decisions already fixed

- Modular monolith in Go, Clean Architecture inside each module (ADR 0001, 0002).
- Shared contracts in `apps/server/internal/app` are written by the planner **before** workers start, so platform packages and modules can be built in parallel against stable interfaces. Contracts: `Module`, `Deps`, `EventBus`, `Services` (`Provide`/`Lookup`), `NotificationRegistry`, `CardRegistry`, `TxManager`, `Principal`, standard errors.
- npm workspaces (`apps/web`, `packages/*`), with no extra tooling to install.
- Go module path: `github.com/KPV2004/samnak-chat/apps/server`.
- License AGPL-3.0, repo public.

## Contract: `GET /api/v1/meta` (used by M0-07 and M0-09)

```json
{
  "name": "samnak-chat",
  "version": "0.0.0-dev",
  "modules": ["auth", "workspace", "chat", "notify", "task"]
}
```

`modules` lists enabled modules in startup order. In M0 no real modules exist yet, so the server enables none and returns `[]` unless test modules are registered.

## Contract: error JSON (used by M0-04)

```json
{ "error": { "code": "not_found", "message": "channel not found" } }
```

Codes: `invalid` 400, `unauthorized` 401, `forbidden` 403, `not_found` 404, `conflict` 409, `rate_limited` 429, `internal` 500.

## M0 dependency graph

```
internal/app contracts (planner, done)
 ├── M0-01 config ─────────┐
 ├── M0-02 eventbus ───────┤
 ├── M0-03 registry ───────┼──► M0-07 server wiring + /meta + OpenAPI ──► M0-09b web shows /meta
 ├── M0-04 httpx ──────────┤
 ├── M0-05 database+tx ──► M0-06 migrate + outbox
 ├── M0-08 compose + Dockerfile + CI + boundary check
 ├── M0-09 web + packages/core skeleton
 └── M0-10 ADRs 0001–0008
```

## Waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 1 | M0-01+02, M0-03, M0-04, M0-05+06, M0-08, M0-09, M0-10 | 7 parallel worker sessions, one worktree each, disjoint files |
| Checkpoint A | Planner merges, runs full build and tests | Planner |
| 2 | M0-07 (then M0-09b) | 1 worker |
| Checkpoint M0 | SRS 16.3 M0 acceptance | Planner |

## M1: accounts and workspaces (SRS AUTH-01..07, WSP-01..08)

### Contracts the planner fixed before M1

- `Module.Routes(api huma.API)`: each module gets a Huma group prefixed with `/{name}`, so its operations appear in `openapi.yaml`. `Module.Init` must not do I/O, because `server openapi` runs it without a database.
- `app.Identity`, `app.Authenticator` (provided by auth) and `app.MembershipResolver` (provided by workspace) in `internal/app/auth.go`.
- `httpx.RequireUser` and `httpx.RequireMember` Huma middleware in `internal/platform/httpx/auth.go`. Every authenticated operation sets `Security: httpx.BearerAuth` and one of them in `Middlewares`.
- `auth.Directory` in `internal/modules/auth/api.go`: other modules read user names and emails through it.
- sqlc is a Go tool: `go tool sqlc generate`. Each module keeps `adapter/postgres/sqlc.yaml`, `queries/*.sql`, and its migrations as the sqlc schema.

### Decision: routes follow the module prefix

The SRS lists `/workspaces/...`, but the architecture mounts every module under `/api/v1/{name}`. The module prefix wins, and workspace-scoped calls take the workspace from `X-Workspace-ID` rather than the path. So `POST /workspace`, `GET /workspace`, `GET /workspace/members`, and so on.

### Tokens

- Access token: JWT HS256 signed with `JWT_SECRET`, 15 minutes. Claims: `sub` (user ID), `did` (device ID), `iat`, `exp`, `jti`.
- `Authenticate` also checks that the device is not revoked, so logout takes effect immediately.
- Refresh token: 32 random bytes, base64url, 30 days. Only its SHA-256 hash is stored. Each use issues a new pair and revokes the old token. Reusing a revoked token revokes every token of that device (AUTH-03).
- One device row per login (`device_name`, `platform`). The device ID is the session.

### API (all paths under `/api/v1`; errors as in the error contract)

| Method and path | Auth | Request body | Response |
| --- | --- | --- | --- |
| `POST /auth/register` | none | `{email, password (8–128), display_name}` | 201 `User` |
| `POST /auth/login` | none | `{email, password, device_name, platform: web\|desktop\|ios\|android}` | 200 `TokenPair` |
| `POST /auth/refresh` | none | `{refresh_token}` | 200 `TokenPair` |
| `POST /auth/logout` | user | `{all_devices: bool}` | 204 |
| `GET /auth/me` | user | | 200 `User` |
| `PATCH /auth/me` | user | `{display_name?, avatar_url?, locale?: th\|en, timezone?: IANA}` | 200 `User` |
| `GET /auth/devices` | user | | 200 `{items: Device[]}` |
| `DELETE /auth/devices/{id}` | user | | 204 |
| `POST /workspace` | user | `{name, slug}` | 201 `Workspace` |
| `GET /workspace` | user | | 200 `{items: Workspace[]}` (mine, with my role) |
| `GET /workspace/members` | member | `?cursor&limit` | 200 `{items: Member[], next_cursor}` |
| `PATCH /workspace/members/{userId}` | member (admin) | `{role?, status?: active\|deactivated}` | 200 `Member` |
| `DELETE /workspace/members/{userId}` | member (admin) | | 204 |
| `POST /workspace/invites` | member (admin) | `{email?, role: admin\|member\|guest}` | 201 `Invite` (includes `token` once) |
| `GET /workspace/invites` | member (admin) | | 200 `{items: Invite[]}` (no tokens) |
| `DELETE /workspace/invites/{id}` | member (admin) | | 204 |
| `POST /workspace/invites/accept` | user | `{token}` | 200 `Workspace` |

Shapes (JSON is snake_case, times RFC 3339 UTC):

- `User {id, email, display_name, avatar_url, locale, timezone, created_at}`
- `TokenPair {access_token, refresh_token, expires_in: 900, device_id, user: User}`
- `Device {id, name, platform, last_seen_at, current: bool}`
- `Workspace {id, name, slug, role}`
- `Member {user_id, display_name, email, role, status, joined_at}`
- `Invite {id, email, role, expires_at, created_at, token?}`

Rules: login failures are limited to 5 per minute per email and per IP (AUTH-06), with 429. Wrong email and wrong password give the same 401. An invite with an email can only be accepted by the user with that email. Invites expire after 7 days. A workspace always keeps at least one active owner (WSP-04). Only owners may grant or remove the owner role.

### M1 waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 1 | M1-01+02 auth, M1-03+04 workspace, M1-05 redis and rate limit, M1-06 web | 4 parallel workers |
| Checkpoint B | Planner merges, registers modules in `cmd/server` and `cmd/migrate`, regenerates OpenAPI | Planner |
| 2 | M1-07 wiring, isolation tests, web on generated types, acceptance test | 1 worker |
| Checkpoint M1 | SRS 16.3 M1 acceptance | Planner |

## Later milestones (to be broken down at each checkpoint)

| Milestone | Scope | SRS IDs |
| --- | --- | --- |
| M1 | auth + workspace, per-device tokens, roles, `Can()` | AUTH-01..07, WSP-01..08 |
| M2 | chat realtime, `seq`, `client_msg_id`, cards, uploads | CHAT-01..15, 17, 18, AUTH-08, PLT-11 |
| M3 | notify over WebSocket, River jobs | NOTI-01..06, PLT-09 |
| M4 | task + approval, slash commands, groups, outbox | TASK-01..08, APR-01..09, 13, CHAT-16, WSP-09, PLT-10, 12 |
| M5 | helpdesk v1 | HD-01..10, 18..20 |
| M6 | load test, polish, v0.1 release | AUTH-09, WSP-10, NOTI-07, all NFR |

## Risks and mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Parallel workers drift from the contracts | High | Contracts in `internal/app` are owned by the planner; workers report contract problems instead of editing them |
| depguard cannot express "only `api.go` and `events.go`" | Medium | Module internals are subpackages (`domain`, `usecase`, `adapter`), so a package-level deny rule is enough; M0-08 adds a Go test as a fallback |
| Huma and chi integration shapes the httpx API | Medium | M0-04 builds plain chi; Huma is added only in M0-07, behind `httpx` |
| Merge conflicts on `go.mod` / `go.sum` | Low | Planner resolves at checkpoint with `go mod tidy` |

## Open questions (SRS section 17)

- Team size and hours per week, which sets the real calendar for section 15
- Default UI language: Thai or English
- Demo hosting and budget
- APR-11 multi-step approval and Guest users: v0.1 or later
- Default helpdesk SLA and business hours
