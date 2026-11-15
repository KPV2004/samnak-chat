-- name: InsertWorkspace :exec
INSERT INTO workspace_workspaces (id, name, slug, created_at)
VALUES ($1, $2, $3, $4);

-- name: LockWorkspace :one
-- Serialises member changes in one workspace, so the last-owner rule holds
-- under concurrent requests.
SELECT id FROM workspace_workspaces WHERE id = $1 FOR UPDATE;

-- name: ListWorkspacesForUser :many
SELECT w.id, w.name, w.slug, w.created_at, m.role
FROM workspace_members m
JOIN workspace_workspaces w ON w.id = m.workspace_id
WHERE m.user_id = $1 AND m.status = 'active'
ORDER BY w.created_at, w.id;

-- name: GetWorkspaceForUser :one
SELECT w.id, w.name, w.slug, w.created_at, m.role
FROM workspace_members m
JOIN workspace_workspaces w ON w.id = m.workspace_id
WHERE m.workspace_id = $1 AND m.user_id = $2 AND m.status = 'active';
