-- name: InsertChannelMember :execrows
INSERT INTO chat_channel_members (channel_id, workspace_id, user_id, joined_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, user_id) DO NOTHING;

-- name: DeleteChannelMember :execrows
DELETE FROM chat_channel_members WHERE workspace_id = $1 AND channel_id = $2 AND user_id = $3;

-- name: DeleteUserChannelMemberships :many
DELETE FROM chat_channel_members WHERE workspace_id = $1 AND user_id = $2
RETURNING channel_id;

-- name: ListChannelMembers :many
SELECT user_id, joined_at FROM chat_channel_members
WHERE workspace_id = $1 AND channel_id = $2
ORDER BY joined_at, user_id;

-- name: ListMembersOfChannels :many
SELECT channel_id, user_id FROM chat_channel_members
WHERE workspace_id = $1 AND channel_id = ANY (@channel_ids::uuid[])
ORDER BY channel_id, user_id;

-- name: FilterChannelMembers :many
SELECT user_id FROM chat_channel_members
WHERE workspace_id = $1 AND channel_id = $2 AND user_id = ANY (@user_ids::uuid[]);

-- name: AdvanceRead :one
-- Moves last_read_seq forward only, and never past the channel's last_seq.
-- No row means the position did not change or the user is not a member.
UPDATE chat_channel_members m
SET last_read_seq = LEAST(@seq::bigint, c.last_seq)
FROM chat_channels c
WHERE c.id = m.channel_id AND c.workspace_id = @workspace_id
  AND m.channel_id = @channel_id AND m.user_id = @user_id
  AND LEAST(@seq::bigint, c.last_seq) > m.last_read_seq
RETURNING m.last_read_seq;
