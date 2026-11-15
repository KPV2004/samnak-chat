-- name: InsertMember :exec
INSERT INTO workspace_members (workspace_id, user_id, role, status, joined_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetMember :one
SELECT workspace_id, user_id, role, status, joined_at
FROM workspace_members
WHERE workspace_id = $1 AND user_id = $2;

-- name: GetActiveMemberRole :one
SELECT role FROM workspace_members
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active';

-- name: ListMembers :many
SELECT workspace_id, user_id, role, status, joined_at
FROM workspace_members
WHERE workspace_id = @workspace_id
  AND (@use_cursor::boolean = false OR (joined_at, user_id) > (@after_joined_at::timestamptz, @after_user_id::uuid))
ORDER BY joined_at, user_id
LIMIT @page_size;

-- name: CountActiveOwners :one
SELECT count(*) FROM workspace_members
WHERE workspace_id = $1 AND role = 'owner' AND status = 'active';

-- name: UpdateMember :execrows
UPDATE workspace_members SET role = $3, status = $4
WHERE workspace_id = $1 AND user_id = $2;

-- name: DeleteMember :execrows
DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2;
