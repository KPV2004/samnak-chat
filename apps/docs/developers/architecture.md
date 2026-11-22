# Architecture

This page is a summary for people who change the code. The full document, with diagrams and the reasons for each decision, is [`docs/architecture.md`](https://github.com/KPV2004/samnak-chat/blob/main/docs/architecture.md) in the repository. It marks what is planned but not built.

## Shape of the system

Samnak Chat is one Go program, a **modular monolith**. Each feature is a module, and a module can be switched on or off. The web app is a React and Vite client that uses only the public API and one WebSocket.

```
Clients: web (React + Vite)          planned: desktop (Tauri), mobile (Expo)
    │  REST + WebSocket
Server (Go): HTTP API and WebSocket hub
    ├── core modules:     auth, workspace, chat
    └── optional modules: notify, task, approval, helpdesk
    │
    ├── PostgreSQL   data, the event outbox, background jobs (River)
    ├── Redis        presence and fan-out between server instances
    └── MinIO / S3   attachments, through presigned URLs
```

Inside each module, the code follows Clean Architecture: `adapter → usecase → domain`. The business rules in `domain` know nothing about SQL, HTTP or JSON.

## Server layout

| Folder | Contents |
| --- | --- |
| `cmd/server` | The composition root. It wires the modules and starts the server. |
| `cmd/migrate` | Runs the migrations of the enabled modules |
| `internal/app` | Shared contracts: interfaces and types only. It imports nothing else from the project. |
| `internal/platform` | Infrastructure: configuration, database, Redis, storage, mail, the event bus, jobs, rate limits, realtime, HTTP and metrics. It knows no feature. |
| `internal/modules` | One folder per module |
| `internal/archtest`, `internal/acceptance` | Tests for forbidden imports, and tests of several modules over HTTP |

## How modules talk

Modules do not call each other's code. They use four mechanisms, in order of preference:

| Mechanism | Used to |
| --- | --- |
| Event bus | Announce that something happened. The publisher does not know who listens. |
| Notification registry | Say who should be told, and what the message says. The notify module decides whether the user gets it. |
| Card registry | Show a card in chat. The chat module stores and delivers cards without reading their payload. |
| Service registry | Ask another module for a result right away. Used rarely. |

A module imports only `api.go` and `events.go` from another module.

## Events and the outbox

An event is a struct in the publishing module's `events.go`, named in the past tense, such as `approval.request_approved`. Inside a transaction, an event is written to the `platform_outbox_events` table in the same transaction as the data. It is delivered only after the commit, so a rolled-back change delivers nothing.

Delivery is at least once, so handlers must be idempotent. A handler that fails is logged and does not affect the publisher.

## Data

- Every table that holds organization data has a `workspace_id` column. Use cases take the workspace from the signed-in user's membership, never from the request body.
- Table names start with the module name, such as `chat_messages`. No module reads or writes another module's tables.
- IDs are UUIDv7. Timestamps are `timestamptz` in UTC.
- Modules refer to each other's data by ID only, with no foreign keys across modules.
- Migrations live in each module's folder and use goose.

## Messages and realtime

A message carries a client-made `client_msg_id`, and the database rejects a second copy from the same sender. The server gives each message a per-channel `seq`, so every device shows the same order. After a reconnect, the client asks the REST API for the messages after the last `seq` it has.

The WebSocket does not replay history. Each server instance keeps its own connections in memory and fans out messages through Redis pub/sub. Presence is stored in Redis with an expiry, and clients refresh it with `ping` messages.

## Background work

The job queue is River, which stores jobs in PostgreSQL, so a job can be queued in the same transaction as the data it belongs to. Periodic jobs cover task due dates, notification cleanup and outbox cleanup. Push notifications and SLA checks are planned, not built.

## Authentication and limits

- The access token is a JWT that lives 15 minutes. The refresh token lives 30 days and rotates on every use. Reusing an old refresh token revokes the device it belongs to.
- Sign-in allows 5 failed attempts a minute for each email address and for each IP address.
- Every `/api/v1` call is rate limited: per user when signed in, per client IP otherwise. See [Configuration](/configuration#rate-limits).
- Permissions are checked in the use case layer with an `Authorizer`. A module adds its own actions when it starts.

## Files

Uploads and downloads go straight from the browser to the object store, through presigned URLs that work for 15 minutes. The server checks each upload with `Stat` before it attaches the file to a message, a request or a ticket. A module writes and deletes only its own object keys.

## Clients

The web app is in `apps/web`. `packages/core` holds the API client, the generated OpenAPI types and the WebSocket connection, with no DOM or browser code, so the planned mobile app can share it. Clients render cards by `kind`, and show `fallback_text` for a kind they do not know.

## Decisions

The reasons for the main choices are in the [decision records](/developers/decisions).
