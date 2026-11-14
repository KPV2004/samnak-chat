# 0008: Web separate from mobile, sharing `packages/core`

## Status

Accepted, 2026-11-14

## Context

The SRS fixes the client technologies: React, TypeScript, Vite and TanStack Query for the web, Tauri for desktop and Expo (React Native) for mobile ([srs.md](../srs.md) section 1.4). The architecture requires the API to be independent of any client, because the web app, desktop and mobile all use the same public API ([architecture.md](../architecture.md) section 2, principle 5).

The layout separates the parts of each client ([architecture.md](../architecture.md) section 13):

- `packages/core` holds the API client, OpenAPI types, the WebSocket connection and chat logic.
- `packages/ui` holds components shared by web and desktop.
- `apps/web`, `apps/desktop` (Tauri, wrapping the web build) and `apps/mobile` (Expo) are the applications.

## Decision

Build the web application and the mobile application as separate apps, each with its own UI. Put logic that does not depend on the platform in `packages/core`, and keep it free of DOM and browser APIs, so mobile can use it too. Share web components with desktop through `packages/ui`.

## Consequences

- `packages/core` must not use DOM or browser-only APIs. Code that needs them lives in the app.
- Each platform gets UI that fits it, and the mobile UI is written separately from the web UI.
- API types are generated from `openapi.yaml` ([architecture.md](../architecture.md) section 13, [NFR-13](../srs.md)), so both clients compile against the same contract.
- Clients render cards by `kind` and show `fallback_text` for unknown kinds, so a new card type does not break an older client ([architecture.md](../architecture.md) section 6.3).

## Alternatives considered

- **One React Native codebase for web and mobile, using react-native-web.** This maximizes shared UI, but the web experience is shaped by the mobile component model, and it adds a compatibility layer. Rejected because the SRS names React and Vite for the web.
- **Native apps per platform with no shared code.** Each platform gets the best fit, but chat logic and API handling are written and tested several times. Rejected because it duplicates the logic that `packages/core` exists to share.
- **Web app only, with a responsive or installable web version as the mobile client.** This needs one codebase, but it does not cover the desktop and mobile apps the SRS lists. Rejected as the plan for this project.
