# 0005: sqlc instead of an ORM

## Status

Accepted, 2026-11-14

## Context

The SRS fixes PostgreSQL with pgx, sqlc and goose for migrations ([srs.md](../srs.md) section 1.4). Each module's `adapter/postgres` implements its repository with sqlc ([architecture.md](../architecture.md) section 5.3).

Every query on organization data must filter on `workspace_id`, because tenant isolation depends on it ([architecture.md](../architecture.md) section 8.1, [NFR-06](../srs.md)). Table names carry the module prefix, and each module keeps its own migrations ([PLT-07](../srs.md)).

## Decision

Write the SQL by hand. sqlc generates typed Go functions from those queries, and goose runs the migrations per module. Repositories sit in the adapter layer, so domain code never sees SQL ([architecture.md](../architecture.md) section 5.3).

## Consequences

- Every query is visible in review, so a missing `workspace_id` filter can be spotted directly in the SQL ([architecture.md](../architecture.md) section 8.1).
- Type mismatches between SQL and Go are caught at generation time.
- Schema changes require editing the SQL queries by hand. There is no automatic mapping from model structs to tables.
- No lazy loading or automatic relation handling; joins are written explicitly.

## Alternatives considered

- **GORM.** Model structs map to tables and relations are managed for you. This is quick for CRUD, but the SQL is hidden, and a query that forgets the tenant filter is easy to write without noticing. Rejected.
- **ent.** Code generation from a schema graph with typed queries. It is a capable ORM, but it places a second model of the database in front of the SQL, which the project does not need. Rejected.
- **Hand-written queries with `database/sql` or pgx directly.** This gives the same SQL with no generated types, so mismatches appear at runtime. Rejected because sqlc gives the same SQL with compile-time checks.
