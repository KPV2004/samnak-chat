-- name: InsertAttachment :exec
INSERT INTO helpdesk_attachments (id, ticket_id, workspace_id, uploader_id, object_key, file_name, size, mime_type, from_message, created_at)
VALUES (@id, sqlc.narg('ticket_id')::uuid, @workspace_id, @uploader_id, @object_key, @file_name, @size, @mime_type, @from_message, @created_at);

-- name: GetAttachment :one
SELECT * FROM helpdesk_attachments WHERE id = @id AND workspace_id = @workspace_id;

-- name: AttachmentsByIDs :many
SELECT * FROM helpdesk_attachments WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: LinkAttachments :execrows
-- Only the uploader's files that no ticket uses yet, and never a file copied
-- from a message.
UPDATE helpdesk_attachments SET ticket_id = @ticket_id
WHERE workspace_id = @workspace_id AND uploader_id = @uploader_id AND ticket_id IS NULL
  AND from_message = false AND id = ANY(@ids::uuid[]);

-- name: ListTicketAttachments :many
SELECT * FROM helpdesk_attachments WHERE ticket_id = @ticket_id ORDER BY created_at, id;
