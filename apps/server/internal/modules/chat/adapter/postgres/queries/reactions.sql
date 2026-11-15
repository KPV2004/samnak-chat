-- name: InsertReaction :execrows
INSERT INTO chat_reactions (message_id, workspace_id, user_id, emoji, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (message_id, user_id, emoji) DO NOTHING;

-- name: DeleteReaction :execrows
DELETE FROM chat_reactions WHERE workspace_id = $1 AND message_id = $2 AND user_id = $3 AND emoji = $4;

-- name: ListReactions :many
-- One row per message and emoji, in the order the emoji first appeared.
SELECT message_id, emoji, array_agg(user_id ORDER BY created_at, user_id)::uuid[] AS user_ids
FROM chat_reactions
WHERE workspace_id = $1 AND message_id = ANY (@message_ids::uuid[])
GROUP BY message_id, emoji
ORDER BY message_id, min(created_at), emoji;
