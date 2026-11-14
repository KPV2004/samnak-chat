# 0002: Clean Architecture inside each module

## Status

Accepted, 2026-11-14

## Context

Each module holds business rules, such as an approval that only the named approver can decide ([architecture.md](../architecture.md) section 12) or a ticket moving through SLA states. The rules must be correct and testable. [NFR-12](../srs.md) requires tests of the domain and usecase layers to cover at least 80%.

The SRS already fixes the internal dependency direction: adapter calls usecase, usecase calls domain, and domain knows nothing about the database or HTTP ([srs.md](../srs.md) section 3).

## Decision

Each module uses three layers, and dependencies point inward only: `adapter → usecase → domain` ([architecture.md](../architecture.md) section 5.3).

- **domain**: entities, state transitions and business errors. No SQL, HTTP, JSON tags or calls to other modules.
- **usecase**: one capability per flow, the transaction boundary, and ports (the interfaces it needs).
- **adapter**: HTTP handlers, the Postgres repository and job workers. Maps input and output, and holds no business rules.

Interfaces are created only where a boundary is crossed: the database, events and other modules. A simple module may skip layers it does not need.

## Consequences

- Domain and usecase tests run with in-memory fakes, with no database. They are fast and run in every change ([architecture.md](../architecture.md) section 14).
- Adapters are tested separately against real PostgreSQL and Redis with testcontainers.
- More folders and mapping code between layers. This is accepted for modules with real rules and skipped for simple ones.
- Developers must know which layer a rule belongs to. The table in [architecture.md](../architecture.md) section 5.3 is the reference.

## Alternatives considered

- **Layers by technical type, with handlers calling a repository directly.** This is quick to start, but business rules spread into HTTP handlers and SQL code, so they can only be tested through the database. Rejected because the approval and helpdesk state rules need isolated tests.
- **Transaction script with no domain layer.** Each use case is a procedure over the database. This suits plain CRUD and is fine for simple modules, but rules that span several entities would be repeated across use cases. Rejected as the default; a module may still use it when it has no rules.
- **Interfaces for every type in every layer.** This gives maximum mocking flexibility but adds many interfaces that test nothing real. Rejected because [architecture.md](../architecture.md) section 5.3 restricts interfaces to real boundaries.
