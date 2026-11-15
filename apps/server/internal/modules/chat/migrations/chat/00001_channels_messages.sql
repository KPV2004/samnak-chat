-- +goose Up
-- workspace_id, user_id and sender_id refer to other modules' rows by ID only:
-- no foreign keys across modules.
CREATE TABLE chat_channels (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    type         text NOT NULL,
    name         text NOT NULL DEFAULT '',
    topic        text NOT NULL DEFAULT '',
    -- dm_key is the sorted member IDs of a direct message, so the same set
    -- of people always maps to the same channel.
    dm_key       text,
    created_by   uuid NOT NULL,
    last_seq     bigint NOT NULL DEFAULT 0,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_channels_type_valid CHECK (type IN ('public', 'private', 'dm')),
    CONSTRAINT chat_channels_shape CHECK (
        (type = 'dm' AND dm_key IS NOT NULL AND name = '')
        OR (type <> 'dm' AND dm_key IS NULL AND name <> '')
    )
);

CREATE UNIQUE INDEX chat_channels_name_key ON chat_channels (workspace_id, lower(name)) WHERE type <> 'dm';
CREATE UNIQUE INDEX chat_channels_dm_key ON chat_channels (workspace_id, dm_key) WHERE type = 'dm';
CREATE INDEX chat_channels_workspace_idx ON chat_channels (workspace_id, type);

CREATE TABLE chat_channel_members (
    channel_id    uuid NOT NULL REFERENCES chat_channels (id) ON DELETE CASCADE,
    workspace_id  uuid NOT NULL,
    user_id       uuid NOT NULL,
    last_read_seq bigint NOT NULL DEFAULT 0,
    muted         boolean NOT NULL DEFAULT false,
    joined_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX chat_channel_members_user_idx ON chat_channel_members (workspace_id, user_id);

CREATE TABLE chat_messages (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL,
    channel_id       uuid NOT NULL REFERENCES chat_channels (id) ON DELETE CASCADE,
    seq              bigint NOT NULL,
    sender_id        uuid NOT NULL,
    client_msg_id    uuid NOT NULL,
    kind             text NOT NULL DEFAULT 'text',
    body             text NOT NULL DEFAULT '',
    payload          jsonb,
    fallback_text    text NOT NULL DEFAULT '',
    thread_root_id   uuid REFERENCES chat_messages (id),
    mentions         uuid[] NOT NULL DEFAULT '{}',
    mentions_channel boolean NOT NULL DEFAULT false,
    edited_at        timestamptz,
    deleted_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_messages_channel_seq_key UNIQUE (channel_id, seq),
    CONSTRAINT chat_messages_sender_client_key UNIQUE (sender_id, client_msg_id)
);

CREATE INDEX chat_messages_thread_idx ON chat_messages (thread_root_id) WHERE thread_root_id IS NOT NULL;

-- +goose Down
DROP TABLE chat_messages;
DROP TABLE chat_channel_members;
DROP TABLE chat_channels;
