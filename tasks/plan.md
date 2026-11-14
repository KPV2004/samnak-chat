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
