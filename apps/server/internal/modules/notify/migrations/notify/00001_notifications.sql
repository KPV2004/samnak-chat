-- +goose Up
-- user_id refers to another module's row by ID only: no foreign keys across
-- modules. key makes a notification idempotent per user (NOTI-02): the
-- unique index lets ON CONFLICT DO NOTHING drop a redelivered event.
CREATE TABLE notify_notifications (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    user_id      uuid NOT NULL,
    kind         text NOT NULL,
    title        text NOT NULL DEFAULT '',
    body         text NOT NULL DEFAULT '',
    link         text NOT NULL DEFAULT '',
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    key          text NOT NULL DEFAULT '',
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- A rule may omit the key; such notifications are never deduplicated.
CREATE UNIQUE INDEX notify_notifications_user_key ON notify_notifications (user_id, key) WHERE key <> '';

-- The inbox query: one workspace's notifications, newest first.
CREATE INDEX notify_notifications_inbox_idx ON notify_notifications (user_id, workspace_id, created_at DESC, id DESC);
-- The bell: the unread count and the unread filter.
CREATE INDEX notify_notifications_unread_idx ON notify_notifications (user_id, workspace_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notify_notifications;
