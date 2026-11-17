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

### Event payloads (fixed at checkpoint C; the server's shapes are canonical)

| Type | `data` |
| --- | --- |
| `message.created`, `message.updated`, `message.deleted` | `{workspace_id, channel_id, message: Message}` (a deleted message has empty `body` and `deleted_at`; a reply also sends `message.updated` for its root so `reply_count` stays current) |
| `channel.read` | `{workspace_id, channel_id, last_read_seq, unread_count, mention_count}` (only to the reader's own devices) |
| `channel.updated` | `{workspace_id, channel_id, type, name, topic, last_seq, member_count, dm_user_ids, archived_at, created_at}` (no per-user counts) |
| `card.updated` | `{workspace_id, channel_id, message_id, payload}` |
| `typing` | `{workspace_id, channel_id, user_id}` (client sends `{channel_id}`; the server fills the rest and fans out to other members) |
| `presence.changed` | `{user_id, online}` |
| `error` | `{type, code, message}` (hub reply to a bad client frame; the connection stays open) |

`GET .../messages` pages are always ascending by `seq`. With no cursor (or `before_seq=0`) it returns the newest page. Close codes: 4401 auth failed or device revoked (refresh the token, then reconnect), 1011 server error (reconnect), 1008 idle, 1001 shutdown.

### M2 waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 1 | M2-01 realtime hub, M2-02 storage, M2-03 chat core, M2-04 core sync engine | 4 parallel workers |
| Checkpoint C | Planner merges, wires realtime, storage and chat into `cmd/server` | Planner |
| 2 | M2-05 chat extras (uploads, reactions, cards, typing, presence), M2-06 web chat UI | 2 parallel workers |
| 3 | M2-07 AUTH-08, acceptance test | 1 worker |
| Checkpoint M2 | SRS 16.3 M2 acceptance | Planner |

## M3: notifications and the job queue (SRS NOTI-01..06, PLT-09)

### Contracts the planner fixed before M3

- `app.Notification` gains `Key`: notify keeps at most one notification per `(user_id, key)`, so a redelivered event adds nothing. chat uses `chat.message:{message_id}`.
- `app.NotificationRules` (`internal/app/notification.go`) collects rules. `cmd/server` always passes one as `Deps.Notifications`; modules call `On` in `Init`. notify reads them through `app.NotificationRuleSource` in `Subscribe`, after every `Init` ran, and subscribes one bus handler per event name. With notify disabled nobody reads the rules, so chat runs exactly as before (NOTI-05).
- notify is no longer a core module. `app.DefaultModules = ["notify"]` is used when `MODULES_ENABLED` is unset; `MODULES_ENABLED=` (set, empty) means core only. `config.ParseModules` is shared by `cmd/server` and `cmd/migrate`, and `.env.example` sets `MODULES_ENABLED=notify`. This is the switch the SRS 16.3 M3 check ("turn notify off, chat still works") needs.
- `app.Jobs` (`internal/app/jobs.go`): `Handle(kind, handler, opts)` and `Every(interval, kind, args)` in `Init`, `Enqueue(ctx, kind, args, opts)` at run time; inside `TxManager.Do` the insert joins the transaction (PLT-09). `Deps.Jobs` is set by the server and may be nil in tests and `server openapi`.

### Job queue (`internal/platform/jobs`, River)

- One River client per server on the shared pgx pool, started after `reg.Start` and stopped on shutdown after HTTP drains (NFR-11). River's own migrations run from `cmd/migrate` as part of the platform set.
- Kinds are dynamic strings, so one generic args type carries `{kind, json}`; the worker dispatches to the handler registered for that kind.
- Retries: River's backoff, `MaxAttempts` default 5 (NOTI-08), timeout default 1 min.
- No worker: at start and every minute, jobs in `available`, `scheduled` or `retryable` whose kind has no `Handle` on this server are set to `cancelled` (PLT-09, architecture section 7). All instances run the same module set.
- `Every` uses River periodic jobs with a leader, so one instance enqueues.
- Joining the transaction needs the `pgx.Tx` that `database.TxManager` keeps in the context; `platform/database` exports `TxFromContext`.

### notify module (`internal/modules/notify`)

Table `notify_notifications (id uuid v7, workspace_id, user_id, kind, title, body, link, data jsonb, key, read_at, created_at)`, unique `(user_id, key)` where key is not empty, index `(user_id, workspace_id, created_at desc)` and a partial index for unread.

Flow per event: run each rule (a rule that errors or panics is logged and skipped; it never affects the publisher or other rules) → drop notifications without `UserID`, `WorkspaceID` or `Kind` → insert with `ON CONFLICT (user_id, key) DO NOTHING` in one transaction (NOTI-02) → after commit, `Realtime.SendToUsers` a `notification.created` for each row actually inserted (NOTI-03). Offline users simply find it in the list; push is P3.

API (prefix `/notify`, all `RequireMember`, scoped to the `X-Workspace-ID` workspace and the caller):

| Method and path | Body or query | Response |
| --- | --- | --- |
| `GET /notify/notifications` | `?unread=true`, `?before=<id>` (cursor, newest first), `?limit` ≤ 50 (default 20) | `{items: Notification[], has_more}` |
| `GET /notify/unread-count` | | `{count}` |
| `POST /notify/notifications/{id}/read` | | 204 (idempotent; another user's id is 404) |
| `POST /notify/read-all` | | 204 |

`Notification {id, kind, title, body, link, data, read_at, created_at}`.

WebSocket (server → client), both only to the user's own devices:

| Type | `data` |
| --- | --- |
| `notification.created` | `{workspace_id, notification: Notification, unread_count}` |
| `notification.read` | `{workspace_id, ids: string[], all: bool, unread_count}` (so other devices clear the bell) |

Job: `notify.prune` every 24 h deletes read notifications older than 90 days.

### chat notification rules (NOTI-06)

chat registers one rule on `chat.message_created`. For each recipient, at most one notification per message, by priority mention > DM > thread:

- `chat.mention`: users in `mentions`, and every channel member for `<!channel>`.
- `chat.dm`: the other members of a DM.
- `chat.thread_reply`: for a reply, the root's author and everyone who already replied in the thread (the "followed thread").
- Never the sender, never a user who is no longer a channel member.
- `Title` is the sender's display name and the channel (`#name` or "DM"), `Body` a plain-text excerpt of at most 140 characters with `<@id>` rendered as `@name`. `Link` is `/chat/{channel_id}`; `Data` carries `channel_id, message_id, seq, thread_root_id`. `Key` is `chat.message:{message_id}`.
- The web renders known kinds from `data` with its own i18n and falls back to `title`/`body`.

### M3 waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 0 | M3-00 contracts (this section) | Planner |
| 1 | M3-01 platform/jobs, M3-02 notify module, M3-03 chat rules, M3-04 web bell | 4 parallel workers (disjoint files) |
| Checkpoint D | Planner merges, registers notify in `cmd/server` and `cmd/migrate`, regenerates OpenAPI and web types | Planner |
| 2 | M3-05 M3 acceptance test | 1 worker |
| Checkpoint M3 | SRS 16.3 M3 acceptance, two browsers | Planner |

## M4: tasks, approvals, chat extensions, groups and the outbox (SRS TASK-01..08, APR-01..09, 13, CHAT-16, WSP-09, PLT-10, 12)

### Contracts the planner fixed before M4

- `app.EventBus.Declare(protos ...Event)` and the outbox rule: `Publish` outside a transaction runs handlers now (unchanged); inside `TxManager.Do` it writes `platform_outbox_events` in that transaction and the event is delivered after commit, at least once, on one instance (PLT-10). A module that publishes inside a transaction declares those event types in `Init`. The in-memory bus accepts `Declare` and ignores it.
- `app.ChatExtensions` (`internal/app/chat.go`): `RegisterCommand(Command)` and `RegisterMessageAction(MessageAction)`, provided by chat; modules register in `Init` when the lookup succeeds (CHAT-16).
- `app.ChannelAccess.CanRead(ctx, workspaceID, channelID, userID)`, provided by chat, so task shows a task only to people who can read its channel (TASK-05).
- `workspace.GroupDirectory.ManagersOf(ctx, workspaceID, userID)` in `workspace/api.go`, provided by workspace (WSP-09, APR-02).
- Unchanged and used: `app.CardRegistry` (Post, Update, DirectChannel), `auth.Directory` (display names), `workspace.Authorizer` and `ActionRegistry`, `app.Jobs`, `Deps.Notifications`, `Deps.Storage`.

### Rules for modules that own cards (task, approval)

- Write the row and `Publish` the event inside one `TxManager.Do`. Post or update the card in your own handler of that event, after commit, never inside the transaction (chat sends realtime frames as soon as its own call returns). The handler is idempotent: posting a card stores its `message_id` with `UPDATE ... WHERE message_id IS NULL`, and a redelivered event finds it set and only updates the payload.
- Each card payload carries what the web needs to render it without another request, plus a `version` (the row's `updated_at` in Unix ms) so a late `card.updated` never overwrites a newer one on the client.
- Card buttons call the owning module's REST API; chat has no generic action endpoint.

### Outbox (`internal/platform/eventbus`, PLT-10)

- `Publish` inside a transaction (`database.TxFromContext`) inserts `{id uuidv7, name, payload jsonb}` and runs `SELECT pg_notify('platform_outbox', '')` in the same transaction; Postgres delivers the notification only on commit.
- A relay per instance `LISTEN`s and also polls every 2 s. It claims a batch with `SELECT ... WHERE published_at IS NULL ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 100`, decodes each by its declared type, runs the local handlers, and sets `published_at` in that claim transaction. A handler error is logged (like today) and the event still counts as delivered; a crash before the commit redelivers. An undeclared name is logged and left unpublished.
- `notify.prune`-style cleanup: delete published rows older than 7 days (a `platform.outbox_prune` job every 24 h).
- `cmd/server` builds the bus with the pool, starts the relay after `reg.Start` and stops it after HTTP drains.

### Chat extensions (chat, CHAT-16)

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /chat/extensions` | | `{commands: [{name, usage, description, module}], message_actions: [{id, module}]}` |
| `POST /chat/channels/{id}/commands` | `{name, args, thread_root_id?}` | 200 `{text}`; unknown command 404; the caller must be able to read the channel (404 otherwise) |

Handlers run with the caller's `Principal` in `ctx`. Command names are unique; registering a taken name panics at startup.

### Groups (workspace, WSP-09)

Tables `workspace_groups (id, workspace_id, name, created_at)` with unique `(workspace_id, lower(name))`, and `workspace_group_members (group_id, user_id, is_manager, manager_since, added_at)`.

| Method and path | Body | Response | Who |
| --- | --- | --- | --- |
| `GET /workspace/groups` | | `{items: [{id, name, member_count, manager_ids}]}` | members |
| `POST /workspace/groups` | `{name}` | 201 group | owner, admin (`workspace.group.manage`) |
| `PATCH /workspace/groups/{id}` | `{name}` | group | owner, admin |
| `DELETE /workspace/groups/{id}` | | 204 | owner, admin |
| `GET /workspace/groups/{id}/members` | | `{items: [{user_id, display_name, is_manager}]}` | members |
| `PUT /workspace/groups/{id}/members` | `{members: [{user_id, is_manager}]}` (replaces the set; only active members of the workspace) | 204 | owner, admin |

Removing a member from the workspace removes them from its groups.

### task module (`internal/modules/task`)

Table `task_items (id, workspace_id, channel_id, source_message_id, message_id, title, description, creator_id, assignee_id, due_at, priority low|normal|high, status todo|doing|done, due_soon_sent_at, overdue_sent_at, created_at, updated_at)`. Changing `due_at` clears both `*_sent_at`.

| Method and path | Body or query | Response |
| --- | --- | --- |
| `POST /task/items` | `{channel_id, title (1-200), description? (≤ 4000), assignee_id?, due_at?, priority? (normal), source_message_id?}` | 201 `Task` |
| `GET /task/items` | `?assignee=me\|any`, `?creator=me`, `?status=todo,doing`, `?channel_id=`, `?due=overdue\|today\|week`, `?cursor=&limit=` (≤ 100) | `{items: Task[], next_cursor}`; only tasks whose channel the caller can read |
| `GET /task/items/{id}` | | `Task` (404 when the channel is not readable) |
| `PATCH /task/items/{id}` | any of `{title, description, assignee_id (null clears), due_at (null clears), priority, status}` | `Task` |

`Task {id, channel_id, message_id, source_message_id, title, description, creator_id, assignee_id, due_at, priority, status, created_at, updated_at}`.

- The caller must be able to read the channel to create; the assignee must be an active workspace member who can read the channel (400 otherwise).
- Edit (TASK-05): creator, assignee, owner or admin; others who can read get 403. Status changes follow the same rule.
- Events (`task/events.go`, published in the transaction, declared): `task.created {workspace_id, task_id, channel_id, creator_id, assignee_id}`, `task.assigned {workspace_id, task_id, assignee_id, actor_id}` (on create with an assignee and on a change of assignee), `task.status_changed {workspace_id, task_id, from, to, actor_id}`, `task.updated {workspace_id, task_id}` (any other field), `task.due_soon` and `task.overdue {workspace_id, task_id, assignee_id, due_at}` (TASK-08).
- Card kind `task.item`, posted in `channel_id` by the creator on `task.created`; payload `{task_id, title, status, priority, assignee_id, assignee_name, due_at, version}`; fallback text `Task: {title}`. Every later event updates the payload. The card's thread is the task's comments (TASK-07); nothing extra on the server.
- Notification rules: `task.assigned` → the assignee (not when they assigned themselves); `task.due_soon` and `task.overdue` → the assignee. Keys `task.assigned:{task_id}:{assignee_id}:{updated_at}`, `task.due_soon:{task_id}:{due_at}`, `task.overdue:{task_id}:{due_at}`.
- Job `task.due_scan` every 5 minutes: tasks not done, with an assignee and `due_at`, publish `task.due_soon` when `due_at - 24h <= now < due_at` and `due_soon_sent_at` is null, and `task.overdue` when `due_at <= now` and `overdue_sent_at` is null, setting the column in the same transaction (TASK-06).
- Chat extensions: command `/task <title>` (an optional `<@user>` token anywhere sets the assignee; the rest is the title) creates a task in that channel and returns `{text: ""}`; message action `task.create_from_message` (the web opens the create form with the message text as the title and its ID as `source_message_id`).

### approval module (`internal/modules/approval`)

Tables `approval_requests (id, workspace_id, type leave|general, requester_id, approver_id, status pending|approved|rejected|cancelled, payload jsonb, channel_id, message_id, decided_at, reason, created_at, updated_at)`, `approval_steps (request_id, step_no, approver_id, status, reason, decided_at)` (one step in v0.1; APR-11 is P4), `approval_history (id, request_id, actor_id, action created|approved|rejected|cancelled, reason, at)` (APR-07), `approval_attachments (id, request_id null until submit, workspace_id, uploader_id, object_key, file_name, size, mime_type, created_at)`.

Payloads (APR-01): `leave {leave_type annual|sick|personal|other, start_date, end_date (dates, end ≥ start), half_day? morning|afternoon (only when start = end), reason (1-1000)}`; `general {subject (1-200), details (≤ 4000), amount? (decimal string, ≥ 0, 2 places), currency? (default THB)}`.

| Method and path | Body or query | Response |
| --- | --- | --- |
| `POST /approval/requests` | `{type, payload, approver_id?, attachment_ids?}` | 201 `Request`; without `approver_id` the first of `GroupDirectory.ManagersOf` (400 `approver_required` when none); the approver must be an active member other than the requester |
| `GET /approval/requests` | `?role=requester\|approver`, `?status=`, `?cursor=&limit=` | `{items: Request[], next_cursor}` (APR-06) |
| `GET /approval/requests/{id}` | | `Request` with `history` and `attachments`; visible to the requester and approver, else 404 |
| `POST /approval/requests/{id}/approve` | | `Request`; caller not the approver → 403; not pending → 409 |
| `POST /approval/requests/{id}/reject` | `{reason (1-1000)}` | same rules; empty reason → 400 (APR-04) |
| `POST /approval/requests/{id}/cancel` | | requester only (403), pending only (409) (APR-05) |
| `POST /approval/uploads` | `{file_name, size (≤ 50 MB), mime_type}` | 201 `{attachment_id, upload: {url, method, headers, expires_at}}`; key `approval/{workspace_id}/{attachment_id}` (APR-09) |
| `GET /approval/attachments/{id}` | | `{url, expires_at}` for the requester and approver |

`Request {id, type, status, requester_id, approver_id, payload, reason, decided_at, created_at, updated_at, message_id}`.

- Events (published in the transaction, declared): `approval.request_created {workspace_id, request_id, type, requester_id, approver_id}`, `approval.request_approved`, `approval.request_rejected` (+ `reason`), `approval.request_cancelled`, each with `actor_id` (APR-13).
- Card kind `approval.request`, posted on `approval.request_created` in the DM between requester and approver (`DirectChannel`), sent as the requester (APR-03); payload `{request_id, type, status, summary, fields, requester_id, requester_name, approver_id, approver_name, reason, decided_at, version}` where `summary` is the leave range or the subject and `fields` the payload. Fallback `Approval request: {summary} ({status})`. The decision events update it, so both people see the new status at once.
- Notification rules (APR-08): created → approver (`approval.requested`); approved and rejected → requester (`approval.approved`, `approval.rejected`); cancelled → approver. Key `approval:{request_id}:{action}`.
- Actions registered with `ActionRegistry` are not needed; the approver rule is the module's own domain rule.

### Web (wave 2)

- Card renderers in `apps/web/src/chat/cards.tsx`'s registry: `task.item` (title, assignee, due, priority, a status select that PATCHes, "Comments" opens the thread) and `approval.request` (summary, fields, status badge; Approve and Reject with a reason prompt for the approver while pending; Cancel for the requester). A 403 or 409 shows the server's message and refetches.
- Composer: typing `/` lists `GET /chat/extensions` commands; Enter on a known command sends `POST .../commands`; an unknown command is sent as text. Message menu: "Create task" for `task.create_from_message`.
- Pages: "My tasks" (`/tasks`, filters status, due and channel), "Approvals" (`/approvals`: new request form for leave and general with approver picker defaulting to the server's choice, tabs "My requests" and "Waiting for me", detail with history and attachments), "Groups" (`/groups`, admin edits, members see).
- Navigation hides each page when `/meta` does not list its module.

### PLT-12

CI starts the server with `MODULES_ENABLED` unset, empty, `task`, `approval` and `notify,task,approval`, then checks `/healthz`, `/meta` and a 404 for a disabled module's route.

### M4 waves

| Wave | Tasks | Run as |
| --- | --- | --- |
| 0 | M4-00 contracts (this section) | Planner |
| 1 | M4-01 outbox, M4-02 groups, M4-03 chat extensions, M4-04 task, M4-05 approval | 5 parallel workers (disjoint files) |
| Checkpoint E | Planner merges, registers task and approval, regenerates OpenAPI and web types, live API run | Planner |
| 2 | M4-06 web: extensions and tasks, M4-07 web: approvals and groups | 2 parallel workers |
| 3 | M4-08 acceptance test and PLT-12 CI matrix | 1 worker |
| Checkpoint M4 | SRS 16.3 M4 acceptance in two browsers | Planner |

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
