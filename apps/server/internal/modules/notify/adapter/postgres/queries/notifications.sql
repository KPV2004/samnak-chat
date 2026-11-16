-- name: InsertNotification :one
INSERT INTO notify_notifications (id, workspace_id, user_id, kind, title, body, link, data, key, created_at)
VALUES (@id, @workspace_id, @user_id, @kind, @title, @body, @link, @data, @key, @created_at)
ON CONFLICT (user_id, key) WHERE key <> '' DO NOTHING
RETURNING id, workspace_id, user_id, kind, title, body, link, data, key, read_at, created_at;

-- name: GetNotification :one
SELECT id, workspace_id, user_id, kind, title, body, link, data, key, read_at, created_at
FROM notify_notifications
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: ListNotifications :many
SELECT id, workspace_id, user_id, kind, title, body, link, data, key, read_at, created_at
FROM notify_notifications
WHERE user_id = @user_id
  AND workspace_id = @workspace_id
  AND (@unread_only::boolean = false OR read_at IS NULL)
  AND (
    sqlc.narg('before_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('before_created_at')::timestamptz, sqlc.narg('before_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT @page_size;

-- name: UnreadCount :one
SELECT count(*) FROM notify_notifications
WHERE user_id = @user_id AND workspace_id = @workspace_id AND read_at IS NULL;

-- name: MarkRead :one
UPDATE notify_notifications SET read_at = @read_at
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id AND read_at IS NULL
RETURNING id;

-- name: MarkAllRead :many
UPDATE notify_notifications SET read_at = @read_at
WHERE workspace_id = @workspace_id AND user_id = @user_id AND read_at IS NULL
RETURNING id;

-- name: PruneNotifications :execrows
DELETE FROM notify_notifications WHERE read_at IS NOT NULL AND created_at < @before;
