-- name: InsertAudit :exec
INSERT INTO workspace_audit (id, workspace_id, actor_id, action, target_type, target_id, data, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListAudit :many
SELECT id, workspace_id, actor_id, action, target_type, target_id, data, created_at
FROM workspace_audit
WHERE workspace_id = @workspace_id
  AND (@use_cursor::boolean = false OR (created_at, id) < (@after_created_at::timestamptz, @after_id::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @page_size;
