-- name: InsertQueue :exec
INSERT INTO helpdesk_queues (id, workspace_id, name, prefix, channel_id, created_at, updated_at)
VALUES (@id, @workspace_id, @name, @prefix, sqlc.narg('channel_id')::uuid, @created_at, @updated_at);

-- name: UpdateQueue :execrows
UPDATE helpdesk_queues
SET name = @name, channel_id = sqlc.narg('channel_id')::uuid, archived_at = sqlc.narg('archived_at')::timestamptz,
    updated_at = @updated_at
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetQueue :one
SELECT * FROM helpdesk_queues WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListQueues :many
SELECT * FROM helpdesk_queues
WHERE workspace_id = @workspace_id AND (@include_archived::boolean OR archived_at IS NULL)
ORDER BY name, id;

-- name: ListQueueAgentsOfWorkspace :many
SELECT a.queue_id, a.user_id FROM helpdesk_queue_agents a
JOIN helpdesk_queues q ON q.id = a.queue_id
WHERE q.workspace_id = @workspace_id
ORDER BY a.added_at, a.user_id;

-- name: ListQueueAgents :many
SELECT user_id FROM helpdesk_queue_agents WHERE queue_id = @queue_id ORDER BY added_at, user_id;

-- name: DeleteQueueAgents :exec
DELETE FROM helpdesk_queue_agents WHERE queue_id = @queue_id;

-- name: InsertQueueAgent :exec
INSERT INTO helpdesk_queue_agents (queue_id, user_id, added_at) VALUES (@queue_id, @user_id, @added_at);

-- name: IsQueueAgent :one
SELECT EXISTS (
    SELECT 1 FROM helpdesk_queue_agents a
    JOIN helpdesk_queues q ON q.id = a.queue_id
    WHERE a.queue_id = @queue_id AND a.user_id = @user_id AND q.workspace_id = @workspace_id);
