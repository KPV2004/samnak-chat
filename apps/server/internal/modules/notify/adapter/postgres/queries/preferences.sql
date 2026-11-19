-- name: GetPreferences :many
SELECT user_id, workspace_id, disabled_kinds, muted_channel_ids, dnd_start, dnd_end, updated_at
FROM notify_preferences
WHERE workspace_id = @workspace_id AND user_id = ANY(@user_ids::uuid[]);

-- name: UpsertPreferences :exec
INSERT INTO notify_preferences (user_id, workspace_id, disabled_kinds, muted_channel_ids, dnd_start, dnd_end, updated_at)
VALUES (@user_id, @workspace_id, @disabled_kinds, @muted_channel_ids, @dnd_start, @dnd_end, @updated_at)
ON CONFLICT (user_id, workspace_id) DO UPDATE SET
    disabled_kinds    = EXCLUDED.disabled_kinds,
    muted_channel_ids = EXCLUDED.muted_channel_ids,
    dnd_start         = EXCLUDED.dnd_start,
    dnd_end           = EXCLUDED.dnd_end,
    updated_at        = EXCLUDED.updated_at;
