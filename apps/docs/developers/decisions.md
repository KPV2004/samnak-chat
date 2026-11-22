# Decision records

An architecture decision record (ADR) says what was decided, why, and what was rejected. Each decision has its own file in the repository, in [`docs/adr/`](https://github.com/KPV2004/samnak-chat/tree/main/docs/adr). The files list the status, the context, the decision, the consequences and the alternatives.

| No. | Decision | Reason in brief | Record |
| --- | --- | --- | --- |
| 0001 | Modular monolith instead of microservices | Easy for a small team to run, and modules can be split out later | [0001](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0001-modular-monolith.md) |
| 0002 | Clean Architecture inside each module | Business rules can be tested without a database | [0002](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0002-clean-architecture-per-module.md) |
| 0003 | Event bus with an outbox | Modules stay decoupled, and events survive a crash | [0003](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0003-event-bus-with-outbox.md) |
| 0004 | Chi on `net/http` | Works with every standard Go library | [0004](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0004-chi-on-net-http.md) |
| 0005 | sqlc instead of an ORM | Explicit SQL, with compile-time type checks | [0005](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0005-sqlc-instead-of-orm.md) |
| 0006 | River instead of a separate queue | Reuses PostgreSQL, and jobs can be queued inside transactions | [0006](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0006-river-job-queue.md) |
| 0007 | Per-channel `seq` and `client_msg_id` | No duplicate messages, and the same order on every device | [0007](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0007-per-channel-seq-and-client-msg-id.md) |
| 0008 | Web separate from mobile, sharing `packages/core` | Each platform gets a fitting UI without duplicating the logic | [0008](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0008-web-and-mobile-separate-shared-core.md) |

## Writing a new record

Name the file `NNNN-title.md`, with the next free number. Give it the sections the existing records use: Status, Context, Decision, Consequences and Alternatives considered. Link the new record from the table in [`docs/architecture.md`](https://github.com/KPV2004/samnak-chat/blob/main/docs/architecture.md), section 16.
