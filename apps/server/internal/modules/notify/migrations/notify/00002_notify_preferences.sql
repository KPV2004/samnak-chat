-- +goose Up
-- One row per user and workspace (NOTI-07). A user with no row has the
-- defaults: every kind on, no muted channel, no do-not-disturb window. The
-- window is both times or neither, and both are in the user's time zone.
CREATE TABLE notify_preferences (
    user_id           uuid NOT NULL,
    workspace_id      uuid NOT NULL,
    disabled_kinds    text[] NOT NULL DEFAULT '{}',
    muted_channel_ids uuid[] NOT NULL DEFAULT '{}',
    dnd_start         time,
    dnd_end           time,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, workspace_id),
    CHECK ((dnd_start IS NULL) = (dnd_end IS NULL))
);

-- +goose Down
DROP TABLE notify_preferences;
