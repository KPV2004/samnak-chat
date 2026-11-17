-- +goose Up
-- channel_id, creator_id, assignee_id and the message IDs refer to other
-- modules' rows by ID only: no foreign keys across modules.
CREATE TABLE task_items (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL,
    channel_id        uuid NOT NULL,
    source_message_id uuid,
    -- The card's message. NULL until the card is posted; the card handler sets
    -- it with UPDATE ... WHERE message_id IS NULL, so a redelivered event
    -- never posts a second card.
    message_id        uuid,
    title             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    creator_id        uuid NOT NULL,
    assignee_id       uuid,
    due_at            timestamptz,
    priority          text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high')),
    status            text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'doing', 'done')),
    -- Set by the due scan in the transaction that publishes the event; a new
    -- due_at clears both so the new date is announced again (TASK-06).
    due_soon_sent_at  timestamptz,
    overdue_sent_at   timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- "My tasks": by assignee and status.
CREATE INDEX task_items_assignee_idx ON task_items (workspace_id, assignee_id, status);
CREATE INDEX task_items_creator_idx ON task_items (workspace_id, creator_id);
CREATE INDEX task_items_channel_idx ON task_items (workspace_id, channel_id);
-- The due scan only looks at open tasks that have an assignee and a due date.
CREATE INDEX task_items_due_idx ON task_items (due_at)
    WHERE status <> 'done' AND assignee_id IS NOT NULL AND due_at IS NOT NULL;

-- +goose Down
DROP TABLE task_items;
