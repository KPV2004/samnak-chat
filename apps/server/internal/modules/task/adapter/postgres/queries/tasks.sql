-- name: InsertTask :one
INSERT INTO task_items (id, workspace_id, channel_id, source_message_id, title, description, creator_id, assignee_id, due_at, priority, status, created_at, updated_at)
VALUES (@id, @workspace_id, @channel_id, @source_message_id, @title, @description, @creator_id, @assignee_id, @due_at, @priority, @status, @created_at, @updated_at)
RETURNING *;

-- name: GetTask :one
SELECT * FROM task_items WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetTaskForUpdate :one
SELECT * FROM task_items WHERE id = @id AND workspace_id = @workspace_id FOR UPDATE;

-- name: UpdateTask :one
UPDATE task_items
SET title = @title, description = @description, assignee_id = @assignee_id, due_at = @due_at,
    priority = @priority, status = @status, due_soon_sent_at = @due_soon_sent_at,
    overdue_sent_at = @overdue_sent_at, updated_at = @updated_at
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: SetTaskMessage :execrows
UPDATE task_items SET message_id = @message_id
WHERE id = @id AND workspace_id = @workspace_id AND message_id IS NULL;

-- name: ListTasks :many
SELECT * FROM task_items
WHERE workspace_id = @workspace_id
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR assignee_id = sqlc.narg('assignee_id')::uuid)
  AND (sqlc.narg('creator_id')::uuid IS NULL OR creator_id = sqlc.narg('creator_id')::uuid)
  AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id')::uuid)
  AND (cardinality(@statuses::text[]) = 0 OR status = ANY(@statuses::text[]))
  AND (sqlc.narg('due_from')::timestamptz IS NULL OR due_at >= sqlc.narg('due_from')::timestamptz)
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR due_at < sqlc.narg('due_before')::timestamptz)
  AND (@open_only::boolean = false OR status <> 'done')
  AND (sqlc.narg('after_id')::uuid IS NULL OR id < sqlc.narg('after_id')::uuid)
ORDER BY id DESC
LIMIT @page_size;

-- name: ClaimDueSoon :many
UPDATE task_items SET due_soon_sent_at = @now
WHERE id IN (
    SELECT t.id FROM task_items t
    WHERE t.status <> 'done' AND t.assignee_id IS NOT NULL AND t.due_at IS NOT NULL
      AND t.due_soon_sent_at IS NULL AND t.due_at > @now AND t.due_at <= @horizon
    ORDER BY t.due_at, t.id
    LIMIT @batch FOR UPDATE SKIP LOCKED
)
RETURNING id, workspace_id, assignee_id, due_at;

-- name: ClaimOverdue :many
UPDATE task_items SET overdue_sent_at = @now
WHERE id IN (
    SELECT t.id FROM task_items t
    WHERE t.status <> 'done' AND t.assignee_id IS NOT NULL AND t.due_at IS NOT NULL
      AND t.overdue_sent_at IS NULL AND t.due_at <= @now
    ORDER BY t.due_at, t.id
    LIMIT @batch FOR UPDATE SKIP LOCKED
)
RETURNING id, workspace_id, assignee_id, due_at;
