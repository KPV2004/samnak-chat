-- name: InsertMessage :exec
INSERT INTO chat_messages (id, workspace_id, channel_id, seq, sender_id, client_msg_id, kind, body, payload, fallback_text,
                           thread_root_id, mentions, mentions_channel, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: GetMessageByClientID :one
SELECT m.id, m.workspace_id, m.channel_id, m.seq, m.sender_id, m.client_msg_id, m.kind, m.body, m.payload, m.fallback_text,
       m.thread_root_id, m.mentions, m.mentions_channel, m.edited_at, m.deleted_at, m.created_at,
       (SELECT count(*) FROM chat_messages r WHERE r.thread_root_id = m.id AND r.deleted_at IS NULL)::bigint AS reply_count
FROM chat_messages m
WHERE m.workspace_id = $1 AND m.sender_id = $2 AND m.client_msg_id = $3;

-- name: GetMessage :one
SELECT m.id, m.workspace_id, m.channel_id, m.seq, m.sender_id, m.client_msg_id, m.kind, m.body, m.payload, m.fallback_text,
       m.thread_root_id, m.mentions, m.mentions_channel, m.edited_at, m.deleted_at, m.created_at,
       (SELECT count(*) FROM chat_messages r WHERE r.thread_root_id = m.id AND r.deleted_at IS NULL)::bigint AS reply_count
FROM chat_messages m
WHERE m.workspace_id = $1 AND m.id = $2;

-- name: ListMessagesAfter :many
SELECT m.id, m.workspace_id, m.channel_id, m.seq, m.sender_id, m.client_msg_id, m.kind, m.body, m.payload, m.fallback_text,
       m.thread_root_id, m.mentions, m.mentions_channel, m.edited_at, m.deleted_at, m.created_at,
       (SELECT count(*) FROM chat_messages r WHERE r.thread_root_id = m.id AND r.deleted_at IS NULL)::bigint AS reply_count
FROM chat_messages m
WHERE m.workspace_id = @workspace_id AND m.channel_id = @channel_id AND m.seq > @after_seq::bigint
  AND (sqlc.narg('thread_root_id')::uuid IS NULL OR m.thread_root_id = sqlc.narg('thread_root_id')::uuid)
ORDER BY m.seq ASC
LIMIT @page_size;

-- name: ListMessagesBefore :many
-- before_seq 0 means the newest messages.
SELECT m.id, m.workspace_id, m.channel_id, m.seq, m.sender_id, m.client_msg_id, m.kind, m.body, m.payload, m.fallback_text,
       m.thread_root_id, m.mentions, m.mentions_channel, m.edited_at, m.deleted_at, m.created_at,
       (SELECT count(*) FROM chat_messages r WHERE r.thread_root_id = m.id AND r.deleted_at IS NULL)::bigint AS reply_count
FROM chat_messages m
WHERE m.workspace_id = @workspace_id AND m.channel_id = @channel_id
  AND (@before_seq::bigint = 0 OR m.seq < @before_seq::bigint)
  AND (sqlc.narg('thread_root_id')::uuid IS NULL OR m.thread_root_id = sqlc.narg('thread_root_id')::uuid)
ORDER BY m.seq DESC
LIMIT @page_size;

-- name: UpdateMessageBody :execrows
UPDATE chat_messages SET body = $3, mentions = $4, mentions_channel = $5, edited_at = $6
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteMessage :execrows
UPDATE chat_messages SET body = '', payload = NULL, fallback_text = '', mentions = '{}', mentions_channel = false, deleted_at = $3
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: UpdateMessagePayload :execrows
UPDATE chat_messages SET payload = $3
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL AND kind <> 'text';
