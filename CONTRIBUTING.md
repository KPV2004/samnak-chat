# Contributing

1. Read `docs/architecture.md`, especially sections 4 (dependency rules) and 5.3 (module layout).
2. Pick an open task from `tasks/todo.md`. One task per branch: `task/<id>-<slug>`.
3. Write tests first for domain and use-case logic.
4. Before opening a PR: `make lint test` passes, and OpenAPI and web types are regenerated if the API changed (`make openapi`).
5. Use [Conventional Commits](https://www.conventionalcommits.org/): `feat(chat): ...`, `fix(auth): ...`.

## Definition of done (SRS 16.2)

- Tests cover success, forbidden and invalid-input cases
- CI passes: lint, tests, module-boundary check
- OpenAPI and web types regenerated
- Migrations run on an empty database
- Docs updated if user-visible behaviour changed
