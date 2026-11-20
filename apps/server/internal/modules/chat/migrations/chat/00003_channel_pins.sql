-- +goose Up
-- A pin belongs to one member of one channel: it is the user's own choice of
-- where the channel sits in their list. NULL means not pinned.
ALTER TABLE chat_channel_members ADD COLUMN pinned_at timestamptz;

-- +goose Down
ALTER TABLE chat_channel_members DROP COLUMN pinned_at;
