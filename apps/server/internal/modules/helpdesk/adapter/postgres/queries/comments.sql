-- name: InsertComment :exec
INSERT INTO helpdesk_comments (id, ticket_id, author_id, body, internal, created_at)
VALUES (@id, @ticket_id, @author_id, @body, @internal, @created_at);

-- name: ListComments :many
SELECT * FROM helpdesk_comments WHERE ticket_id = @ticket_id ORDER BY created_at, id;
