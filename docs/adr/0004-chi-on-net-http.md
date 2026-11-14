# 0004: Chi on net/http

## Status

Accepted, 2026-11-14

## Context

The SRS fixes Chi as the HTTP router, with Huma for OpenAPI generation ([srs.md](../srs.md) section 1.4). Each module mounts its own endpoints under `/api/v1/{name}` through a `Routes(r chi.Router)` method ([architecture.md](../architecture.md) section 5.1). The `httpx` package provides the router, middleware and error-to-JSON mapping ([architecture.md](../architecture.md) section 3.2). Endpoints of a disabled module are not mounted, so they return 404 ([architecture.md](../architecture.md) section 7).

## Decision

Use Chi as the router, built on the standard `net/http` package.

## Consequences

- Handlers are plain `http.Handler` values, so standard-library middleware and any library written for `net/http` work without an adapter.
- Chi's sub-router mounting fits the per-module `Routes` method, and a disabled module simply never mounts its router.
- The `Module` interface depends on `chi.Router`, so changing the router later would touch every module's `Routes` method.
- Error mapping and request validation are our code in `httpx`; Chi does not provide them.

## Alternatives considered

- **Gin.** Fast and popular, but it uses its own context type rather than `http.Handler`, so standard middleware needs adapters. Rejected.
- **Echo.** Similar to Gin with the same issue. Rejected for the same reason.
- **Fiber.** Built on fasthttp rather than `net/http`, so it does not work with the standard-library ecosystem. Rejected.
- **`net/http` `ServeMux` alone.** Go 1.22 added method and path patterns, so this is viable. It has less support for grouping routes and composing middleware per sub-tree, which the per-module layout needs. Rejected because Chi is already agreed in [srs.md](../srs.md) section 1.4.
