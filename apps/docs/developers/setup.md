# Development setup

This page gets a development copy of Samnak Chat running on your machine: the API server, the web app and the services they need. For running the production stack on a server, see [Install on your server](/install).

## What you need

- Go 1.26 or later
- Node 22 or later
- Docker, for PostgreSQL, Redis, MinIO and Mailpit
- Git
- `golangci-lint`, only if you run `make lint`

## Start

```sh
git clone https://github.com/KPV2004/samnak-chat.git
cd samnak-chat
cp .env.example .env
npm install
make up          # PostgreSQL, Redis, MinIO and Mailpit in Docker
make migrate     # create the tables of the enabled modules
make server      # API on http://localhost:8080
make web         # web app on http://localhost:5173
```

Open `http://localhost:5173`. The web app proxies `/api`, including the WebSocket at `/api/v1/ws`, to the API on port 8080.

`MODULES_ENABLED` in `.env` decides which optional modules run. The `.env.example` file enables `notify`, `task`, `approval` and `helpdesk`. The web app shows only the menus of the modules the server reports.

## Read the emails

With `SMTP_HOST=localhost` in `.env`, the server sends mail to Mailpit. Open its inbox at `http://localhost:8025`. With `SMTP_HOST` empty, the server writes each email to its log instead.

## Commands

| Command | What it does |
| --- | --- |
| `make up` | Start PostgreSQL, Redis, MinIO and Mailpit with `deploy/docker-compose.yml` |
| `make down` | Stop those services. Data is kept |
| `make migrate` | Run the migrations of the enabled modules |
| `make server` | Run the API server on port 8080 |
| `make web` | Run the web app on port 5173 |
| `make openapi` | Regenerate `apps/server/api/openapi.yaml` and the types in `packages/core` |
| `make test` | `go test ./...` in `apps/server`, then `npm test` |
| `make lint` | `go vet`, `golangci-lint` and `npm run lint` |
| `npm run build` | Type-check and build the web app and the packages |
| `bash scripts/coverage.sh [80]` | Print the coverage of every domain and use-case package, and fail below the minimum |
| `bash scripts/check-modules.sh <MODULES_ENABLED or unset> <optional modules>` | Start the server with one module set, then check `/healthz`, `/meta` and the routes of the optional modules |

For example, `bash scripts/check-modules.sh task task` starts the server with `MODULES_ENABLED=task` and checks that only the task routes are mounted.

## Work on this documentation site

The site is in `apps/docs`. It is built with VitePress.

```sh
npm run docs:dev -w apps/docs     # local preview with live reload
npm run docs:build -w apps/docs   # build to apps/docs/.vitepress/dist
```

Two pages come from files in the repository and are never edited by hand. The API reference is built from `apps/server/api/openapi.yaml`, and the changelog from `CHANGELOG.md`. The build writes them first, so change the source file instead. The build fails on dead links, so check your links before you push.
