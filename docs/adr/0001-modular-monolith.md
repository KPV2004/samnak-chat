# 0001: Modular monolith instead of microservices

## Status

Accepted, 2026-11-14

## Context

Samnak Chat is a self-hosted chat with tasks, approvals and a helpdesk built in ([architecture.md](../architecture.md) section 1). It must install with one command using Docker Compose ([NFR-15](../srs.md)) and run on one or more server instances that share state through Redis ([NFR-04](../srs.md)). The team is small, and a deployment has to be easy to run.

Features must be switchable: `MODULES_ENABLED` turns modules on or off without code changes ([PLT-01](../srs.md)), and disabling a module keeps its data ([PLT-07](../srs.md)).

## Decision

Build the server as one Go program: a modular monolith. Each feature is a module (`auth`, `workspace`, `chat`, `notify` as core; `task`, `approval`, `helpdesk` as optional) that is enabled or disabled at startup. See [architecture.md](../architecture.md) sections 1, 3 and 7.

Modules do not call each other directly. They communicate through events, the notification registry, the card registry and the service registry ([architecture.md](../architecture.md) section 6). CI enforces the import rules with `depguard` ([architecture.md](../architecture.md) section 4, [PLT-08](../srs.md)).

## Consequences

- One binary and one deployment. Operating the system needs one server process per instance, plus PostgreSQL, Redis and MinIO.
- Module boundaries are enforced by code review and CI, not by the network. A bug in one module can still affect the whole process.
- Tables carry no foreign keys across modules, and a module refers to another's data by ID only ([architecture.md](../architecture.md) section 8.2). This keeps a module separable, so it can be split into its own service later if needed.
- Every module must handle being absent. A missing hard dependency stops the server at startup ([PLT-02](../srs.md)); a missing soft dependency falls back to a no-op ([PLT-05](../srs.md)).

## Alternatives considered

- **Microservices, one service per module.** Each module would deploy separately and talk over the network. This gives independent scaling and deploys, but a small team pays for it in service discovery, distributed tracing, and cross-service transactions. Events would need a broker from the start. Rejected for v0.1 because the team cannot run that many moving parts.
- **Single package with no module boundaries.** This is the simplest layout, but disabling one feature cleanly (PLT-01, PLT-02) and keeping modules separable would depend on discipline alone. Rejected because the requirements need enforced boundaries.
