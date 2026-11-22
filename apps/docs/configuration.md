# Configuration

Samnak Chat reads its settings from environment variables. The production stack reads them from the `.env` file you copy from `.env.example`. Each variable below is listed with the value in `.env.example` and the default the server uses when the variable is empty or missing.

Values in `.env` are read by the compose file and passed to the containers. Some settings are used by the server and some only by the stack around it, as the **Used by** column shows.

## Required

| Variable | Used by | Example in `.env.example` | Meaning |
| --- | --- | --- | --- |
| `JWT_SECRET` | server | `change-me` | Signs the access tokens. The server stops at startup if it is empty. Use a long random string. |
| `DATABASE_URL` | server | `postgres://samnak:samnak@localhost:5432/samnak?sslmode=disable` | PostgreSQL connection. The compose file builds its own URL from `POSTGRES_PASSWORD`, so you do not set it there. |
| `REDIS_URL` | server | `redis://localhost:6379/0` | Redis connection. The server connects at startup and stops if Redis does not answer. |

## Storage

| Variable | Used by | Example | Default | Meaning |
| --- | --- | --- | --- | --- |
| `S3_ENDPOINT` | server | `http://localhost:9000` | none | Address of MinIO or another S3 service, as the server reaches it. The compose file sets this to the `minio` service. |
| `S3_PUBLIC_ENDPOINT` | server | not set | the value of `S3_ENDPOINT` | The address browsers use to upload and download files. The compose file sets it to `PUBLIC_URL`. |
| `S3_BUCKET` | server, stack | `samnak` | none | The bucket for attachments. Keep `samnak`: the stack's Caddy route and the MinIO setup both use that name. |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | server | `minioadmin` | none | Keys for the bucket. Use random values in production. |
| `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` | stack | `minioadmin` | none | The MinIO login. They must equal `S3_ACCESS_KEY` and `S3_SECRET_KEY`. |

Presigned upload and download links are valid for 15 minutes.

## Addresses

| Variable | Used by | Example | Default | Meaning |
| --- | --- | --- | --- | --- |
| `PUBLIC_URL` | stack | `http://localhost` | `http://localhost` | The address people open, with the port if it is not 80 or 443. Email links and file links use it. |
| `WEB_URL` | server | `http://localhost:5173` | `http://localhost:5173` | The base of the links in emails. The compose file sets it to `PUBLIC_URL`. |
| `DOMAIN` | stack | empty | empty (plain HTTP on port 80) | The host name for automatic HTTPS, for example `chat.example.com`. Set it only when ports 80 and 443 reach this machine. |
| `HTTP_ADDR` | server | `:8080` | `:8080` | The address the API listens on, inside the container. |
| `HTTP_PORT`, `HTTPS_PORT` | stack | not in `.env.example` | `80`, `443` | Host ports Caddy publishes. Set them in your shell, not in `.env`, and set `PUBLIC_URL` to match. |
| `STACK_SUBNET` | stack | `172.28.0.0/24` | `172.28.0.0/24` | The address range of the stack's private network. Change it only if it clashes with another Docker network. |
| `TRUSTED_PROXIES` | server | empty | empty (no proxy trusted) | Addresses or CIDR ranges of reverse proxies, separated by commas. The server believes their `X-Forwarded-For` header when it counts requests by client IP. The compose file sets it to `STACK_SUBNET`, where only Caddy reaches the server. |

## Email

With `SMTP_HOST` empty, the server writes each email to its log and sends nothing. That is useful for a trial. Set a real mail server for production.

| Variable | Example | Default | Meaning |
| --- | --- | --- | --- |
| `SMTP_HOST` | empty | empty | The mail server. Empty means: write to the log. |
| `SMTP_PORT` | `1025` | `587` | The port. Port 465 uses implicit TLS. Other ports use STARTTLS when the server offers it. |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | empty | empty | Login for the mail server, if it needs one. |
| `SMTP_FROM` | `Samnak <no-reply@localhost>` | `Samnak <no-reply@localhost>` | The sender address. |

The mail is for sign-up verification, resending the verification email, and password reset. Email from the notification module is planned.

## Modules

| Variable | Example | Default | Meaning |
| --- | --- | --- | --- |
| `MODULES_ENABLED` | `notify,task,approval,helpdesk` | not set: `notify` only | The optional modules to turn on. The core modules `auth`, `workspace` and `chat` are always on. |

The three settings behave differently:

| Setting | Modules enabled |
| --- | --- |
| Not set | `auth`, `workspace`, `chat`, `notify` |
| Empty (`MODULES_ENABLED=`) | `auth`, `workspace`, `chat` only |
| `task,helpdesk` | the core modules, `task` and `helpdesk` |

Names are case-insensitive, and unknown names stop the server at startup with a message. A disabled module returns 404 for its routes and its menu items are hidden. Its data stays in the database. Restart the server after you change the value.

`GET /api/v1/meta` returns the modules that are on.

## Rate limits

Every API call is limited per minute. Sign-in has its own, stricter limit.

| Variable | Example | Default | Meaning |
| --- | --- | --- | --- |
| `RATE_LIMIT_ENABLED` | `true` | `true` | Turns the API-wide limit on or off. |
| `RATE_LIMIT_PER_MINUTE` | `600` | `600` | Calls a signed-in user may make in a minute. |
| `RATE_LIMIT_IP_PER_MINUTE` | `120` | `120` | Calls a signed-out client may make in a minute, counted by IP address. |

Over the limit, the API answers 429 with a `Retry-After` header. Health checks, `/metrics` and the WebSocket connection are not counted. Sign-in allows 5 failed attempts a minute for each email address and for each IP address.

## Passwords for the compose stack

| Variable | Example | Default | Meaning |
| --- | --- | --- | --- |
| `POSTGRES_PASSWORD` | `samnak` | `samnak` | The database password. Set it before the first start: the database keeps the password it was created with. |

Change every example value (`change-me`, `minioadmin`, `samnak`) before you use the stack for real work.
