# 0003: Event bus with an outbox

## Status

Accepted, 2026-11-14

## Context

Modules must not call each other directly, so that any module can be disabled without breaking the rest ([architecture.md](../architecture.md) section 2, principle 1). A publisher should not know who listens ([srs.md](../srs.md) section 3).

Three requirements shape the bus:

- An event with no listener is not an error, and a handler that fails or panics must not affect the publisher ([PLT-04](../srs.md), Must, M0).
- Events are sent only after the transaction commits, and they survive a server crash through an `platform_outbox_events` table ([PLT-10](../srs.md), Should, M4; [architecture.md](../architecture.md) section 6.1).
- Delivery is at least once, so handlers must give the same result when they receive the same event twice ([NFR-10](../srs.md)).

## Decision

Modules publish events through a `Publisher` interface, declared in each module's `events.go`, and other modules subscribe to them. An event is written to `platform_outbox_events` in the same transaction as the data it describes, and a relay publishes it after commit ([architecture.md](../architecture.md) sections 6.1 and 8.3). The `TxManager` carries the transaction in the context, so a use case can publish without knowing about pgx.

In M0 the bus is in-memory, and the outbox implementation arrives in M4 ([architecture.md](../architecture.md) section 3.2 lists both `eventbus` implementations).

## Consequences

- Until M4, events on the in-memory bus are lost if the server stops. This is accepted because M0 is only the wiring; it is not durable.
- Handlers must be idempotent, and new fields may be added to an event, but existing fields must not be removed or change meaning ([architecture.md](../architecture.md) section 6.1, [srs.md](../srs.md) section 14.4).
- The relay adds delay between commit and delivery. Latency targets for chat are carried by the WebSocket path, not the outbox ([NFR-02](../srs.md)).
- The outbox table is owned by `platform`, not by a feature module ([srs.md](../srs.md) section 13).

## Alternatives considered

- **Publish inside the transaction.** A listener could act on data that is later rolled back. Rejected.
- **Publish after commit with no outbox.** This is simple, but a crash between commit and publish loses the event. This conflicts with [PLT-10](../srs.md). Rejected.
- **Direct synchronous calls between modules.** This ties modules together and conflicts with principle 1 in [architecture.md](../architecture.md). Rejected; it stays available only through the service registry when a result is needed right away.
- **External broker such as Kafka or NATS.** This gives durable, scalable delivery, but adds a service to run in Docker Compose, which [NFR-15](../srs.md) makes a single-command install. Rejected for v0.1.
