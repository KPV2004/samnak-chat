-- name: InsertRequest :exec
INSERT INTO approval_requests (id, workspace_id, type, requester_id, approver_id, status, payload, reason, created_at, updated_at)
VALUES (@id, @workspace_id, @type, @requester_id, @approver_id, @status, @payload, '', @created_at, @updated_at);

-- name: InsertStep :exec
INSERT INTO approval_steps (request_id, step_no, approver_id, status)
VALUES (@request_id, @step_no, @approver_id, @status);

-- name: InsertHistory :exec
INSERT INTO approval_history (id, request_id, actor_id, action, reason, at)
VALUES (@id, @request_id, @actor_id, @action, @reason, @at);

-- name: GetRequest :one
SELECT id, workspace_id, type, requester_id, approver_id, status, payload, channel_id, message_id, decided_at, reason, created_at, updated_at
FROM approval_requests
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListRequests :many
SELECT id, workspace_id, type, requester_id, approver_id, status, payload, channel_id, message_id, decided_at, reason, created_at, updated_at
FROM approval_requests
WHERE workspace_id = @workspace_id
  AND ((@as_requester::boolean AND requester_id = @user_id) OR (@as_approver::boolean AND approver_id = @user_id))
  AND (cardinality(@statuses::text[]) = 0 OR status = ANY(@statuses::text[]))
  AND (
    sqlc.narg('before_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('before_created_at')::timestamptz, sqlc.narg('before_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT @page_size;

-- name: DecideRequest :one
UPDATE approval_requests
SET status = @status, reason = @reason, decided_at = @at, updated_at = @at
WHERE id = @id AND workspace_id = @workspace_id AND status = 'pending'
RETURNING id, workspace_id, type, requester_id, approver_id, status, payload, channel_id, message_id, decided_at, reason, created_at, updated_at;

-- name: DecideStep :exec
UPDATE approval_steps
SET status = @status, reason = @reason, decided_at = @at
WHERE request_id = @request_id AND status = 'pending';

-- name: ListHistory :many
SELECT id, request_id, actor_id, action, reason, at
FROM approval_history
WHERE request_id = @request_id
ORDER BY at, id;

-- name: SetCard :execrows
UPDATE approval_requests SET channel_id = @channel_id, message_id = @message_id
WHERE id = @id AND workspace_id = @workspace_id AND message_id IS NULL;

-- name: InsertAttachment :exec
INSERT INTO approval_attachments (id, workspace_id, uploader_id, object_key, file_name, size, mime_type, created_at)
VALUES (@id, @workspace_id, @uploader_id, @object_key, @file_name, @size, @mime_type, @created_at);

-- name: GetAttachment :one
SELECT id, request_id, workspace_id, uploader_id, object_key, file_name, size, mime_type, created_at
FROM approval_attachments
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetAttachmentsByIDs :many
SELECT id, request_id, workspace_id, uploader_id, object_key, file_name, size, mime_type, created_at
FROM approval_attachments
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[])
ORDER BY created_at, id;

-- name: LinkAttachments :execrows
UPDATE approval_attachments SET request_id = @request_id
WHERE workspace_id = @workspace_id AND uploader_id = @uploader_id AND request_id IS NULL AND id = ANY(@ids::uuid[]);

-- name: ListRequestAttachments :many
SELECT id, request_id, workspace_id, uploader_id, object_key, file_name, size, mime_type, created_at
FROM approval_attachments
WHERE request_id = @request_id
ORDER BY created_at, id;
