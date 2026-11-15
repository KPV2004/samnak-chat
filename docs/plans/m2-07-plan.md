# Plan: M2-07 AUTH-08 and M2 acceptance test

## Objective
Make a revoked device's WebSocket close immediately, and prove the SRS 16.3 M2 acceptance scenarios with an automated end-to-end test, so Checkpoint M2 has evidence.

## Context already in place
- `app.Realtime` has `DisconnectDevice(ctx, deviceID)`; the hub closes that device's sockets on every instance through Redis (`internal/platform/realtime/fanout.go`), tested in `hub_test.go` and `redis_test.go`.
- The hub authenticates the in-band `auth` frame with `app.Authenticator`; a revoked token is already rejected at the frame.
- auth revokes in `usecase/sessions.go`: `Logout` (this device or all devices) and `RevokeDevice`. Nothing publishes an event yet and `Module.Subscribe` is empty.
- `app.Deps` already has `Bus`; `cmd/server` builds it in `newDeps` and passes `Realtime` as the hub.

## Steps
1. **auth event** — add `internal/modules/auth/events.go` with `DeviceRevoked{UserID, DeviceID}` (`EventName() "auth.device_revoked"`; `DeviceID` empty means every device of the user was revoked).
2. **publish on revoke** — give `usecase.Deps` a `Bus app.Publisher` (nil allowed) and publish after the repository write succeeds in `Logout` (all devices → empty device ID; one device → that device) and in `RevokeDevice`. Do not publish on a failed or no-op revoke. Idempotent revokes must not publish twice.
3. **disconnect in cmd/server** — after modules start in `newHandler`, subscribe on `deps.Bus` to `auth.device_revoked` and call `deps.Realtime.DisconnectDevice(ctx, evt.DeviceID)`, or for an empty device ID disconnect every device of the user (list the user's devices through the auth api, or add a small helper on the hub only if the api cannot do it). Log failures; never fail startup.
4. **unit tests** — auth usecase: revoke publishes one event with the right IDs; repeated/no-op revoke publishes nothing; failed revoke publishes nothing. `cmd/server`: the handler calls a fake hub's `DisconnectDevice` for a user and for all devices. Hub behaviour per device is already covered; keep those tests.
5. **acceptance package** — new `internal/acceptance` with tests skipped clearly when Docker is unavailable (same pattern as `internal/platform/database` tests). Start postgres and redis containers, run the per-module goose migrations, build the real registry (auth, workspace, chat), the real hub and the router the way `cmd/server` does, then over one `httptest` server:
   - two users in one workspace see a message over WebSocket in under 1 second (measure from POST response to the recipient frame);
   - close the recipient's socket, send two more messages, reconnect with `after_seq` and receive the missing messages in seq order, once each;
   - POST the same `client_msg_id` twice → same message, one `message.created` for the recipient;
   - revoke a device (AUTH-08) → its socket closes with 4401 and the next `auth` frame with the old token is rejected.
6. **documentation** — add the manual two-browser pass to `docs/` only if the acceptance test cannot cover a UI behaviour; otherwise state in the final report what the test does and does not prove.

## Files to change
- `apps/server/internal/modules/auth/events.go` (new), `usecase/ports.go`, `usecase/sessions.go`, `usecase/service.go`, `module.go`, tests
- `apps/server/cmd/server/main.go`, `main_test.go` (or a new test file in that package)
- `apps/server/internal/acceptance/` (new)
- `apps/server/go.mod`, `go.sum` (testcontainers modules if not already required)

Do **not** change `apps/server/internal/app`: the contracts already cover this task. Do not touch `tasks/`.

## Acceptance criteria
- [ ] auth publishes `auth.device_revoked` after a committed revoke (one device or all devices); no event for failed or repeated revokes
- [ ] `cmd/server` subscribes and disconnects the device on the hub, including through Redis on another instance
- [ ] A revoked device's WebSocket closes with 4401 and the token is rejected at the frame
- [ ] `internal/acceptance` proves the three SRS 16.3 M2 outcomes and the revocation case, with a clear skip when Docker is unavailable
- [ ] `go build ./... && go vet ./... && go test ./...` and `go test -race ./internal/acceptance/ ./internal/modules/auth/... ./cmd/server/` pass

## Delegation
- Planner/Reviewer: DeepSeek (this session)
- Implementer: Qwen in worktree `samnak-chat-worktrees/task-m2-07-auth08-acceptance`, branch `task/m2-07-auth08-acceptance`
