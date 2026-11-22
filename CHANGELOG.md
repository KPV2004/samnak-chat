# Changelog

All notable changes to Samnak Chat are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Documentation site in `apps/docs`, published with the landing page at `/docs/` (Thai at `/docs/th/`). It covers install, configuration, operations, the user guide for every module, the API reference generated from the OpenAPI file, and the developer pages. The landing page links to it in English and Thai.

## [0.1.0] - 2026-11-21

The first release. It covers milestones M0 to M6 of the plan in [`tasks/plan.md`](tasks/plan.md), and the web redesign that followed.

### Added

**Server and platform**

- Go modular monolith with a module registry. Modules are enabled with `MODULES_ENABLED`, and a missing dependency stops the server at startup.
- Transactional outbox for events, so an event is published only after its transaction commits.
- Background jobs on River, stored in PostgreSQL.
- WebSocket hub with Redis fan-out between server instances, and presence.
- Presigned upload and download URLs for MinIO or S3, valid for 15 minutes.
- Email through SMTP, or written to the server log when `SMTP_HOST` is empty.
- Health checks at `/healthz` and `/readyz`, and Prometheus metrics at `/metrics`.
- Per-module database migrations, and an OpenAPI document generated from the code.

**Accounts and workspaces** (`auth`, `workspace`)

- Sign-up, sign-in, sign-out, and profile.
- Access tokens that last 15 minutes, and refresh tokens that last 30 days and rotate on every use.
- A list of signed-in devices, and the option to sign out a device.
- Email verification and password reset.
- Workspaces, members with roles, and invites.
- Groups and managers in a workspace.
- An audit log of workspace actions.

**Chat** (`chat`)

- Public and private channels, and direct messages.
- Messages with threads, edits, deletes, reactions, pins and read state.
- File attachments through presigned URLs.
- Typing indicators and presence.
- Slash commands and message actions, which other modules can add.
- Pinned channels and a last-message preview in the channel list.

**Notifications** (`notify`)

- An inbox with an unread count, and a bell in the web app.
- Notices from chat, tasks, approvals and helpdesk, sent over the WebSocket.
- Per-kind switches, muted channels, and do-not-disturb hours.

**Tasks** (`task`)

- Tasks with due dates.
- Reminders for tasks due within 24 hours and for overdue tasks.
- Task cards in chat.

**Approvals** (`approval`)

- Approval requests with approve, reject and cancel.
- Attachments on requests, and request cards in chat.

**Helpdesk** (`helpdesk`)

- Queues, categories, agents, and tickets with assignment and status.
- Public and internal comments on tickets.
- Ticket cards in chat, a `/ticket` command, and a ticket made from a chat message.
- Attachments on tickets.

**Web app** (`apps/web`, `packages/core`)

- Messenger with a conversation list, filters, pins, and search.
- Chat view with grouped messages, threads in a side pane, a composer with formatting, and mentions.
- Pages for tasks, approvals, helpdesk, groups, members, invites, the audit log, notification settings, profile and devices.
- Thai and English, a light and a dark theme, and a layout for phones.
- Sign-in, sign-up, email verification and password reset pages.

**Self-hosting and delivery**

- `deploy/compose.prod.yml`: the web app behind Caddy with automatic HTTPS when `DOMAIN` is set, the API server, PostgreSQL, Redis and MinIO.
- Server and web images built for `linux/amd64` and `linux/arm64`, published to GHCR on `v*` tags.
- A module check in CI that starts the server with each module set.
- A coverage gate in CI: 80% of the statements in domain and use-case packages.

### Security

- A refresh token that is used again after rotation revokes the device it belongs to.
- Failed sign-ins are limited per email address and per IP address.
- Every API call is rate limited, per user when signed in and per client IP otherwise, with 429 and `Retry-After` over the limit.
- Every request field has a declared length, size or count limit, and a test fails when a new field has none.
- A test checks that every workspace-scoped endpoint refuses a member of another workspace.
- `/metrics` is not published through the proxy.
- Workspace identity comes from the signed-in user's membership, never from the request body.
