# Samnak Chat

Self-hosted chat for organizations, with work management built in: tasks, approval requests and a helpdesk, all usable from inside a conversation.

> Status: **M0 – project skeleton**. Not usable yet. See [`tasks/plan.md`](tasks/plan.md).

## Repository layout

```
apps/server/     Go modular monolith (Chi, Huma, pgx, sqlc, River)
apps/web/        React + Vite + TypeScript (see apps/web/README.md)
apps/desktop/    Tauri (phase P2)
apps/mobile/     Expo (phase P3)
packages/core/   API client, OpenAPI types, WebSocket, chat logic (no DOM)
packages/ui/     Components shared by web and desktop
deploy/          Docker Compose for local dev and self-hosting
docs/            SRS, architecture, ADRs
tasks/           Plan and task board
```

## Quick start (dev)

Requires Go 1.26+, Node 22+, and Docker.

```sh
cp .env.example .env
make up        # postgres, redis, minio
make server    # API on :8080
make web       # web on :5173
```

## Docs

- [Requirements (SRS, Thai)](docs/srs.md)
- [Architecture](docs/architecture.md)
- [Decision records](docs/adr/)
- [Contributing](CONTRIBUTING.md)

## License

[AGPL-3.0](LICENSE)
