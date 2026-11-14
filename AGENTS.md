# Agent rules for Samnak Chat

Read this before any change. Specs: `docs/srs.md` (Thai, requirement IDs like `PLT-01`) and `docs/architecture.md`.

## Roles

| Role | Model | Does | Does not |
| --- | --- | --- | --- |
| **Planner** (high tier) | Opus | Writes `tasks/plan.md` and `tasks/todo.md`, owns `apps/server/internal/app` contracts, reviews and merges worker branches, runs checkpoints | Write feature code a task already assigns to a worker |
| **Worker** (low tier) | Sonnet / Haiku | Implements exactly one task from `tasks/todo.md` in its own worktree and branch | Change files outside the task's "Files" list, edit `internal/app` contracts, edit the plan |

If a worker finds that a contract in `internal/app` is wrong or missing something, it stops and writes the problem in its final report. The planner changes the contract.

## Worker loop

1. Read the task in `tasks/todo.md`, the SRS IDs it cites, and the architecture sections it cites.
2. Work on branch `task/<id>-<slug>` in your own worktree.
3. Test first: write a failing test, then make it pass (skill: `test-driven-development`).
4. Touch only the files listed in the task. Create new files only inside the listed folders.
5. Verify: `cd apps/server && go build ./... && go vet ./... && go test ./...` (and `npm run build` and `npm test` for web tasks).
6. Commit with a Conventional Commit message that cites the task: `feat(platform): in-memory event bus (M0-02)`.
7. Push the branch. Do not merge to `main` and do not tick the box in `tasks/todo.md`; the planner does both.
8. Final report: what was done, verification output, deviations, contract problems.

## Hard rules (from architecture section 4)

- `internal/app` imports nothing from this project.
- `internal/platform` knows no feature.
- A module imports only `api.go` and `events.go` from another module. Never another module's `domain`, `usecase` or `adapter`.
- No module reads or writes another module's tables. Table names are prefixed with the module name.
- `workspace_id` always comes from `app.Principal`, never from the request body.
- IDs are UUIDv7 and timestamps are `timestamptz` in UTC.
- Do not silence checks: no `//nolint`, skipped tests or weakened assertions without the planner's approval.

## Commands

```sh
make up                                   # postgres, redis, minio
cd apps/server && go test ./...           # server tests
npm install && npm run build              # web and packages
```
