# Install on your server

This runs the whole app on one machine with Docker Compose. The stack has five services:

- **web**: the web app, served by Caddy, which also proxies the API and handles HTTPS
- **server**: the API and WebSocket server
- **postgres**: the database (PostgreSQL 17)
- **redis**: presence and realtime fan-out between server instances
- **minio**: file storage for attachments, with an S3-compatible API

The `migrate` service runs the database migrations once, before the server starts.

## What you need

- Docker with Compose v2: Docker Desktop, OrbStack, or Docker Engine with the Compose plugin
- Git
- About 2 GB of free memory and 5 GB of disk for the images and data

The compose file builds the images from the code you clone. The first start takes several minutes, mostly for the build.

## Install

```sh
git clone https://github.com/KPV2004/samnak-chat.git
cd samnak-chat
cp .env.example .env
```

Open `.env` and set these values. The full list is on the [configuration](/configuration) page.

| Setting | What to set |
| --- | --- |
| `JWT_SECRET` | A long random string, for example the output of `openssl rand -hex 32`. Required. |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | Random values |
| `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` | The same values as `S3_ACCESS_KEY` and `S3_SECRET_KEY` |
| `PUBLIC_URL` | The address people open, with the port if it is not 80 or 443, for example `http://localhost` or `https://chat.example.com`. Email links use it, and browsers upload and download files through it. |
| `POSTGRES_PASSWORD` | A random value. Set it before the first start: the database keeps the password it was created with. |
| `DOMAIN` | Your host name, for example `chat.example.com`, for automatic HTTPS. Leave it empty for plain HTTP. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | Your mail server. Leave `SMTP_HOST` empty to write mail to the server log instead. |

Then start the stack from the repository root:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
```

`--wait` returns when every service reports healthy. Open `http://localhost` (or your `PUBLIC_URL`), register an account, and create a workspace. The account that creates a workspace becomes its owner.

## HTTPS

Set `DOMAIN` to your host name, for example `chat.example.com`, and set `PUBLIC_URL=https://chat.example.com`. Then:

- ports 80 and 443 on this machine must be reachable from the internet, because the certificate is issued over HTTP
- the host name must already point to this machine

Caddy obtains the certificate and renews it. Certificate data is kept in the `caddydata` volume.

## Ports

Caddy publishes port 80 and port 443. To use other host ports, set them in your shell before the command, and set `PUBLIC_URL` with the same port, because file links are signed for that address:

```sh
HTTP_PORT=8080 HTTPS_PORT=8443 PUBLIC_URL=http://localhost:8080 docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
```

Only Caddy publishes ports. PostgreSQL, Redis, MinIO and the server are reachable only inside the stack's private network.

## Try it on your own computer

This runs the same stack on one computer for a demo or a trial. It is not a production setup: nothing backs it up, it has no HTTPS, and it has no mail server.

1. Follow [Install](#install) to create `.env`. Keep `PUBLIC_URL=http://localhost`, and leave `DOMAIN=` and `SMTP_HOST=` empty.
2. Start the stack from the repository root:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
   ```

3. Open `http://localhost`. Sign-up is open, so sign up, then create a workspace. You become its owner.
4. Without `SMTP_HOST`, the server writes each email to its log, so the verification link is there:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml logs server | grep verify-email
   ```

   Open the link to verify the account.
5. Stop the stack with `down`, as shown in [Operate](/operate#day-to-day-commands). The data stays, so the next start has the same accounts.

If port 80 is already in use on your machine, use the [ports](#ports) steps.

## After the first start

- Check the stack with `ps` and read the [operate](/operate) page for logs, backups and upgrades.
- Read the [configuration](/configuration) page to decide which optional modules to turn on with `MODULES_ENABLED`. The `.env.example` file turns on all four; with the variable unset, only `notify` runs.
- Known limits in v0.1.0: the MinIO bucket is `samnak` and must stay that way. Compose builds the images from your checkout; the release images on GitHub Container Registry are not used by the compose file yet.
