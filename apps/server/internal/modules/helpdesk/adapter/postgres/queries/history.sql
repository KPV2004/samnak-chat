-- name: InsertHistory :exec
INSERT INTO helpdesk_history (id, ticket_id, actor_id, action, from_value, to_value, internal, at)
VALUES (@id, @ticket_id, @actor_id, @action, @from_value, @to_value, @internal, @at);
