# 0006: River instead of a separate queue

## Status

Accepted, 2026-11-14

## Context

Some work runs in the background: sending push and email with retries ([architecture.md](../architecture.md) section 10, notify), reminders for tasks that are due or overdue (task), and SLA checks for helpdesk tickets (helpdesk). The SRS fixes River, which stores jobs in PostgreSQL ([srs.md](../srs.md) section 1.4).

Three requirements apply:

- A job can be enqueued in the same transaction as the data it works on ([PLT-09](../srs.md), Must, M3; [architecture.md](../architecture.md) section 8.3).
- A job with no registered worker is cancelled, so it does not sit in the queue ([PLT-09](../srs.md); [architecture.md](../architecture.md) section 7).
- Shutdown waits for in-flight requests and jobs to finish ([NFR-11](../srs.md)).

## Decision

Use River as the job queue. It stores jobs in PostgreSQL, so no extra service is needed ([architecture.md](../architecture.md) section 10).

## Consequences

- No extra service in Docker Compose, which keeps [NFR-15](../srs.md) simple.
- A job and the data it depends on commit or roll back together, so a job never refers to data that was not saved.
- Job load shares PostgreSQL with request traffic. This is acceptable at the scale in the SRS and must be watched as load grows.
- Workers must be registered for each job kind; the cancel-without-worker rule in PLT-09 has to be implemented, not assumed.

## Alternatives considered

- **Redis-based queue such as Asynq.** Fast and simple, but Redis is a separate store, so a job cannot be enqueued in the same PostgreSQL transaction as the data. It would need the outbox pattern from [0003](./0003-event-bus-with-outbox.md) to get that guarantee. Rejected.
- **RabbitMQ or NATS JetStream.** Mature brokers with rich routing, but each is an extra service to run and does not join the data transaction. Rejected for v0.1.
- **Polling a jobs table written by hand.** This avoids a dependency, but retries, scheduling and worker coordination would have to be written and tested by the team. Rejected because River already provides them.
