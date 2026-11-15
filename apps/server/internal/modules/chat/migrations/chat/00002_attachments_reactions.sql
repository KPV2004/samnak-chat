-- +goose Up
-- An attachment exists from the moment its upload is presigned; message_id is
-- set when the uploader sends a message that lists it.
CREATE TABLE chat_attachments (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    channel_id   uuid NOT NULL REFERENCES chat_channels (id) ON DELETE CASCADE,
    uploader_id  uuid NOT NULL,
    message_id   uuid REFERENCES chat_messages (id) ON DELETE CASCADE,
    object_key   text NOT NULL,
    file_name    text NOT NULL,
    size         bigint NOT NULL,
    mime_type    text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_attachments_size_valid CHECK (size > 0 AND size <= 52428800)
);

CREATE INDEX chat_attachments_message_idx ON chat_attachments (message_id) WHERE message_id IS NOT NULL;

CREATE TABLE chat_reactions (
    message_id   uuid NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL,
    user_id      uuid NOT NULL,
    emoji        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id, emoji)
);

CREATE INDEX chat_reactions_message_idx ON chat_reactions (message_id, created_at);

-- +goose Down
DROP TABLE chat_reactions;
DROP TABLE chat_attachments;
