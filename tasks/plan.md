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

## M2: realtime chat (SRS CHAT-01..15, 17, 18, AUTH-08, PLT-11)

### Contracts the planner fixed before M2

- `app.Realtime` (`internal/app/realtime.go`): `SendToUsers`, `Online`, `DisconnectDevice`, `Handle(msgType, ClientHandler)`, and `app.Envelope`. Implemented by `platform/realtime`, passed as `Deps.Realtime`.
- `app.ObjectStorage` (`internal/app/storage.go`): `PresignPut`, `PresignGet`, `Stat`. Implemented by `platform/storage`, passed as `Deps.Storage`.
- `app.CardRegistry` (`internal/app/card.go`) now takes an `app.Card` with explicit workspace and sender, and adds `DirectChannel`. chat provides it in `Provide`; `Deps.Cards` is removed, so other modules use `app.Lookup[app.CardRegistry]`.
- `Deps.Realtime` and `Deps.Storage` may be nil in tests and in `server openapi`; use them only while handling requests.

### WebSocket protocol (`GET /api/v1/ws`)

- The hub upgrades without auth. The first client message must be `{"type":"auth","data":{"access_token":"..."}}` within 10 s. The hub calls `app.Authenticator`; on success it replies `{"type":"ready","data":{"user_id","device_id"}}`, otherwise it closes with code 4401.
- Every frame is an `app.Envelope` (`{type, data}`), JSON text frames, at most 64 KiB inbound.
- Client sends `{"type":"ping"}` every 30 s; the hub replies `{"type":"pong"}`. No inbound frame for 60 s → close.
- One reader and one writer goroutine per connection; a bounded send queue (256). A slow client whose queue is full is disconnected, never blocks others.
- Fan-out across instances via Redis pub/sub channel `rt:users` (message: target user IDs + envelope) and `rt:control` (disconnect device). Each instance delivers to its own connections.
- Presence: Redis key `rt:online:{user_id}` with a 60 s TTL, refreshed on ping. `Online` reads these keys.
- Server → client types (SRS 14.3): `message.created`, `message.updated`, `message.deleted`, `card.updated`, `channel.read`, `channel.updated`, `presence.changed`, `typing`, `notification.created`. Client → server: `auth`, `ping`, `typing`.
- Every server event's `data` carries `workspace_id`; clients drop events for workspaces they are not viewing but still update unread counts.
- The WebSocket never replays history. After `ready`, the client calls `GET .../messages?after_seq=` per channel it has open.

### Chat API (prefix `/chat`; all operations use `RequireMember`)

| Method and path | Body or query | Response |
| --- | --- | --- |
| `GET /chat/channels` | `?scope=joined\|public` (default joined) | `{items: Channel[]}` |
| `POST /chat/channels` | `{type: public\|private, name, topic?, member_ids?}` | 201 `Channel` |
| `POST /chat/dms` | `{user_ids}` (1–8 others; same set returns the existing DM) | 200/201 `Channel` |
| `GET /chat/channels/{id}` | | `Channel` |
| `PATCH /chat/channels/{id}` | `{name?, topic?, archived?}` | `Channel` |
| `POST /chat/channels/{id}/join` · `/leave` | | 204 |
| `GET /chat/channels/{id}/members` | | `{items: ChannelMember[]}` |
| `POST /chat/channels/{id}/members` | `{user_ids}` | 204 |
| `GET /chat/channels/{id}/messages` | `?after_seq=` (catch-up, ascending) or `?before_seq=` (history); `?limit` ≤ 100; `?thread_root_id=` | `{items: Message[], has_more}` |
| `POST /chat/channels/{id}/messages` | `{client_msg_id (UUID), body, thread_root_id?, attachment_ids?}` | 201 `Message`, or 200 with the original on a repeated `client_msg_id` |
| `PATCH /chat/messages/{id}` | `{body}` | `Message` |
| `DELETE /chat/messages/{id}` | | 204 |
| `POST /chat/channels/{id}/read` | `{seq}` | 204 |
| `POST /chat/messages/{id}/reactions` | `{emoji}` | 204 |
| `DELETE /chat/messages/{id}/reactions/{emoji}` | | 204 |
| `POST /chat/uploads` | `{channel_id, file_name, size, mime_type}` | 201 `{attachment_id, upload: {url, method, headers, expires_at}}` |
| `GET /chat/attachments/{id}` | | `{url, expires_at}` (presigned download) |

Shapes:

- `Channel {id, type: public|private|dm, name, topic, last_seq, last_read_seq, unread_count, mention_count, member_count, dm_user_ids, archived_at, created_at}`
- `ChannelMember {user_id, display_name, joined_at}`
- `Message {id, channel_id, seq, sender_id, client_msg_id, kind, body, payload, fallback_text, thread_root_id, reply_count, attachments: Attachment[], reactions: {emoji, count, me}[], mentions: string[], mentions_channel, edited_at, deleted_at, created_at}`; `kind` is `text` or a card kind. A deleted message keeps its row and seq, with empty body and `deleted_at` set.
- `Attachment {id, file_name, size, mime_type, is_image}`

Rules:

- `seq`: in the send transaction, `UPDATE chat_channels SET last_seq = last_seq + 1 ... RETURNING last_seq`, then insert. A rolled-back send leaves no gap. Unique `(channel_id, seq)` and `(sender_id, client_msg_id)`.
- A repeated `client_msg_id` returns the stored message and does not publish again (also when two sends race: catch the unique violation and read the original).
- Mentions are tokens in the body: `<@user_id>` and `<!channel>`. The server stores the parsed user IDs (members of the channel only).
- Body is markdown, 1–4,000 characters (CHAT-04); attachments only messages may have an empty body.
- Public channels: any active workspace member may read and join. Private channels and DMs: members only; everyone else gets 404, not 403, so their existence does not leak. Guests see only channels they were added to.
- Edit and delete: own messages; admins and owners may delete anyone's (CHAT-10).
- Threads are one level deep: a reply's root must be a top-level message in the same channel. Replies get their own `seq` in the channel.
- Uploads: at most 50 MB, key `chat/{workspace_id}/{attachment_id}`, presigned for 15 minutes (NFR-07). Sending with `attachment_ids` checks the object exists (`Stat`) and belongs to the sender and channel.
- Events (`chat/events.go`): `chat.message_created {workspace_id, channel_id, message_id, seq, sender_id, channel_type, thread_root_id, mentions, mentions_channel}`. Realtime delivery to channel members happens after the transaction commits.
- chat listens to `workspace.member_removed` and removes that user from the workspace's channels.

### M2 waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 1 | M2-01 realtime hub, M2-02 storage, M2-03 chat core, M2-04 core sync engine | 4 parallel workers |
| Checkpoint C | Planner merges, wires realtime, storage and chat into `cmd/server` | Planner |
| 2 | M2-05 chat extras (uploads, reactions, cards, typing, presence), M2-06 web chat UI | 2 parallel workers |
| 3 | M2-07 AUTH-08, acceptance test | 1 worker |
| Checkpoint M2 | SRS 16.3 M2 acceptance | Planner |

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
