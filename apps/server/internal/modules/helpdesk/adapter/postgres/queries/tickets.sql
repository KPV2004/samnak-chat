-- name: NextTicketNumber :one
-- HD-04: the increment and the read happen in one statement, so two creates in
-- the same queue never get the same number.
UPDATE helpdesk_queues SET next_number = next_number + 1
WHERE id = @id AND workspace_id = @workspace_id AND archived_at IS NULL
RETURNING prefix, (next_number - 1)::bigint AS number;

-- name: InsertTicket :exec
INSERT INTO helpdesk_tickets (
    id, workspace_id, queue_id, category_id, number, title, description, requester_id, assignee_id, status, priority,
    source_channel_id, source_message_id, message_id, queue_message_id, resolved_at, closed_at, created_at, updated_at)
VALUES (
    @id, @workspace_id, @queue_id, @category_id, @number, @title, @description, @requester_id, @assignee_id, @status, @priority,
    @source_channel_id, @source_message_id, @message_id, @queue_message_id, @resolved_at, @closed_at, @created_at, @updated_at);

-- name: GetTicket :one
-- The queue's prefix comes with the ticket, so the key can be built.
SELECT sqlc.embed(t), q.prefix
FROM helpdesk_tickets t JOIN helpdesk_queues q ON q.id = t.queue_id
WHERE t.id = @id AND t.workspace_id = @workspace_id;

-- name: ListTickets :many
-- HD-10: the requester, the agents of the queue and the admins see a ticket.
SELECT sqlc.embed(t), q.prefix
FROM helpdesk_tickets t JOIN helpdesk_queues q ON q.id = t.queue_id
WHERE t.workspace_id = @workspace_id
  AND (@is_admin::boolean
       OR t.requester_id = @user_id
       OR t.queue_id IN (SELECT a.queue_id FROM helpdesk_queue_agents a WHERE a.user_id = @user_id))
  AND (sqlc.narg('queue_id')::uuid IS NULL OR t.queue_id = sqlc.narg('queue_id')::uuid)
  AND (cardinality(@statuses::text[]) = 0 OR t.status = ANY(@statuses::text[]))
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR t.assignee_id = sqlc.narg('assignee_id')::uuid)
  AND (@no_assignee::boolean = false OR t.assignee_id IS NULL)
  AND (sqlc.narg('requester_id')::uuid IS NULL OR t.requester_id = sqlc.narg('requester_id')::uuid)
  AND (sqlc.narg('before_id')::uuid IS NULL OR t.id < sqlc.narg('before_id')::uuid)
ORDER BY t.id DESC
LIMIT @page_size;

-- name: GetTicketForUpdate :one
SELECT sqlc.embed(t), q.prefix
FROM helpdesk_tickets t JOIN helpdesk_queues q ON q.id = t.queue_id
WHERE t.id = @id AND t.workspace_id = @workspace_id
FOR UPDATE OF t;

-- name: UpdateTicket :execrows
UPDATE helpdesk_tickets
SET queue_id = @queue_id, category_id = @category_id, assignee_id = @assignee_id, status = @status,
    priority = @priority, resolved_at = @resolved_at, closed_at = @closed_at, updated_at = @updated_at
WHERE id = @id AND workspace_id = @workspace_id;

-- name: SetRequesterCard :execrows
-- HD-08: the card in the requester's channel is stored once. A redelivered
-- event finds it set and changes nothing (PLT-10). updated_at stays: a card
-- is not a change of the ticket.
UPDATE helpdesk_tickets SET message_id = @message_id
WHERE id = @id AND workspace_id = @workspace_id AND message_id IS NULL;

-- name: SetQueueCard :execrows
-- HD-08: the card in the agents' channel, stored once like SetRequesterCard.
UPDATE helpdesk_tickets SET queue_message_id = @message_id
WHERE id = @id AND workspace_id = @workspace_id AND queue_message_id IS NULL;
