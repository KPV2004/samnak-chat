-- name: InsertInvite :exec
INSERT INTO workspace_invites (id, workspace_id, email, token_hash, role, created_by, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListPendingInvites :many
-- Pending means not revoked, not used and not expired.
SELECT id, workspace_id, email, role, created_by, expires_at, created_at, revoked_at, used_at
FROM workspace_invites
WHERE workspace_id = $1 AND revoked_at IS NULL AND used_at IS NULL AND expires_at > $2
ORDER BY created_at DESC, id;

-- name: RevokeInvite :execrows
UPDATE workspace_invites SET revoked_at = $3
WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL AND used_at IS NULL;

-- name: GetInviteByTokenHashForUpdate :one
-- The only query that is not scoped by a workspace ID from the Principal: the
-- caller is not a member yet, and the secret token is the key. The row is
-- locked so concurrent accepts of one link run one after the other.
SELECT id, workspace_id, email, role, created_by, expires_at, created_at, revoked_at, used_at
FROM workspace_invites
WHERE token_hash = $1
FOR UPDATE;

-- name: MarkInviteUsed :execrows
UPDATE workspace_invites SET used_at = $3, used_by = $4
WHERE workspace_id = $1 AND id = $2 AND used_at IS NULL AND revoked_at IS NULL;
