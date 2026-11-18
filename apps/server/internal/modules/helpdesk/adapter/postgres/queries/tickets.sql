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
