-- name: InsertChannel :exec
INSERT INTO chat_channels (id, workspace_id, type, name, topic, dm_key, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetDirectChannelID :one
SELECT id FROM chat_channels
WHERE workspace_id = $1 AND type = 'dm' AND dm_key = $2;

-- name: GetChannel :one
SELECT id, workspace_id, type, name, topic, dm_key, created_by, last_seq, archived_at, created_at
FROM chat_channels
WHERE workspace_id = $1 AND id = $2;

-- name: UpdateChannel :execrows
UPDATE chat_channels SET name = $3, topic = $4, archived_at = $5
WHERE workspace_id = $1 AND id = $2;

-- name: NextSeq :one
-- Takes the channel row lock until the transaction ends, so senders to one
-- channel run one at a time and a rolled-back send leaves no gap.
UPDATE chat_channels SET last_seq = last_seq + 1
WHERE workspace_id = $1 AND id = $2 AND archived_at IS NULL
RETURNING last_seq;

-- name: ChannelViews :many
-- mode: 'one' (channel_id), 'joined' (channels the user is in) or 'public'
-- (unarchived public channels). Counts are for the user and exclude their own
-- and deleted messages. last_message is the channel's newest message, deleted
-- ones included. The list is pinned first (newest pin first), then by the
-- newest message, then by name.
SELECT c.id, c.workspace_id, c.type, c.name, c.topic, c.dm_key, c.created_by, c.last_seq, c.archived_at, c.created_at,
       (m.user_id IS NOT NULL)::boolean AS is_member,
       m.pinned_at,
       COALESCE(m.last_read_seq, 0)::bigint AS last_read_seq,
       (SELECT count(*) FROM chat_channel_members x WHERE x.channel_id = c.id)::bigint AS member_count,
       (CASE WHEN m.user_id IS NULL THEN 0 ELSE (
            SELECT count(*) FROM chat_messages g
            WHERE g.channel_id = c.id AND g.seq > m.last_read_seq AND g.deleted_at IS NULL AND g.sender_id <> m.user_id
       ) END)::bigint AS unread_count,
       (CASE WHEN m.user_id IS NULL THEN 0 ELSE (
            SELECT count(*) FROM chat_messages g
            WHERE g.channel_id = c.id AND g.seq > m.last_read_seq AND g.deleted_at IS NULL AND g.sender_id <> m.user_id
              AND (g.mentions_channel OR m.user_id = ANY (g.mentions))
       ) END)::bigint AS mention_count,
       lm.id AS last_message_id, lm.sender_id AS last_message_sender_id, lm.kind AS last_message_kind,
       lm.body AS last_message_body, lm.fallback_text AS last_message_fallback_text,
       lm.deleted_at AS last_message_deleted_at, lm.created_at AS last_message_created_at
FROM chat_channels c
LEFT JOIN chat_channel_members m ON m.channel_id = c.id AND m.user_id = @user_id
-- The newest message has the channel's last_seq, so this is one indexed row.
LEFT JOIN chat_messages lm ON lm.channel_id = c.id AND lm.seq = c.last_seq
WHERE c.workspace_id = @workspace_id
  AND (
       (@mode::text = 'one' AND c.id = @channel_id)
    OR (@mode::text = 'joined' AND m.user_id IS NOT NULL)
    OR (@mode::text = 'public' AND c.type = 'public' AND c.archived_at IS NULL)
  )
ORDER BY m.pinned_at DESC NULLS LAST, lm.created_at DESC NULLS LAST, lower(c.name), c.id;
