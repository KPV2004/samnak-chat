# Self-hosting Samnak Chat

This installs the whole app on one machine with Docker Compose: the web app behind Caddy, the API server, PostgreSQL, Redis and MinIO (file storage). Images are built from this repository, for `linux/amd64` and `linux/arm64`.

## Requirements

- Docker with Compose v2: Docker Desktop, OrbStack, or Docker Engine with the Compose plugin
- Git
- About 2 GB of free memory and 5 GB of disk for the images and data

## Install

```sh
git clone https://github.com/KPV2004/samnak-chat.git
cd samnak-chat
cp .env.example .env
```

Edit `.env`:

| Setting | What to set |
| --- | --- |
| `JWT_SECRET` | A long random string, for example the output of `openssl rand -hex 32`. Required. |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | Random values. |
| `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` | The same values as `S3_ACCESS_KEY` and `S3_SECRET_KEY`. |
| `PUBLIC_URL` | The address people open, with the port if it is not 80 or 443, for example `http://localhost` or `https://chat.example.com`. Email links use it, and browsers upload and download files through it. |
| `POSTGRES_PASSWORD` | A random value. Set it before the first start: the database keeps the password it was created with. |
| `DOMAIN` | Your host name, for example `chat.example.com`, for automatic HTTPS. Ports 80 and 443 must reach this machine and the name must point to it. Leave the line as `DOMAIN=` for plain HTTP. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | Your mail server. Leave `SMTP_HOST` empty to write mail to the server log instead. |

Then start the stack:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
```

The first start builds the images, which takes several minutes. `--wait` returns when every service is healthy. The `migrate` service runs the database migrations and exits with status 0 before the server starts.

Open `http://localhost` (or your `PUBLIC_URL`), register an account, and create a workspace.

### Ports

Caddy publishes port 80 and port 443. To use other host ports, set them in your shell before the command, and make `PUBLIC_URL` carry the port, since file links are signed for that address:

```sh
HTTP_PORT=8080 HTTPS_PORT=8443 PUBLIC_URL=http://localhost:8080 docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
```

## Operate

```sh
docker compose --env-file .env -f deploy/compose.prod.yml ps                 # status of each service
docker compose --env-file .env -f deploy/compose.prod.yml logs -f server     # server log (JSON lines)
docker compose --env-file .env -f deploy/compose.prod.yml down               # stop; data is kept
```

`down -v` also deletes the volumes, which hold all data. Do not run it unless you mean to.

**Upgrade:** pull the new code, then rebuild. Migrations run before the new server starts.

```sh
git pull
docker compose --env-file .env -f deploy/compose.prod.yml up -d --build --wait
```

**Back up the database:**

```sh
docker compose --env-file .env -f deploy/compose.prod.yml exec postgres pg_dump -U samnak samnak > samnak-backup.sql
```

Attachments are in the `miniodata` volume.

**Metrics** (`/metrics`, Prometheus format) are served by the server on the internal network only. Caddy does not forward them. To read them on the host:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml exec web wget -qO- http://server:8080/metrics
```

## Known limits in v0.1

- **The MinIO bucket is `samnak`**; keep `S3_BUCKET=samnak`.
- **Images are not pulled from a registry.** `up` builds them from this checkout. Release images are published to GHCR on `v*` tags (`ghcr.io/kpv2004/samnak-chat/server` and `ghcr.io/kpv2004/samnak-chat/web`), but this file does not use them yet.
