# Operate

This page covers the tasks you do after the first start: looking at the stack, upgrading, backing up and restoring, reading logs and metrics, and changing modules. The commands assume you run them from the repository root, with the production compose file and the `.env` file you created in [Install](/install).

## Day-to-day commands

```sh
docker compose --env-file .env -f deploy/compose.prod.yml ps                 # status of each service
docker compose --env-file .env -f deploy/compose.prod.yml logs -f server     # server log, one JSON object per line
docker compose --env-file .env -f deploy/compose.prod.yml down               # stop; data is kept
```

`down -v` also deletes the volumes, and the volumes hold all the data: the database, the files and the certificates. Do not run it unless you mean to delete everything.

## Upgrade

Pull the new code, then rebuild:

```sh
git pull
docker compose --env-file .env -f deploy/compose.prod.yml up -d --build --wait
```

The `migrate` service runs the migrations of the enabled modules before the new server starts. If the migrations fail, the server does not start, so the old data stays as it was. Read the `migrate` output with `logs migrate` if `up` reports it as failed.

Check the version after the upgrade. The server reports its version at `GET /api/v1/meta`, and the [changelog](/changelog) lists what changed.

## Back up

Back up two things: the database and the attachments.

**The database** is a PostgreSQL dump. The `--clean --if-exists` flags make the dump drop and recreate its tables, so it can be loaded over the empty tables that migrations create:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml exec -T postgres pg_dump -U samnak --clean --if-exists samnak > samnak-backup.sql
```

**The attachments** are in the `miniodata` volume. Include that volume in your usual backup of Docker volumes.

Store both copies somewhere other than this machine. The `.env` file holds the secrets too, so back it up with them.

## Restore

A restore replaces the data in the database with the dump. Restore into a new stack, on a machine that has no data you want to keep.

1. Start the stack, so that the migrations create the tables:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
   ```

2. Stop the server, so that nothing writes while the dump loads:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml stop server
   ```

3. Load the dump. It runs without errors when it is loaded into a stack that the migrations have just set up:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml exec -T postgres psql -U samnak samnak < samnak-backup.sql
   ```

4. Start the server again:

   ```sh
   docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
   ```

The attachments are a separate copy. Put them back into the `miniodata` volume with the tool you used to copy that volume. The project has not tested this step.

## Health and status

The server has two health endpoints:

| Endpoint | Answers |
| --- | --- |
| `GET /healthz` | 200 while the process runs |
| `GET /readyz` | 200 when the database and Redis answer; otherwise 503 with the names of the checks that failed |

Both are on the server, not under `/api`, and Caddy does not forward them. From outside the stack, both paths return the web app's page instead of a health answer. Inside the stack, the health check of the `server` service runs `/healthcheck`, a small program in the image that exits with 0 when `/readyz` answers 200. You can run it yourself:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml exec server /healthcheck; echo $?
```

`ps` shows the health of each service. For a service that stays unhealthy, read its `logs`.

## Metrics

The server publishes Prometheus metrics at `/metrics`. The metrics are on the internal network only: Caddy does not forward `/metrics`, and no port is published for it. To read them on the host, ask the `web` container, which is on the same network:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml exec web wget -qO- http://server:8080/metrics
```

The metrics include:

- `http_request_duration_seconds`, by route pattern, method and status. WebSocket handshakes are not timed.
- `ws_connections`, a gauge of open WebSocket connections
- `ws_messages_sent_total`, a counter of messages sent over WebSocket
- the Go runtime and process metrics

`/metrics` has no authentication. That is why the proxy does not forward it.

## Logs

The server writes one JSON object per line to stdout. Use `logs -f server` to follow it, or send the output to your log system. Email is written to this log when `SMTP_HOST` is empty, so the verification and reset links are there too. To find a verification link:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml logs server | grep verify-email
```

Treat these links as secrets. Anyone with one can verify or reset that account.

## Modules

Change `MODULES_ENABLED` in `.env`, then apply the change by running `up` again:

```sh
docker compose --env-file .env -f deploy/compose.prod.yml up -d --wait
```

The `migrate` service then creates the tables of any newly enabled module. A module you turn off keeps its data, and turning it on again shows the same data. See [Configuration](/configuration#modules) for the values.

## Background jobs

The server runs these jobs itself. They need no setup:

| Job | When | What it does |
| --- | --- | --- |
| Send email, with retries | when mail is queued | Sends sign-up, verification and reset email |
| Due-date reminders for tasks | every 5 minutes | Notifies the assignee of tasks due within 24 hours and of overdue tasks |
| Notification cleanup | every 24 hours | Deletes read notifications older than about 90 days |
| Event outbox cleanup | daily | Deletes delivered events older than 7 days |

Jobs run in the server's PostgreSQL database, so a backup of that database includes them.
