-- name: InsertAttachment :exec
INSERT INTO chat_attachments (id, workspace_id, channel_id, uploader_id, object_key, file_name, size, mime_type, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetAttachment :one
SELECT id, workspace_id, channel_id, uploader_id, message_id, object_key, file_name, size, mime_type, created_at
FROM chat_attachments
WHERE workspace_id = $1 AND id = $2;

-- name: ListAttachmentsByIDs :many
SELECT id, workspace_id, channel_id, uploader_id, message_id, object_key, file_name, size, mime_type, created_at
FROM chat_attachments
WHERE workspace_id = $1 AND id = ANY (@ids::uuid[])
ORDER BY created_at, id;

-- name: ListMessageAttachments :many
SELECT id, workspace_id, channel_id, uploader_id, message_id, object_key, file_name, size, mime_type, created_at
FROM chat_attachments
WHERE workspace_id = $1 AND message_id = ANY (@message_ids::uuid[])
ORDER BY created_at, id;

-- name: AttachToMessage :execrows
-- Only files the uploader put in this channel and nobody has used yet.
UPDATE chat_attachments SET message_id = @message_id
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND uploader_id = @uploader_id
  AND message_id IS NULL AND id = ANY (@ids::uuid[]);
