-- name: InsertCategory :exec
INSERT INTO helpdesk_categories (id, workspace_id, queue_id, name, is_default, archived_at, created_at)
VALUES (@id, @workspace_id, @queue_id, @name, @is_default, sqlc.narg('archived_at')::timestamptz, @created_at);

-- name: UpdateCategory :execrows
UPDATE helpdesk_categories
SET queue_id = @queue_id, name = @name, is_default = @is_default, archived_at = sqlc.narg('archived_at')::timestamptz
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetCategory :one
SELECT * FROM helpdesk_categories WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListCategories :many
SELECT * FROM helpdesk_categories
WHERE workspace_id = @workspace_id AND (@include_archived::boolean OR archived_at IS NULL)
ORDER BY name, id;

-- name: ClearDefaultCategory :exec
UPDATE helpdesk_categories SET is_default = false
WHERE workspace_id = @workspace_id AND is_default
  AND (sqlc.narg('except_id')::uuid IS NULL OR id <> sqlc.narg('except_id')::uuid);
